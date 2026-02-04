package fetch_random_subscription_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/usecase/fetch_random_subscription_usecase"
	"context"
)

// FetchRandomSubscriptionGateway implements the port for fetching random feeds.
type FetchRandomSubscriptionGateway struct {
	altDB *alt_db.AltDBRepository
}

// NewFetchRandomSubscriptionGateway creates a new gateway instance.
func NewFetchRandomSubscriptionGateway(altDB *alt_db.AltDBRepository) *FetchRandomSubscriptionGateway {
	return &FetchRandomSubscriptionGateway{
		altDB: altDB,
	}
}

// FetchRandomSubscription retrieves a random feed from the feeds table.
// Returns a Feed with title, description, and link for the Tag Trail feature.
func (g *FetchRandomSubscriptionGateway) FetchRandomSubscription(ctx context.Context) (*domain.Feed, error) {
	feed, err := g.altDB.FetchRandomFeed(ctx)
	if err != nil {
		return nil, err
	}
	if feed == nil {
		return nil, fetch_random_subscription_usecase.ErrNoSubscriptions
	}
	return feed, nil
}
