// ABOUTME: Centralized error handling middleware for Echo framework
// ABOUTME: Converts AppContextError to secure HTTP responses, hides internal details
package middleware

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v5"

	apperrors "pre-processor/utils/errors"
)

// CustomHTTPErrorHandler creates the centralized HTTP error handler for Echo.
// It converts various error types to consistent, secure HTTP responses.
//
// Error handling priority:
// 1. AppContextError - uses ToSecureHTTPResponse() for consistent format
// 2. *echo.HTTPError / echo.HTTPStatusCoder - preserves Echo's error format for backward compatibility
// 3. Unknown errors - returns generic 500 response to hide internal details
func CustomHTTPErrorHandler(logger *slog.Logger) echo.HTTPErrorHandler {
	return func(c *echo.Context, err error) {
		// Don't write to already committed responses
		if resp, uErr := echo.UnwrapResponse(c.Response()); uErr == nil && resp.Committed {
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
		var appErr *apperrors.AppContextError
		var httpErr *echo.HTTPError
		var statusCoder echo.HTTPStatusCoder
		switch {
		case errors.As(err, &appErr):
			status = appErr.HTTPStatusCode()
			response = appErr.ToSecureHTTPResponse()

			// Log full error details for internal debugging
			logger.Error("application error",
				"request_id", requestID,
				"error_id", appErr.ErrorID,
				"code", appErr.Code,
				"message", appErr.Message,
				"layer", appErr.Layer,
				"component", appErr.Component,
				"operation", appErr.Operation,
				"cause", appErr.Cause,
				"context", appErr.Context,
			)

		case errors.As(err, &httpErr):
			status = httpErr.Code
			msg := httpErr.Message
			if msg == "" {
				msg = "An error occurred"
			}
			response = buildHTTPErrorResponse(logger, requestID, status, msg, msg)

		case errors.As(err, &statusCoder) && statusCoder.StatusCode() != 0:
			status = statusCoder.StatusCode()
			response = buildHTTPErrorResponse(logger, requestID, status, http.StatusText(status), err.Error())

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

func buildHTTPErrorResponse(logger *slog.Logger, requestID string, status int, clientMsg, logMsg string) apperrors.SecureHTTPResponse {
	// For 5xx errors, hide the actual message
	safeMsg := clientMsg
	if status >= 500 {
		safeMsg = "An unexpected error occurred. Please try again later."
	}

	logger.Warn("HTTP error",
		"request_id", requestID,
		"status", status,
		"message", logMsg,
	)

	return apperrors.SecureHTTPResponse{
		Error: apperrors.SecureErrorDetail{
			Code:      "HTTP_ERROR",
			Message:   safeMsg,
			Retryable: apperrors.IsRetryableHTTPStatus(status),
		},
	}
}
