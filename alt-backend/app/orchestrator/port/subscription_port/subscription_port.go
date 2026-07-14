package subscription_port

import (
	"alt/domain"
	"context"

	"github.com/google/uuid"
)

// SubscriptionPort defines operations for managing user feed subscriptions.
type SubscriptionPort interface {
	ListSubscriptions(ctx context.Context, userID uuid.UUID) ([]*domain.FeedSource, error)
	Subscribe(ctx context.Context, userID uuid.UUID, feedLinkID uuid.UUID) error
	Unsubscribe(ctx context.Context, userID uuid.UUID, feedLinkID uuid.UUID) error
}
