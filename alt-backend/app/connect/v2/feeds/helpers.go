// Package feeds contains helper functions for FeedService handlers.
package feeds

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/microcosm-cc/bluemonday"

	"alt/domain"
	feedsv2 "alt/gen/proto/alt/feeds/v2"
)

// convertFeedsToProto converts domain FeedItems to proto FeedItems.
func convertFeedsToProto(feeds []*domain.FeedItem) []*feedsv2.FeedItem {
	result := make([]*feedsv2.FeedItem, 0, len(feeds))
	for _, feed := range feeds {
		// ID for Svelte {#each} keying only (not used in business logic).
		// Use ArticleID when available; otherwise generate a UUID to guarantee uniqueness.
		id := uuid.New().String()
		if feed.ArticleID != "" {
			id = feed.ArticleID
		}
		item := &feedsv2.FeedItem{
			Id:          id,
			Title:       feed.Title,
			Description: sanitizeDescription(feed.Description),
			Link:        feed.Link,
			Published:   formatTimeAgo(feed.PublishedParsed),
			CreatedAt:   feed.PublishedParsed.Format(time.RFC3339),
			Author:      formatAuthor(feed.Author, feed.Authors),
			IsRead:      feed.IsRead,
		}

		// Set ArticleId if article exists in database
		// When empty, mark as read functionality should be disabled on frontend
		if feed.ArticleID != "" {
			item.ArticleId = &feed.ArticleID
		}

		result = append(result, item)
	}
	return result
}

// deriveNextCursor extracts the next cursor from the feed list.
func deriveNextCursor(feeds []*domain.FeedItem, hasMore bool) *string {
	if !hasMore || len(feeds) == 0 {
		return nil
	}
	lastFeed := feeds[len(feeds)-1]
	if !lastFeed.PublishedParsed.IsZero() {
		cursor := lastFeed.PublishedParsed.Format(time.RFC3339)
		return &cursor
	}

	published := strings.TrimSpace(lastFeed.Published)
	if published == "" {
		return nil
	}

	parsed, err := time.Parse(time.RFC3339, published)
	if err != nil {
		return nil
	}

	cursor := parsed.Format(time.RFC3339)
	return &cursor
}

// spaceCollapseRe pre-compiles the whitespace collapsing regex.
var spaceCollapseRe = regexp.MustCompile(`\s+`)

// sanitizeDescription removes HTML tags, decodes HTML entities, and returns plain text.
func sanitizeDescription(rawHTML string) string {
	if rawHTML == "" {
		return ""
	}

	p := bluemonday.StrictPolicy()
	text := p.Sanitize(rawHTML)

	// Decode HTML entities (e.g. &#39; -> ', &amp; -> &)
	text = html.UnescapeString(text)

	// Trimming whitespace
	text = strings.TrimSpace(text)

	// Collapse multiple spaces
	text = spaceCollapseRe.ReplaceAllString(text, " ")

	return text
}

// formatTimeAgo formats the time as a relative string (e.g., "2 hours ago").
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

// formatAuthor extracts the author name from Author or Authors.
func formatAuthor(author domain.Author, authors []domain.Author) string {
	if author.Name != "" {
		return author.Name
	}
	if len(authors) > 0 && authors[0].Name != "" {
		return authors[0].Name
	}
	return ""
}

// parseSSESummary extracts the summary text from SSE-formatted data.
func parseSSESummary(sseData string) string {
	if !strings.Contains(sseData, "data:") {
		return sseData
	}

	var result strings.Builder
	lines := strings.Split(sseData, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			dataContent := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			result.WriteString(dataContent)
		}
	}

	return result.String()
}
