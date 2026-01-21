// ABOUTME: This file provides OpenTelemetry span status middleware
// ABOUTME: Sets span status based on HTTP response codes per OTel semantic conventions
package middleware

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// OTelStatusMiddleware sets span status and HTTP attributes based on response.
// It follows the OpenTelemetry HTTP semantic conventions:
// - 1xx, 2xx, 3xx, 4xx: StatusCode = Unset (normal operation or client error)
// - 5xx: StatusCode = Error (server error)
//
// This middleware should be used AFTER otelecho.Middleware which creates the span.
func OTelStatusMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Execute the handler
			err := next(c)

			// Get current span from context
			span := trace.SpanFromContext(c.Request().Context())
			if !span.SpanContext().IsValid() {
				return err
			}

			// Get response status
			status := c.Response().Status

			// Set HTTP semantic convention attributes
			span.SetAttributes(
				semconv.HTTPResponseStatusCode(status),
			)

			// Set span status based on HTTP status code (OTel spec)
			// - 5xx: Error
			// - 4xx (server): Unset (default)
			// - 1xx, 2xx, 3xx: Unset (default)
			if status >= 500 {
				span.SetStatus(codes.Error, http.StatusText(status))
				if err != nil {
					span.RecordError(err)
				}
			}

			return err
		}
	}
}
