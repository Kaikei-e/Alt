package middleware

import (
	"alt/utils/logger"
	"log/slog"
	"time"

	"github.com/labstack/echo/v4"
)

func LoggingMiddleware(baseLogger *slog.Logger) echo.MiddlewareFunc {
	contextLogger := logger.NewContextLogger(baseLogger)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()

			req := c.Request()

			// Skip logging for health check endpoint to reduce noise
			if req.URL.Path == "/v1/health" {
				return next(c)
			}
			ctx := req.Context()

			// Log request start (use InfoContext to propagate trace context)
			contextLogger.WithContext(ctx).InfoContext(ctx, "request started",
				"method", req.Method,
				"path", req.URL.Path,
				"remote_addr", c.RealIP(),
				"user_agent", req.UserAgent(),
			)

			// Execute the next handler
			err := next(c)

			// Calculate duration
			duration := time.Since(start)

			// Get response information
			res := c.Response()
			status := res.Status
			size := res.Size

			// Log request completion with appropriate level (use *Context to propagate trace context)
			logAttrs := []any{
				"method", req.Method,
				"path", req.URL.Path,
				"status", status,
				"duration_ms", duration.Milliseconds(),
				"response_size", size,
			}
			if status >= 500 {
				contextLogger.WithContext(ctx).ErrorContext(ctx, "request completed", logAttrs...)
			} else if status >= 400 {
				contextLogger.WithContext(ctx).WarnContext(ctx, "request completed", logAttrs...)
			} else {
				contextLogger.WithContext(ctx).InfoContext(ctx, "request completed", logAttrs...)
			}

			// Log error if present (use ErrorContext to propagate trace context)
			if err != nil {
				contextLogger.WithContext(ctx).ErrorContext(ctx, "request error",
					"method", req.Method,
					"path", req.URL.Path,
					"error", err,
					"duration_ms", duration.Milliseconds(),
				)
			}

			return err
		}
	}
}
