package domain

import "github.com/google/uuid"

// FeedLink represents a registered RSS source that can be managed by the user.
type FeedLink struct {
	ID  uuid.UUID `json:"id"`
	URL string    `json:"url"`
}
