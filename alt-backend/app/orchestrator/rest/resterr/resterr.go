package resterr

import (
	"alt/utils/errors"
	"alt/utils/logger"
	stderrors "errors"
	"net/http"

	"github.com/labstack/echo/v4"
)

// BuildAppContextError enriches an error with REST layer context and request metadata.
func BuildAppContextError(c echo.Context, err error, layer string, operation string) *errors.AppContextError {
	var appContextErr *errors.AppContextError
	var appErr *errors.AppError
	if stderrors.As(err, &appContextErr) {
		return errors.EnrichWithContext(
			appContextErr,
			layer,
			"RESTHandler",
			operation,
			map[string]interface{}{
				"path":        c.Request().URL.Path,
				"method":      c.Request().Method,
				"remote_addr": c.Request().RemoteAddr,
				"user_agent":  c.Request().UserAgent(),
				"request_id":  c.Response().Header().Get("X-Request-ID"),
			},
		)
	} else if stderrors.As(err, &appErr) {
		// Handle legacy AppError by converting to AppContextError
		return errors.NewAppContextError(
			string(appErr.Code),
			appErr.Message,
			layer,
			"RESTHandler",
			operation,
			appErr.Cause,
			map[string]interface{}{
				"path":           c.Request().URL.Path,
				"method":         c.Request().Method,
				"remote_addr":    c.Request().RemoteAddr,
				"user_agent":     c.Request().UserAgent(),
				"request_id":     c.Response().Header().Get("X-Request-ID"),
				"legacy_context": appErr.Context,
			},
		)
	}

	// Handle unknown errors
	return errors.NewUnknownContextError(
		"internal server error",
		layer,
		"RESTHandler",
		operation,
		err,
		map[string]interface{}{
			"path":        c.Request().URL.Path,
			"method":      c.Request().Method,
			"remote_addr": c.Request().RemoteAddr,
			"user_agent":  c.Request().UserAgent(),
			"request_id":  c.Response().Header().Get("X-Request-ID"),
		},
	)
}

// HandleErrorWithLayer converts errors to appropriate HTTP responses using enhanced error handling for a given layer.
func HandleErrorWithLayer(c echo.Context, err error, layer string, operation string) error {
	enrichedErr := BuildAppContextError(c, err, layer, operation)

	// Log the full error details (internal only - never sent to client)
	ctx := c.Request().Context()
	logger.Logger.ErrorContext(ctx,
		"REST API Error",
		"error_id", enrichedErr.ErrorID,
		"error", enrichedErr.Error(),
		"code", enrichedErr.Code,
		"operation", operation,
		"path", c.Request().URL.Path,
	)

	// Return secure JSON response (SafeMessage() ensures no internal details leak)
	return c.JSON(enrichedErr.HTTPStatusCode(), enrichedErr.ToSecureHTTPResponse())
}

// HandleError converts errors to appropriate HTTP responses using enhanced error handling for the "rest" layer.
// IMPORTANT: This function ensures internal error details are NEVER exposed to clients.
// All error messages are sanitized using SafeMessage() before being returned.
func HandleError(c echo.Context, err error, operation string) error {
	return HandleErrorWithLayer(c, err, "rest", operation)
}

// HandleFeedError converts errors to appropriate HTTP responses using enhanced error handling for the "rest_feeds" layer.
// IMPORTANT: This function ensures internal error details are NEVER exposed to clients.
// All error messages are sanitized using SafeMessage() before being returned.
func HandleFeedError(c echo.Context, err error, operation string) error {
	return HandleErrorWithLayer(c, err, "rest_feeds", operation)
}

// HandleValidationError handles validation errors
func HandleValidationError(c echo.Context, message string, field string, value interface{}) error {
	ctx := c.Request().Context()
	logger.Logger.WarnContext(ctx, "Validation error", "message", message, "field", field, "value", value)
	return c.JSON(http.StatusBadRequest, map[string]interface{}{
		"error": message,
		"field": field,
		"value": value,
		"code":  "VALIDATION_ERROR",
	})
}
