package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"pre-processor/handler"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestNewHTTPServer_UntrustedForwardingHeadersDoNotSpoofRealIP(t *testing.T) {
	deps := newTestHTTPServer(t, "unit-test-secret")
	srv := NewHTTPServer(deps, false, "")

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", nil)
	req.RemoteAddr = "203.0.113.10:43210"
	req.Header.Set("X-Forwarded-For", "198.51.100.1")
	req.Header.Set("X-Real-IP", "198.51.100.2")
	rec := httptest.NewRecorder()

	c := srv.NewContext(req, rec)
	assert.Equal(t, "203.0.113.10", c.RealIP(),
		"spoofed X-Forwarded-For / X-Real-IP from untrusted peer must not change c.RealIP()")
}

func TestNewHTTPServer_RequestLoggerFieldsAndHealthSkip(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	deps := &Dependencies{
		SummarizeHandler: handler.NewSummarizeHandler(nil, nil, nil, nil, logger),
		Logger:           logger,
	}
	srv := NewHTTPServer(deps, false, "")

	// Health routes must be skipped by the request logger.
	for _, path := range []string{"/health", "/api/v1/health"} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	}
	assert.NotContains(t, logBuf.String(), "HTTP request completed")

	// Non-health request must log the expected structured keys.
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/summarize", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	var found map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(logBuf.Bytes()), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal(line, &entry))
		if entry["msg"] == "HTTP request completed" {
			found = entry
			break
		}
	}
	require.NotNil(t, found, "expected 'HTTP request completed' log entry, got: %s", logBuf.String())
	assert.Equal(t, http.MethodPost, found["method"])
	assert.Equal(t, "/api/v1/summarize", found["uri"])
	assert.InDelta(t, float64(http.StatusUnauthorized), found["status"], 0)
	assert.Contains(t, found, "latency")
	assert.Contains(t, found, "error")
}

func TestNewHTTPServer_OTelMiddlewareTracesAPIAndSkipsHealth(t *testing.T) {
	spanRecorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	origTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(origTP)

	deps := newTestHTTPServer(t, "unit-test-secret")
	srv := NewHTTPServer(deps, true, "pre-processor")

	// Health routes must be skipped by OTelSkipper.
	for _, path := range []string{"/health", "/api/v1/health"} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	}
	assert.Empty(t, spanRecorder.Ended())

	// Business route must record a span.
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/v1/summarize", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	spans := spanRecorder.Ended()
	require.Len(t, spans, 1)
}

func TestStartHTTPServer_ConfigAndShutdown(t *testing.T) {
	t.Setenv("HTTP_PORT", "0")
	deps := newTestHTTPServer(t, "unit-test-secret")
	e := NewHTTPServer(deps, false, "")
	errCh := make(chan error, 1)

	srv := StartHTTPServer(e, deps.Logger, errCh)
	require.NotNil(t, srv)
	assert.Equal(t, ":0", srv.Addr)
	assert.Equal(t, 10*time.Second, srv.ReadHeaderTimeout)
	assert.Zero(t, srv.WriteTimeout)

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	require.NoError(t, srv.Shutdown(ctx))

	select {
	case err := <-errCh:
		t.Fatalf("unexpected server error: %v", err)
	default:
	}
}
