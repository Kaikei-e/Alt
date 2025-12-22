package models

import (
	"time"
)

type Article struct {
	CreatedAt   time.Time `db:"created_at"`
	ID          string    `db:"id"`
	Title       string    `db:"title"`
	Content     string    `db:"content"`
	URL         string    `db:"url"`
	FeedID      string    `db:"feed_id"`
	UserID      string    `db:"user_id"`
	PublishedAt time.Time `db:"published_at"`
	InoreaderID string    `db:"inoreader_id"` // Transient field
	FeedURL     string    `db:"-"`            // Transient field for sync
}
