package domain

import "time"

// FeedTag represents a tag associated with a feed's articles
type FeedTag struct {
	ID         string    `json:"id"`
	FeedID     string    `json:"feed_id"`
	TagName    string    `json:"tag_name"`
	Confidence float64   `json:"confidence"`
	TagType    string    `json:"tag_type"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
