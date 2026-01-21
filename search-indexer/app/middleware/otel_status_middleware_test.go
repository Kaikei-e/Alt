package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func setupTestTracer(t *testing.T) (*tracetest.SpanRecorder, func()) {
	t.Helper()

	spanRecorder := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(spanRecorder),
	)

	originalProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(tracerProvider)

	cleanup := func() {
		otel.SetTracerProvider(originalProvider)
	}

	return spanRecorder, cleanup
}

func TestOTelStatusHandler_2xxResponse_StatusUnset(t *testing.T) {
	spanRecorder, cleanup := setupTestTracer(t)
	defer cleanup()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	})

	wrappedHandler := OTelStatusHandler(handler, "/test")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	spans := spanRecorder.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Unset, spans[0].Status().Code)

	attrs := spans[0].Attributes()
	var statusCodeFound bool
	for _, attr := range attrs {
		if string(attr.Key) == "http.response.status_code" {
			statusCodeFound = true
			assert.Equal(t, int64(200), attr.Value.AsInt64())
		}
	}
	assert.True(t, statusCodeFound, "http.response.status_code attribute not found")
}

func TestOTelStatusHandler_4xxResponse_StatusUnset(t *testing.T) {
	spanRecorder, cleanup := setupTestTracer(t)
	defer cleanup()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	})

	wrappedHandler := OTelStatusHandler(handler, "/notfound")

	req := httptest.NewRequest(http.MethodGet, "/notfound", nil)
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)

	spans := spanRecorder.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Unset, spans[0].Status().Code)

	attrs := spans[0].Attributes()
	var statusCodeFound bool
	for _, attr := range attrs {
		if string(attr.Key) == "http.response.status_code" {
			statusCodeFound = true
			assert.Equal(t, int64(404), attr.Value.AsInt64())
		}
	}
	assert.True(t, statusCodeFound, "http.response.status_code attribute not found")
}

func TestOTelStatusHandler_5xxResponse_StatusError(t *testing.T) {
	spanRecorder, cleanup := setupTestTracer(t)
	defer cleanup()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	})

	wrappedHandler := OTelStatusHandler(handler, "/error")

	req := httptest.NewRequest(http.MethodGet, "/error", nil)
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	spans := spanRecorder.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Error, spans[0].Status().Code)
	assert.Equal(t, "Internal Server Error", spans[0].Status().Description)

	attrs := spans[0].Attributes()
	var statusCodeFound bool
	for _, attr := range attrs {
		if string(attr.Key) == "http.response.status_code" {
			statusCodeFound = true
			assert.Equal(t, int64(500), attr.Value.AsInt64())
		}
	}
	assert.True(t, statusCodeFound, "http.response.status_code attribute not found")
}

func TestOTelStatusHandler_ServiceUnavailable(t *testing.T) {
	spanRecorder, cleanup := setupTestTracer(t)
	defer cleanup()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("service unavailable"))
	})

	wrappedHandler := OTelStatusHandler(handler, "/busy")

	req := httptest.NewRequest(http.MethodGet, "/busy", nil)
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	spans := spanRecorder.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Error, spans[0].Status().Code)
	assert.Equal(t, "Service Unavailable", spans[0].Status().Description)
}

func TestOTelStatusHandler_DefaultStatusOK(t *testing.T) {
	spanRecorder, cleanup := setupTestTracer(t)
	defer cleanup()

	// Handler that doesn't explicitly write header (defaults to 200)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("success"))
	})

	wrappedHandler := OTelStatusHandler(handler, "/default")

	req := httptest.NewRequest(http.MethodGet, "/default", nil)
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	spans := spanRecorder.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Unset, spans[0].Status().Code)
}
