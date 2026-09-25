package rest_feeds

import (
	"alt/domain"
	"alt/orchestrator/rest/resterr"
	"fmt"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// HandleError converts errors to appropriate HTTP responses using enhanced error handling.
// IMPORTANT: This function ensures internal error details are NEVER exposed to clients.
// All error messages are sanitized using SafeMessage() before being returned.
func HandleError(c echo.Context, err error, operation string) error {
	return resterr.HandleFeedError(c, err, operation)
}

// HandleValidationError handles validation errors
func HandleValidationError(c echo.Context, message string, field string, value interface{}) error {
	return resterr.HandleValidationError(c, message, field, value)
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

// DeriveNextCursorFromFeeds extracts the next cursor from the feed list.
//
// The cursor is formatted with sub-second precision because it comes back as
// the right-hand side of a strict `created_at < $1`: created_at is microsecond
// precision and one harvester transaction stamps a whole batch inside the same
// second, so a cursor truncated to the second skips every remaining row of
// that second permanently.
func DeriveNextCursorFromFeeds(feeds []*domain.FeedItem) (string, bool) {
	if len(feeds) == 0 {
		return "", false
	}
	lastFeed := feeds[len(feeds)-1]
	if !lastFeed.PublishedParsed.IsZero() {
		return lastFeed.PublishedParsed.Format(time.RFC3339Nano), true
	}

	published := strings.TrimSpace(lastFeed.Published)
	if published == "" {
		return "", false
	}

	parsed, err := time.Parse(time.RFC3339, published)
	if err != nil {
		return "", false
	}

	return parsed.Format(time.RFC3339Nano), true
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
