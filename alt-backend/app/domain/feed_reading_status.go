package domain

import "time"

type FeedReadingStatus struct {
	FeedURL string    `db:"feed_url"`
	IsRead  bool      `db:"is_read"`
	ReadAt  time.Time `db:"read_at"`
}
