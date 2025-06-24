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
			ctx := req.Context()

			// Log request start
			contextLogger.WithContext(ctx).Info("request started",
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

			// Determine log level based on status code
			logFunc := contextLogger.WithContext(ctx).Info
			if status >= 400 && status < 500 {
				logFunc = contextLogger.WithContext(ctx).Warn
			} else if status >= 500 {
				logFunc = contextLogger.WithContext(ctx).Error
			}

			// Log request completion
			logFunc("request completed",
				"method", req.Method,
				"path", req.URL.Path,
				"status", status,
				"duration_ms", duration.Milliseconds(),
				"response_size", size,
			)

			// Log error if present
			if err != nil {
				contextLogger.WithContext(ctx).Error("request error",
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