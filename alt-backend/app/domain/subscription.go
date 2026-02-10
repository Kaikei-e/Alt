package domain

import "time"

// FeedSource represents a feed link with its subscription status for a user.
type FeedSource struct {
	ID           string
	URL          string
	Title        string
	IsSubscribed bool
	CreatedAt    time.Time
}
