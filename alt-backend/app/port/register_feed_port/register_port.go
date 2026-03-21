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

type RegisterFeedResult struct {
	ArticleID string
	Created   bool
}
