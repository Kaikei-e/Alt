package domain

import "time"

// FeedTag represents a tag associated with a feed's articles
type FeedTag struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}