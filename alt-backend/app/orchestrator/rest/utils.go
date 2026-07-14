package rest

import (
	"alt/domain"
	"alt/utils/errors"
	"alt/utils/logger"
	"alt/utils/url_validator"
	stderrors "errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v4"
)

// HandleError converts errors to appropriate HTTP responses using enhanced error handling.
// IMPORTANT: This function ensures internal error details are NEVER exposed to clients.
// All error messages are sanitized using SafeMessage() before being returned.
func HandleError(c echo.Context, err error, operation string) error {
	// Enrich error with REST layer context
	var enrichedErr *errors.AppContextError

	// Check if it's already an AppContextError and enrich it with REST context
	var appContextErr *errors.AppContextError
	var appErr *errors.AppError
	if stderrors.As(err, &appContextErr) {
		enrichedErr = errors.EnrichWithContext(
			appContextErr,
			"rest",
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
		enrichedErr = errors.NewAppContextError(
			string(appErr.Code),
			appErr.Message,
			"rest",
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
	} else {
		// Handle unknown errors
		enrichedErr = errors.NewUnknownContextError(
			"internal server error",
			"rest",
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

// handleValidationError handles validation errors
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

// IsAllowedURL checks if the URL is allowed (not private IP).
// Deprecated: Use utils/url_validator.IsAllowedURL directly.
func IsAllowedURL(u *url.URL) error {
	return url_validator.IsAllowedURL(u)
}

// Optimize feeds response specifically for search results
// Note: Description is NOT truncated here to allow full text display in Search Feeds page
func OptimizeFeedsResponseForSearch(feeds []*domain.FeedItem) []*domain.FeedItem {
	for _, feed := range feeds {
		feed.Title = strings.TrimSpace(feed.Title)
		// Description is kept full-length for search results to support "Read more" functionality
		// Only trim whitespace, do not truncate content
		feed.Description = strings.TrimSpace(feed.Description)
	}
	return feeds
}

// Determine cache age based on limit to optimize caching strategy
func GetCacheAgeForLimit(limit int) int {
	switch {
	case limit <= 10:
		return 60 // 1 minute for small limits
	case limit <= 50:
		return 300 // 5 minutes for medium limits
	default:
		return 600 // 10 minutes for large limits
	}
}
