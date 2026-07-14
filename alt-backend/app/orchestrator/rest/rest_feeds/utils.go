package rest_feeds

import (
	"alt/domain"
	"alt/utils/errors"
	"alt/utils/logger"
	"alt/utils/url_validator"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// HandleError converts errors to appropriate HTTP responses using enhanced error handling.
// IMPORTANT: This function ensures internal error details are NEVER exposed to clients.
// All error messages are sanitized using SafeMessage() before being returned.
func HandleError(c echo.Context, err error, operation string) error {
	// Enrich error with REST layer context
	var enrichedErr *errors.AppContextError

	// Check if it's already an AppContextError and enrich it with REST context
	if appContextErr, ok := err.(*errors.AppContextError); ok {
		enrichedErr = errors.EnrichWithContext(
			appContextErr,
			"rest_feeds",
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
	} else if appErr, ok := err.(*errors.AppError); ok {
		// Handle legacy AppError by converting to AppContextError
		enrichedErr = errors.NewAppContextError(
			string(appErr.Code),
			appErr.Message,
			"rest_feeds",
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
			"rest_feeds",
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

// IsAllowedURL checks if the URL is allowed (not private IP)
// IsAllowedURL checks if the URL is allowed (not private IP).
// Deprecated: Use utils/url_validator.IsAllowedURL directly.
func IsAllowedURL(u *url.URL) error {
	return url_validator.IsAllowedURL(u)
}

// OptimizeFeedsResponse transforms domain feeds into a client-optimized structure
func OptimizeFeedsResponse(feeds []*domain.FeedItem) []map[string]interface{} {
	optimized := make([]map[string]interface{}, 0, len(feeds))
	for _, feed := range feeds {
		optimized = append(optimized, map[string]interface{}{
			"id":          feed.Link, // domain.FeedItem keeps RSS-spec Link
			"title":       feed.Title,
			"description": feed.Description,
			"link":        feed.Link,
			"published":   formatTimeAgo(feed.PublishedParsed),
			"created_at":  feed.PublishedParsed.Format(time.RFC3339),
			"author":      formatAuthor(feed.Author, feed.Authors),
		})
	}
	return optimized
}

// formatTimeAgo formats the time as a relative string (e.g., "2 hours ago")
// or a date string if it's older.
func formatTimeAgo(t time.Time) string {
	if t.IsZero() {
		return ""
	}

	now := time.Now()
	diff := now.Sub(t)

	// If future (clock skew), treat as just now
	if diff < 0 {
		return "Just now"
	}

	if diff < time.Minute {
		return "Just now"
	}
	if diff < time.Hour {
		minutes := int(diff.Minutes())
		return fmt.Sprintf("%dm ago", minutes)
	}
	if diff < 24*time.Hour {
		hours := int(diff.Hours())
		return fmt.Sprintf("%dh ago", hours)
	}
	if diff < 48*time.Hour {
		return "Yesterday"
	}
	if diff < 7*24*time.Hour {
		days := int(diff.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	}

	// Older than a week, return YYYY/MM/DD
	return t.Format("2006/01/02")
}

func formatAuthor(author domain.Author, authors []domain.Author) string {
	if author.Name != "" {
		return author.Name
	}
	if len(authors) > 0 && authors[0].Name != "" {
		return authors[0].Name
	}
	return ""
}

// DeriveNextCursorFromFeeds extracts the next cursor from the feed list
func DeriveNextCursorFromFeeds(feeds []*domain.FeedItem) (string, bool) {
	if len(feeds) == 0 {
		return "", false
	}
	lastFeed := feeds[len(feeds)-1]
	if !lastFeed.PublishedParsed.IsZero() {
		return lastFeed.PublishedParsed.Format(time.RFC3339), true
	}

	published := strings.TrimSpace(lastFeed.Published)
	if published == "" {
		return "", false
	}

	parsed, err := time.Parse(time.RFC3339, published)
	if err != nil {
		return "", false
	}

	return parsed.Format(time.RFC3339), true
}

// OptimizeFeedsResponseForSearch optimizes feeds response specifically for search results
func OptimizeFeedsResponseForSearch(feeds []*domain.FeedItem) []*domain.FeedItem {
	for _, feed := range feeds {
		feed.Title = strings.TrimSpace(feed.Title)
		// Description is kept full-length for search results to support "Read more" functionality
		// Only trim whitespace, do not truncate content
		feed.Description = strings.TrimSpace(feed.Description)
	}
	return feeds
}

// GetCacheAgeForLimit determines cache age based on limit to optimize caching strategy
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
