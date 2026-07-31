package feed_link_gateway

import (
	"alt/domain"
	"context"

	"github.com/google/uuid"
)

// FeedLinkStore is the slice of alt-data-hub the feed-link management screens
// use (ADR-000954 Wave 3 batch 3, capability catalog §2.F).
//
// An interface rather than a repository handle because this gateway no longer
// owns a database. Naming the three calls it makes keeps a later edit from
// reaching a fourth one it has no capability for.
type FeedLinkStore interface {
	FetchFeedLinks(ctx context.Context) ([]*domain.FeedLink, error)
	FetchFeedLinksWithAvailability(ctx context.Context) ([]*domain.FeedLinkWithHealth, error)
	DeleteFeedLink(ctx context.Context, id uuid.UUID) error
}

type FeedLinkGateway struct {
	store FeedLinkStore
}

func NewFeedLinkGateway(store FeedLinkStore) *FeedLinkGateway {
	if store == nil {
		panic("feed_link_gateway: FeedLinkStore is required (see .claude/rules/di-wiring.md)")
	}
	return &FeedLinkGateway{store: store}
}

func (g *FeedLinkGateway) ListFeedLinks(ctx context.Context) ([]*domain.FeedLink, error) {
	return g.store.FetchFeedLinks(ctx)
}

func (g *FeedLinkGateway) ListFeedLinksWithHealth(ctx context.Context) ([]*domain.FeedLinkWithHealth, error) {
	return g.store.FetchFeedLinksWithAvailability(ctx)
}

func (g *FeedLinkGateway) DeleteFeedLink(ctx context.Context, id uuid.UUID) error {
	return g.store.DeleteFeedLink(ctx, id)
}
