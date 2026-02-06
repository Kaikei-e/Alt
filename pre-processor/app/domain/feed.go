package domain

import (
	"time"
)

// Feed represents an RSS feed entry.
type Feed struct {
	CreatedAt time.Time `db:"created_at"`
	ID        string    `db:"id"`
	Link      string    `db:"link"`
	Title     string    `db:"title"`
}

// ProcessingStatistics represents processing statistics.
type ProcessingStatistics struct {
	TotalFeeds     int `json:"total_feeds"`
	ProcessedFeeds int `json:"processed_feeds"`
	RemainingFeeds int `json:"remaining_feeds"`
}
