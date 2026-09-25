package prometheus_client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// classifyHTTPError parses Prometheus error payloads or maps HTTP status codes.
func classifyHTTPError(status int, body []byte) error {
	var env promResponse
	_ = json.Unmarshal(body, &env)
	kind := ErrKindUnknown
	switch env.ErrorType {
	case "bad_data", "invalid_input":
		kind = ErrKindBadData
	case "execution", "timeout":
		kind = ErrKindExecution
		if env.ErrorType == "timeout" {
			kind = ErrKindTimeout
		}
	case "unavailable":
		kind = ErrKindUnavailable
	}
	if kind == ErrKindUnknown {
		switch {
		case status == http.StatusBadRequest:
			kind = ErrKindBadData
		case status == http.StatusUnprocessableEntity:
			kind = ErrKindExecution
		case status >= 500:
			kind = ErrKindUnavailable
		}
	}
	msg := env.Error
	if msg == "" {
		msg = string(body)
	}
	return &QueryError{Kind: kind, Status: status, Type: env.ErrorType, Message: msg}
}

type promResponse struct {
	Status    string          `json:"status"`
	Data      json.RawMessage `json:"data"`
	ErrorType string          `json:"errorType"`
	Error     string          `json:"error"`
	Warnings  []string        `json:"warnings"`
}

type promData struct {
	ResultType string          `json:"resultType"`
	Result     json.RawMessage `json:"result"`
}

type promSample struct {
	Metric map[string]string `json:"metric"`
	Value  [2]interface{}    `json:"value"`
}

type promMatrix struct {
	Metric map[string]string `json:"metric"`
	Values [][2]interface{}  `json:"values"`
}

// decodeInstantResult decodes instant query response payloads.
func decodeInstantResult(body []byte) (*Result, error) {
	var env promResponse
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("prometheus_client: decode envelope: %w", err)
	}
	if env.Status != "success" {
		return nil, &QueryError{Kind: ErrKindBadData, Type: env.ErrorType, Message: env.Error}
	}
	var d promData
	if err := json.Unmarshal(env.Data, &d); err != nil {
		return nil, fmt.Errorf("prometheus_client: decode data: %w", err)
	}
	if d.ResultType != "vector" && d.ResultType != "scalar" {
		return nil, fmt.Errorf("prometheus_client: unexpected resultType %q", d.ResultType)
	}
	var samples []promSample
	if err := json.Unmarshal(d.Result, &samples); err != nil {
		return nil, fmt.Errorf("prometheus_client: decode vector: %w", err)
	}
	out := &Result{Warnings: env.Warnings, Vector: make([]Sample, 0, len(samples))}
	for _, s := range samples {
		ts, val, err := parsePair(s.Value)
		if err != nil {
			return nil, err
		}
		out.Vector = append(out.Vector, Sample{Labels: s.Metric, Time: ts, Value: val})
	}
	return out, nil
}

// decodeRangeResult decodes range query response payloads.
func decodeRangeResult(body []byte) (*Result, error) {
	var env promResponse
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("prometheus_client: decode envelope: %w", err)
	}
	if env.Status != "success" {
		return nil, &QueryError{Kind: ErrKindBadData, Type: env.ErrorType, Message: env.Error}
	}
	var d promData
	if err := json.Unmarshal(env.Data, &d); err != nil {
		return nil, fmt.Errorf("prometheus_client: decode data: %w", err)
	}
	if d.ResultType != "matrix" {
		return nil, fmt.Errorf("prometheus_client: unexpected resultType %q", d.ResultType)
	}
	var matrix []promMatrix
	if err := json.Unmarshal(d.Result, &matrix); err != nil {
		return nil, fmt.Errorf("prometheus_client: decode matrix: %w", err)
	}
	out := &Result{Warnings: env.Warnings, Matrix: make([]Series, 0, len(matrix))}
	for _, s := range matrix {
		pts := make([]SeriesPoint, 0, len(s.Values))
		for _, v := range s.Values {
			ts, val, err := parsePair(v)
			if err != nil {
				return nil, err
			}
			pts = append(pts, SeriesPoint{Time: ts, Value: val})
		}
		out.Matrix = append(out.Matrix, Series{Labels: s.Metric, Points: pts})
	}
	return out, nil
}

func parsePair(pair [2]interface{}) (time.Time, float64, error) {
	var tsFloat float64
	switch v := pair[0].(type) {
	case float64:
		tsFloat = v
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return time.Time{}, 0, fmt.Errorf("prometheus_client: parse ts: %w", err)
		}
		tsFloat = f
	default:
		return time.Time{}, 0, fmt.Errorf("prometheus_client: unsupported ts type %T", v)
	}
	str, ok := pair[1].(string)
	if !ok {
		return time.Time{}, 0, fmt.Errorf("prometheus_client: value not string: %T", pair[1])
	}
	val, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("prometheus_client: parse value: %w", err)
	}
	sec := int64(tsFloat)
	nsec := int64((tsFloat - float64(sec)) * 1e9)
	return time.Unix(sec, nsec).UTC(), val, nil
}

// formatPrometheusTime formats a timestamp as Prometheus seconds float string.
func formatPrometheusTime(t time.Time) string {
	return strconv.FormatFloat(float64(t.Unix())+float64(t.Nanosecond())/1e9, 'f', -1, 64)
}
