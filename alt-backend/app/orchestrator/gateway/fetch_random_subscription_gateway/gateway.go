package fetch_random_subscription_gateway

import (
	"alt/domain"
	"context"
)

// RandomFeedStore is the Tag Trail entry point's one read (capability catalog
// §2.H W3-H10).
//
// It answers (nil, nil) when no feed has a tag yet, which this gateway turns
// into ErrNoSubscriptions — a state the entry point renders rather than an
// error it reports.
type RandomFeedStore interface {
	FetchRandomFeed(ctx context.Context) (*domain.Feed, error)
}

// FetchRandomSubscriptionGateway implements the port for fetching random feeds.
type FetchRandomSubscriptionGateway struct {
	store RandomFeedStore
}

// NewFetchRandomSubscriptionGateway creates a new gateway instance.
func NewFetchRandomSubscriptionGateway(store RandomFeedStore) *FetchRandomSubscriptionGateway {
	if store == nil {
		panic("fetch_random_subscription_gateway: RandomFeedStore is required (see .claude/rules/di-wiring.md)")
	}
	return &FetchRandomSubscriptionGateway{store: store}
}

// FetchRandomSubscription retrieves a random feed from the feeds table.
// Returns a Feed with title, description, and link for the Tag Trail feature.
func (g *FetchRandomSubscriptionGateway) FetchRandomSubscription(ctx context.Context) (*domain.Feed, error) {
	feed, err := g.store.FetchRandomFeed(ctx)
	if err != nil {
		return nil, err
	}
	if feed == nil {
		return nil, domain.ErrNoSubscriptions
	}
	return feed, nil
}
