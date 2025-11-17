package feed_link_port

import (
	"alt/domain"
	"context"

	"github.com/google/uuid"
)

// FeedLinkPort defines operations for managing registered feed URLs.
type FeedLinkPort interface {
	ListFeedLinks(ctx context.Context) ([]*domain.FeedLink, error)
	DeleteFeedLink(ctx context.Context, id uuid.UUID) error
}
