package prometheus_client

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestClassifyHTTPError(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		body        string
		wantKind    ErrKind
		wantType    string
		wantMessage string
	}{
		{
			name:        "errorType bad_data",
			status:      http.StatusBadRequest,
			body:        `{"status":"error","errorType":"bad_data","error":"bad query syntax"}`,
			wantKind:    ErrKindBadData,
			wantType:    "bad_data",
			wantMessage: "bad query syntax",
		},
		{
			name:        "errorType invalid_input",
			status:      http.StatusBadRequest,
			body:        `{"status":"error","errorType":"invalid_input","error":"invalid step"}`,
			wantKind:    ErrKindBadData,
			wantType:    "invalid_input",
			wantMessage: "invalid step",
		},
		{
			name:        "errorType execution",
			status:      http.StatusUnprocessableEntity,
			body:        `{"status":"error","errorType":"execution","error":"expression error"}`,
			wantKind:    ErrKindExecution,
			wantType:    "execution",
			wantMessage: "expression error",
		},
		{
			name:        "errorType timeout",
			status:      http.StatusServiceUnavailable,
			body:        `{"status":"error","errorType":"timeout","error":"query execution timed out"}`,
			wantKind:    ErrKindTimeout,
			wantType:    "timeout",
			wantMessage: "query execution timed out",
		},
		{
			name:        "errorType unavailable",
			status:      http.StatusServiceUnavailable,
			body:        `{"status":"error","errorType":"unavailable","error":"server shutting down"}`,
			wantKind:    ErrKindUnavailable,
			wantType:    "unavailable",
			wantMessage: "server shutting down",
		},
		{
			name:        "fallback status 400 bad data",
			status:      http.StatusBadRequest,
			body:        `plain bad request`,
			wantKind:    ErrKindBadData,
			wantType:    "",
			wantMessage: "plain bad request",
		},
		{
			name:        "fallback status 422 execution",
			status:      http.StatusUnprocessableEntity,
			body:        `plain unprocessable`,
			wantKind:    ErrKindExecution,
			wantType:    "",
			wantMessage: "plain unprocessable",
		},
		{
			name:        "fallback status 500 unavailable",
			status:      http.StatusInternalServerError,
			body:        `internal server error`,
			wantKind:    ErrKindUnavailable,
			wantType:    "",
			wantMessage: "internal server error",
		},
		{
			name:        "fallback other status unknown",
			status:      http.StatusNotFound,
			body:        `404 not found`,
			wantKind:    ErrKindUnknown,
			wantType:    "",
			wantMessage: "404 not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := classifyHTTPError(tt.status, []byte(tt.body))
			var qErr *QueryError
			if !errors.As(err, &qErr) {
				t.Fatalf("classifyHTTPError() error type = %T, want *QueryError", err)
			}
			if qErr.Kind != tt.wantKind {
				t.Errorf("Kind = %v, want %v", qErr.Kind, tt.wantKind)
			}
			if qErr.Status != tt.status {
				t.Errorf("Status = %v, want %v", qErr.Status, tt.status)
			}
			if qErr.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", qErr.Type, tt.wantType)
			}
			if qErr.Message != tt.wantMessage {
				t.Errorf("Message = %q, want %q", qErr.Message, tt.wantMessage)
			}
		})
	}
}

func TestDecodeInstantResult_Success(t *testing.T) {
	raw := `{"status":"success","warnings":["test warning"],"data":{"resultType":"vector","result":[{"metric":{"job":"alt"},"value":[1700000000.5,"42.5"]}]}}`
	res, err := decodeInstantResult([]byte(raw))
	if err != nil {
		t.Fatalf("decodeInstantResult() error: %v", err)
	}
	if len(res.Warnings) != 1 || res.Warnings[0] != "test warning" {
		t.Fatalf("unexpected warnings: %v", res.Warnings)
	}
	if len(res.Vector) != 1 {
		t.Fatalf("want 1 sample, got %d", len(res.Vector))
	}
	s := res.Vector[0]
	if s.Labels["job"] != "alt" {
		t.Errorf("label job = %q, want alt", s.Labels["job"])
	}
	if s.Value != 42.5 {
		t.Errorf("value = %v, want 42.5", s.Value)
	}
	expectedTime := time.Unix(1700000000, 500000000).UTC()
	if !s.Time.Equal(expectedTime) {
		t.Errorf("time = %v, want %v", s.Time, expectedTime)
	}
}

func TestDecodeInstantResult_Errors(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name:    "invalid envelope json",
			body:    `{invalid`,
			wantErr: true,
		},
		{
			name:    "status not success",
			body:    `{"status":"error","errorType":"bad_data","error":"boom"}`,
			wantErr: true,
		},
		{
			name:    "invalid data json",
			body:    `{"status":"success","data":"not-an-object"}`,
			wantErr: true,
		},
		{
			name:    "unexpected resultType matrix",
			body:    `{"status":"success","data":{"resultType":"matrix","result":[]}}`,
			wantErr: true,
		},
		{
			name:    "malformed vector sample",
			body:    `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":["invalid_ts","1"]}]}}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeInstantResult([]byte(tt.body))
			if (err != nil) != tt.wantErr {
				t.Fatalf("decodeInstantResult() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDecodeRangeResult_Success(t *testing.T) {
	raw := `{"status":"success","warnings":["warn"],"data":{"resultType":"matrix","result":[{"metric":{"instance":"node1"},"values":[[1700000000,"10"],[1700000015,"20"]]}]}}`
	res, err := decodeRangeResult([]byte(raw))
	if err != nil {
		t.Fatalf("decodeRangeResult() error: %v", err)
	}
	if len(res.Warnings) != 1 || res.Warnings[0] != "warn" {
		t.Fatalf("unexpected warnings: %v", res.Warnings)
	}
	if len(res.Matrix) != 1 {
		t.Fatalf("want 1 series, got %d", len(res.Matrix))
	}
	s := res.Matrix[0]
	if s.Labels["instance"] != "node1" {
		t.Errorf("label instance = %q, want node1", s.Labels["instance"])
	}
	if len(s.Points) != 2 {
		t.Fatalf("want 2 points, got %d", len(s.Points))
	}
	if s.Points[0].Value != 10 || s.Points[1].Value != 20 {
		t.Errorf("point values = [%v, %v], want [10, 20]", s.Points[0].Value, s.Points[1].Value)
	}
}

func TestDecodeRangeResult_Errors(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name:    "invalid envelope json",
			body:    `{broken`,
			wantErr: true,
		},
		{
			name:    "status not success",
			body:    `{"status":"error","errorType":"execution","error":"query error"}`,
			wantErr: true,
		},
		{
			name:    "invalid data json",
			body:    `{"status":"success","data":"bad"}`,
			wantErr: true,
		},
		{
			name:    "unexpected resultType vector",
			body:    `{"status":"success","data":{"resultType":"vector","result":[]}}`,
			wantErr: true,
		},
		{
			name:    "malformed matrix points",
			body:    `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{},"values":[[1700000000, 123]]}]}}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeRangeResult([]byte(tt.body))
			if (err != nil) != tt.wantErr {
				t.Fatalf("decodeRangeResult() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFormatPrometheusTime(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{
			name: "exact second",
			in:   time.Unix(1700000000, 0),
			want: "1700000000",
		},
		{
			name: "fractional second",
			in:   time.Unix(1700000000, 500000000),
			want: "1700000000.5",
		},
		{
			name: "subsecond precision",
			in:   time.Unix(1700000000, 125000000),
			want: "1700000000.125",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatPrometheusTime(tt.in)
			if got != tt.want {
				t.Errorf("formatPrometheusTime(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
