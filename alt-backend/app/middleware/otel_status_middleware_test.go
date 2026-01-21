package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func setupTestTracer(t *testing.T) (*tracetest.SpanRecorder, func()) {
	t.Helper()

	// Create a span recorder to capture spans
	spanRecorder := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(spanRecorder),
	)

	// Set as global tracer provider
	originalProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(tracerProvider)

	cleanup := func() {
		otel.SetTracerProvider(originalProvider)
	}

	return spanRecorder, cleanup
}

func TestOTelStatusMiddleware_2xxResponse_StatusUnset(t *testing.T) {
	spanRecorder, cleanup := setupTestTracer(t)
	defer cleanup()

	e := echo.New()
	tracer := otel.Tracer("test")

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Create a span and set it in the context (simulating otelecho middleware)
	ctx, span := tracer.Start(req.Context(), "test-span")
	c.SetRequest(req.WithContext(ctx))
	defer span.End()

	middleware := OTelStatusMiddleware()
	handler := func(c echo.Context) error {
		return c.String(http.StatusOK, "success")
	}

	err := middleware(handler)(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// End span to flush to recorder
	span.End()

	// Check span status is Unset (default for 2xx)
	spans := spanRecorder.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Unset, spans[0].Status().Code)

	// Check http.response.status_code attribute is set
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

func TestOTelStatusMiddleware_4xxResponse_StatusUnset(t *testing.T) {
	spanRecorder, cleanup := setupTestTracer(t)
	defer cleanup()

	e := echo.New()
	tracer := otel.Tracer("test")

	req := httptest.NewRequest(http.MethodGet, "/api/notfound", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	ctx, span := tracer.Start(req.Context(), "test-span")
	c.SetRequest(req.WithContext(ctx))
	defer span.End()

	middleware := OTelStatusMiddleware()
	handler := func(c echo.Context) error {
		return c.String(http.StatusNotFound, "not found")
	}

	err := middleware(handler)(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	span.End()

	// Check span status is Unset (4xx is client error, not server error)
	spans := spanRecorder.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Unset, spans[0].Status().Code)

	// Check http.response.status_code attribute
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

func TestOTelStatusMiddleware_5xxResponse_StatusError(t *testing.T) {
	spanRecorder, cleanup := setupTestTracer(t)
	defer cleanup()

	e := echo.New()
	tracer := otel.Tracer("test")

	req := httptest.NewRequest(http.MethodGet, "/api/error", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	ctx, span := tracer.Start(req.Context(), "test-span")
	c.SetRequest(req.WithContext(ctx))
	defer span.End()

	middleware := OTelStatusMiddleware()
	handler := func(c echo.Context) error {
		return c.String(http.StatusInternalServerError, "internal error")
	}

	err := middleware(handler)(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	span.End()

	// Check span status is Error for 5xx
	spans := spanRecorder.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Error, spans[0].Status().Code)
	assert.Equal(t, "Internal Server Error", spans[0].Status().Description)

	// Check http.response.status_code attribute
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

func TestOTelStatusMiddleware_5xxWithError_RecordsError(t *testing.T) {
	spanRecorder, cleanup := setupTestTracer(t)
	defer cleanup()

	e := echo.New()
	tracer := otel.Tracer("test")

	req := httptest.NewRequest(http.MethodGet, "/api/error", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	ctx, span := tracer.Start(req.Context(), "test-span")
	c.SetRequest(req.WithContext(ctx))
	defer span.End()

	testErr := errors.New("database connection failed")

	middleware := OTelStatusMiddleware()
	handler := func(c echo.Context) error {
		c.Response().WriteHeader(http.StatusInternalServerError)
		return testErr
	}

	err := middleware(handler)(c)
	assert.Equal(t, testErr, err)

	span.End()

	// Check span has recorded error event
	spans := spanRecorder.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Error, spans[0].Status().Code)

	// Check for error event
	events := spans[0].Events()
	var errorEventFound bool
	for _, event := range events {
		if event.Name == "exception" {
			errorEventFound = true
			break
		}
	}
	assert.True(t, errorEventFound, "exception event not found in span")
}

func TestOTelStatusMiddleware_NoSpanInContext(t *testing.T) {
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	middleware := OTelStatusMiddleware()
	handler := func(c echo.Context) error {
		return c.String(http.StatusOK, "success")
	}

	// Should not panic when no span in context
	err := middleware(handler)(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestOTelStatusMiddleware_ServiceUnavailable(t *testing.T) {
	spanRecorder, cleanup := setupTestTracer(t)
	defer cleanup()

	e := echo.New()
	tracer := otel.Tracer("test")

	req := httptest.NewRequest(http.MethodGet, "/api/busy", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	ctx, span := tracer.Start(req.Context(), "test-span")
	c.SetRequest(req.WithContext(ctx))
	defer span.End()

	middleware := OTelStatusMiddleware()
	handler := func(c echo.Context) error {
		return c.String(http.StatusServiceUnavailable, "service unavailable")
	}

	err := middleware(handler)(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	span.End()

	// Check span status is Error for 503
	spans := spanRecorder.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Error, spans[0].Status().Code)
	assert.Equal(t, "Service Unavailable", spans[0].Status().Description)
}
