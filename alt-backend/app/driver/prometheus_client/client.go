package prometheus_client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type Config struct {
	URL     string
	Timeout time.Duration
}

type Client struct {
	base       *url.URL
	httpClient *http.Client
	timeout    time.Duration
}

func New(cfg Config) (*Client, error) {
	if cfg.URL == "" {
		return nil, errors.New("prometheus_client: URL is required")
	}
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("prometheus_client: parse URL: %w", err)
	}
	to := cfg.Timeout
	if to <= 0 {
		to = 3 * time.Second
	}
	return &Client{
		base:       u,
		httpClient: &http.Client{Timeout: to},
		timeout:    to,
	}, nil
}

type ErrKind int

const (
	ErrKindUnknown ErrKind = iota
	ErrKindTimeout
	ErrKindBadData
	ErrKindExecution
	ErrKindUnavailable
)

type QueryError struct {
	Kind    ErrKind
	Status  int
	Type    string
	Message string
}

func (e *QueryError) Error() string {
	return fmt.Sprintf("prometheus query error (kind=%d status=%d type=%s): %s", e.Kind, e.Status, e.Type, e.Message)
}

type Sample struct {
	Labels map[string]string
	Time   time.Time
	Value  float64
}

type Series struct {
	Labels map[string]string
	Points []SeriesPoint
}

type SeriesPoint struct {
	Time  time.Time
	Value float64
}

type Result struct {
	Vector   []Sample
	Matrix   []Series
	Warnings []string
}

// QueryInstant executes an instant PromQL query at time ts.
func (c *Client) QueryInstant(ctx context.Context, promql string, ts time.Time) (*Result, error) {
	q := url.Values{}
	q.Set("query", promql)
	if !ts.IsZero() {
		q.Set("time", formatTime(ts))
	}
	q.Set("timeout", fmt.Sprintf("%dms", c.timeout.Milliseconds()))
	body, err := c.get(ctx, "/api/v1/query", q)
	if err != nil {
		return nil, err
	}
	return decodeInstant(body)
}

// QueryRange executes a range PromQL query between start and end with step.
func (c *Client) QueryRange(ctx context.Context, promql string, start, end time.Time, step time.Duration) (*Result, error) {
	q := url.Values{}
	q.Set("query", promql)
	q.Set("start", formatTime(start))
	q.Set("end", formatTime(end))
	q.Set("step", fmt.Sprintf("%ds", int64(step.Seconds())))
	q.Set("timeout", fmt.Sprintf("%dms", c.timeout.Milliseconds()))
	body, err := c.get(ctx, "/api/v1/query_range", q)
	if err != nil {
		return nil, err
	}
	return decodeRange(body)
}

// Health returns nil if Prometheus /-/ready responds with 200.
func (c *Client) Health(ctx context.Context) error {
	u := *c.base
	u.Path = "/-/ready"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return classifyTransport(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &QueryError{Kind: ErrKindUnavailable, Status: resp.StatusCode, Message: "prometheus not ready"}
	}
	return nil
}

func (c *Client) get(ctx context.Context, path string, q url.Values) ([]byte, error) {
	u := *c.base
	u.Path = path
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, classifyTransport(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("prometheus_client: read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, classifyHTTP(resp.StatusCode, raw)
	}
	return raw, nil
}

func classifyTransport(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return &QueryError{Kind: ErrKindTimeout, Message: err.Error()}
	}
	// net/http wraps timeout as url.Error with Timeout() bool.
	type timeoutErr interface{ Timeout() bool }
	var te timeoutErr
	if errors.As(err, &te) && te.Timeout() {
		return &QueryError{Kind: ErrKindTimeout, Message: err.Error()}
	}
	return &QueryError{Kind: ErrKindUnavailable, Message: err.Error()}
}

func classifyHTTP(status int, body []byte) error {
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
	ResultType string            `json:"resultType"`
	Result     json.RawMessage   `json:"result"`
}

type promSample struct {
	Metric map[string]string `json:"metric"`
	Value  [2]interface{}    `json:"value"`
}

type promMatrix struct {
	Metric map[string]string `json:"metric"`
	Values [][2]interface{}  `json:"values"`
}

func decodeInstant(raw []byte) (*Result, error) {
	var env promResponse
	if err := json.Unmarshal(raw, &env); err != nil {
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

func decodeRange(raw []byte) (*Result, error) {
	var env promResponse
	if err := json.Unmarshal(raw, &env); err != nil {
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

func formatTime(t time.Time) string {
	return strconv.FormatFloat(float64(t.Unix())+float64(t.Nanosecond())/1e9, 'f', -1, 64)
}
