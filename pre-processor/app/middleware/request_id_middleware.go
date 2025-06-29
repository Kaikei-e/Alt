// ABOUTME: This file provides request ID middleware for HTTP request tracing
// ABOUTME: Generates or extracts request IDs from headers for context propagation
package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/labstack/echo/v4"

	"pre-processor/utils/logger"
)

func RequestIDMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()

			// Extract or generate request ID
			requestID := req.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = generateRequestID()
			}

			// Add to context
			ctx := logger.WithRequestID(req.Context(), requestID)
			c.SetRequest(req.WithContext(ctx))

			// Add to response headers
			c.Response().Header().Set("X-Request-ID", requestID)

			return next(c)
		}
	}
}

func generateRequestID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		logger.Logger.Error("failed to generate request ID", "error", err)
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}
