package middleware

import (
	"alt/utils/logger"
	"context"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

func RequestIDMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			requestID := c.Request().Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = uuid.New().String()
			}

			// Add to response header
			c.Response().Header().Set("X-Request-ID", requestID)

			// Add to context
			ctx := context.WithValue(c.Request().Context(), logger.RequestIDKey, requestID)
			c.SetRequest(c.Request().WithContext(ctx))

			return next(c)
		}
	}
}