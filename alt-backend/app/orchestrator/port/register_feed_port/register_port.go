package register_feed_port

import (
	"alt/domain"
	"context"
)

type RegisterFeedLinkPort interface {
	// RegisterFeedLink inserts a feed link URL into the database.
	// This is a DB-only operation — no external HTTP fetching.
	RegisterFeedLink(ctx context.Context, link string) error
}

type RegisterFeedsPort interface {
	RegisterFeeds(ctx context.Context, feeds []*domain.FeedItem) ([]RegisterFeedResult, error)
}

// RegisterFeedResult reports the outcome of registering one RSS item.
// FeedID is a feeds.id. Registration does not create an articles row, so this
// value must never be published as an article identifier (ADR-000953).
type RegisterFeedResult struct {
	FeedID  string
	Created bool
}
