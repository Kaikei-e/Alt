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

func TestOTelStatusMiddleware_2xxResponse_StatusUnset(t *testing.T) {
	spanRecorder, cleanup := setupTestTracer(t)
	defer cleanup()

	e := echo.New()
	tracer := otel.Tracer("test")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	ctx, span := tracer.Start(req.Context(), "test-span")
	c.SetRequest(req.WithContext(ctx))
	defer span.End()

	middleware := OTelStatusMiddleware()
	handler := func(c echo.Context) error {
		return c.String(http.StatusOK, "healthy")
	}

	err := middleware(handler)(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	span.End()

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

func TestOTelStatusMiddleware_4xxResponse_StatusUnset(t *testing.T) {
	spanRecorder, cleanup := setupTestTracer(t)
	defer cleanup()

	e := echo.New()
	tracer := otel.Tracer("test")

	req := httptest.NewRequest(http.MethodGet, "/validate", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	ctx, span := tracer.Start(req.Context(), "test-span")
	c.SetRequest(req.WithContext(ctx))
	defer span.End()

	middleware := OTelStatusMiddleware()
	handler := func(c echo.Context) error {
		return c.String(http.StatusUnauthorized, "unauthorized")
	}

	err := middleware(handler)(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	span.End()

	spans := spanRecorder.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Unset, spans[0].Status().Code)

	attrs := spans[0].Attributes()
	var statusCodeFound bool
	for _, attr := range attrs {
		if string(attr.Key) == "http.response.status_code" {
			statusCodeFound = true
			assert.Equal(t, int64(401), attr.Value.AsInt64())
		}
	}
	assert.True(t, statusCodeFound, "http.response.status_code attribute not found")
}

func TestOTelStatusMiddleware_5xxResponse_StatusError(t *testing.T) {
	spanRecorder, cleanup := setupTestTracer(t)
	defer cleanup()

	e := echo.New()
	tracer := otel.Tracer("test")

	req := httptest.NewRequest(http.MethodGet, "/validate", nil)
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

func TestOTelStatusMiddleware_5xxWithError_RecordsError(t *testing.T) {
	spanRecorder, cleanup := setupTestTracer(t)
	defer cleanup()

	e := echo.New()
	tracer := otel.Tracer("test")

	req := httptest.NewRequest(http.MethodGet, "/validate", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	ctx, span := tracer.Start(req.Context(), "test-span")
	c.SetRequest(req.WithContext(ctx))
	defer span.End()

	testErr := errors.New("kratos connection failed")

	middleware := OTelStatusMiddleware()
	handler := func(c echo.Context) error {
		c.Response().WriteHeader(http.StatusInternalServerError)
		return testErr
	}

	err := middleware(handler)(c)
	assert.Equal(t, testErr, err)

	span.End()

	spans := spanRecorder.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Error, spans[0].Status().Code)

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

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	middleware := OTelStatusMiddleware()
	handler := func(c echo.Context) error {
		return c.String(http.StatusOK, "healthy")
	}

	err := middleware(handler)(c)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
}
