// ABOUTME: Centralized error handling middleware for Echo framework
// ABOUTME: Converts AppContextError to secure HTTP responses, hides internal details
package middleware

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"

	apperrors "pre-processor/utils/errors"
)

// CustomHTTPErrorHandler creates the centralized HTTP error handler for Echo.
// It converts various error types to consistent, secure HTTP responses.
//
// Error handling priority:
// 1. AppContextError - uses ToSecureHTTPResponse() for consistent format
// 2. echo.HTTPError - preserves Echo's error format for backward compatibility
// 3. Unknown errors - returns generic 500 response to hide internal details
func CustomHTTPErrorHandler(logger *slog.Logger) echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		// Don't write to already committed responses
		if c.Response().Committed {
			return
		}

		ctx := c.Request().Context()
		requestID := ""
		if rid := ctx.Value("request_id"); rid != nil {
			if s, ok := rid.(string); ok {
				requestID = s
			}
		}

		var response apperrors.SecureHTTPResponse
		var status int

		// Handle different error types
		switch e := err.(type) {
		case *apperrors.AppContextError:
			status = e.HTTPStatusCode()
			response = e.ToSecureHTTPResponse()

			// Log full error details for internal debugging
			logger.Error("application error",
				"request_id", requestID,
				"error_id", e.ErrorID,
				"code", e.Code,
				"message", e.Message,
				"layer", e.Layer,
				"component", e.Component,
				"operation", e.Operation,
				"cause", e.Cause,
				"context", e.Context,
			)

		case *echo.HTTPError:
			status = e.Code
			msg := "An error occurred"
			if m, ok := e.Message.(string); ok {
				msg = m
			}

			// For 5xx errors, hide the actual message
			safeMsg := msg
			if status >= 500 {
				safeMsg = "An unexpected error occurred. Please try again later."
			}

			response = apperrors.SecureHTTPResponse{
				Error: apperrors.SecureErrorDetail{
					Code:      "HTTP_ERROR",
					Message:   safeMsg,
					Retryable: apperrors.IsRetryableHTTPStatus(status),
				},
			}

			logger.Warn("HTTP error",
				"request_id", requestID,
				"status", status,
				"message", msg,
			)

		default:
			// Unknown error type - treat as internal error
			status = http.StatusInternalServerError
			response = apperrors.SecureHTTPResponse{
				Error: apperrors.SecureErrorDetail{
					Code:      "INTERNAL_ERROR",
					Message:   "An unexpected error occurred. Please try again later.",
					Retryable: false,
				},
			}

			// Log the actual error for debugging (never expose to client)
			logger.Error("unhandled error",
				"request_id", requestID,
				"error", err.Error(),
				"error_type", err,
			)
		}

		// Send JSON response
		if err := c.JSON(status, response); err != nil {
			logger.Error("failed to send error response",
				"request_id", requestID,
				"error", err,
			)
		}
	}
}
