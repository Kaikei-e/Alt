package domain

import "time"

// TagTrailArticle represents an article in the Tag Trail feature with minimal data for display.
type TagTrailArticle struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Link        string    `json:"link"`
	PublishedAt time.Time `json:"published_at"`
	FeedID      string    `json:"feed_id,omitempty"`
	FeedTitle   string    `json:"feed_title,omitempty"`
}
