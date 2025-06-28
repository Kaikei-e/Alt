// ABOUTME: This file provides HTTP request/response logging middleware
// ABOUTME: Creates rask-compatible access logs with timing and context information
package middleware

import (
	"time"

	"github.com/labstack/echo/v4"

	"pre-processor/utils/logger"
)

func LoggingMiddleware(contextLogger *logger.ContextLogger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			res := c.Response()

			start := time.Now()

			// Add operation context for this request
			ctx := logger.WithOperation(req.Context(), req.Method+" "+req.URL.Path)
			c.SetRequest(req.WithContext(ctx))

			// Create rask-compatible logger with HTTP fields
			log := contextLogger.WithContext(ctx).With(
				// Rask HTTP fields
				"method", req.Method,
				"path", req.URL.Path,
				"ip_address", c.RealIP(),
				"user_agent", req.UserAgent(),
				// Additional fields
				"fields.duration_ms", "", // Will be updated on completion
			)

			// Log request start
			log.Info("request started")

			// Process request
			err := next(c)

			// Calculate duration
			duration := time.Since(start)

			// Log request completion with updated fields
			completionLog := contextLogger.WithContext(ctx).With(
				"log_type", "access", // Change to access log type
				"method", req.Method,
				"path", req.URL.Path,
				"status_code", res.Status,
				"response_size", res.Size,
				"ip_address", c.RealIP(),
				"user_agent", req.UserAgent(),
				"fields.duration_ms", duration.Milliseconds(),
			)

			completionLog.Info("request completed")

			return err
		}
	}
}
