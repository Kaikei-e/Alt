package rest

import (
	"alt/domain"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/microcosm-cc/bluemonday"
)

// OptimizeFeedsResponse transforms domain feeds into a client-optimized structure
func OptimizeFeedsResponse(feeds []*domain.FeedItem) []map[string]interface{} {
	optimized := make([]map[string]interface{}, 0, len(feeds))
	for _, feed := range feeds {
		optimized = append(optimized, map[string]interface{}{
			"id":          feed.Link, // Use Link as ID for consistency with frontend
			"title":       feed.Title,
			"description": sanitizeDescription(feed.Description),
			"link":        feed.Link,
			"published":   formatTimeAgo(feed.PublishedParsed),
			"created_at":  feed.PublishedParsed.Format(time.RFC3339),
			"author":      formatAuthor(feed.Author, feed.Authors),
		})
	}
	return optimized
}

// sanitizeDescription removes HTML tags, specifically ensuring <img> tags are removed.
// It returns plain text.
func sanitizeDescription(html string) string {
	if html == "" {
		return ""
	}

	// Use bluemonday for secure sanitization
	// UGCPolicy allows a broad selection of HTML elements that are safe for user generated content.
	// However, we want to strip ALL tags to get plain text, or at least strip <img> tags.
	// The user request was "remove <img> tags", but the previous regex was removing ALL tags.
	// If we want plain text, we should use StrictPolicy().
	// If we want to keep some formatting but remove images, we can use UGCPolicy() and disallow "img".

	// Given the context of "optimizing payload" and "plain text" for the feed description in the swipe view,
	// usually we want to strip everything or keep minimal formatting like <p>, <br>.
	// The previous regex `<[^>]*>` stripped EVERYTHING.
	// So StrictPolicy() is the closest equivalent but secure.

	p := bluemonday.StrictPolicy()
	text := p.Sanitize(html)

	// Trimming whitespace
	text = strings.TrimSpace(text)

	// Collapse multiple spaces
	spaceRe := regexp.MustCompile(`\s+`)
	text = spaceRe.ReplaceAllString(text, " ")

	return text
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
		// We don't have logger here easily unless we import it, but we can ignore error or return false
		// For now let's just return false if parsing fails
		return "", false
	}

	return parsed.Format(time.RFC3339), true
}
