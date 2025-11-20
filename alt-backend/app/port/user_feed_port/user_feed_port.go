package user_feed_port

import (
	"context"

	"github.com/google/uuid"
)

//go:generate mockgen -source=user_feed_port.go -destination=../../mocks/mock_user_feed_port.go -package=mocks

// UserFeedPort defines the interface for accessing user feed data.
type UserFeedPort interface {
	// GetUserFeedIDs returns the feed IDs that the user is subscribed to.
	// User ID is extracted from the context.
	GetUserFeedIDs(ctx context.Context) ([]uuid.UUID, error)
}
