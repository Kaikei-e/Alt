package middleware

import (
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// statusRecorder wraps http.ResponseWriter to capture the status code
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func newStatusRecorder(w http.ResponseWriter) *statusRecorder {
	return &statusRecorder{
		ResponseWriter: w,
		status:         http.StatusOK, // Default status
	}
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.wroteHeader = true
	}
	return r.ResponseWriter.Write(b)
}

// OTelStatusHandler wraps an http.Handler with OpenTelemetry tracing
// and sets span status based on HTTP response code.
// It follows the OpenTelemetry HTTP semantic conventions:
// - 1xx, 2xx, 3xx, 4xx: StatusCode = Unset (normal operation or client error)
// - 5xx: StatusCode = Error (server error)
func OTelStatusHandler(handler http.Handler, operationName string) http.Handler {
	tracer := otel.Tracer("search-indexer")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Start a new span
		ctx, span := tracer.Start(r.Context(), operationName)
		defer span.End()

		// Wrap response writer to capture status code
		rec := newStatusRecorder(w)

		// Execute the handler with the span context
		handler.ServeHTTP(rec, r.WithContext(ctx))

		// Set HTTP semantic convention attributes
		span.SetAttributes(
			semconv.HTTPResponseStatusCode(rec.status),
		)

		// Set span status based on HTTP status code (OTel spec)
		// - 5xx: Error
		// - 4xx (server): Unset (default)
		// - 1xx, 2xx, 3xx: Unset (default)
		if rec.status >= 500 {
			span.SetStatus(codes.Error, http.StatusText(rec.status))
		}
	})
}

// OTelStatusHandlerFunc is a convenience function for wrapping http.HandlerFunc
func OTelStatusHandlerFunc(handlerFunc http.HandlerFunc, operationName string) http.Handler {
	return OTelStatusHandler(handlerFunc, operationName)
}

// WithOTelStatus is a middleware function that adds OTel status to existing spans.
// Use this when spans are created by another mechanism (e.g., otelhttp).
func WithOTelStatus(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wrap response writer to capture status code
		rec := newStatusRecorder(w)

		// Execute the handler
		handler.ServeHTTP(rec, r)

		// Get current span from context
		span := trace.SpanFromContext(r.Context())
		if !span.SpanContext().IsValid() {
			return
		}

		// Set HTTP semantic convention attributes
		span.SetAttributes(
			semconv.HTTPResponseStatusCode(rec.status),
		)

		// Set span status based on HTTP status code (OTel spec)
		if rec.status >= 500 {
			span.SetStatus(codes.Error, http.StatusText(rec.status))
		}
	})
}
