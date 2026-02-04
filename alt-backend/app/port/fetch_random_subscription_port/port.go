package fetch_random_subscription_port

import (
	"alt/domain"
	"context"
)

// FetchRandomSubscriptionPort defines the interface for fetching a random feed.
// Returns a Feed with title, description, and link for the Tag Trail feature.
type FetchRandomSubscriptionPort interface {
	FetchRandomSubscription(ctx context.Context) (*domain.Feed, error)
}
