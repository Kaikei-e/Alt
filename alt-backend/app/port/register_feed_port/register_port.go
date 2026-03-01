package register_feed_port

import (
	"alt/domain"
	"context"
)

type RegisterFeedLinkPort interface {
	RegisterRSSFeedLink(ctx context.Context, link string) error
}

type RegisterFeedsPort interface {
	RegisterFeeds(ctx context.Context, feeds []*domain.FeedItem) ([]string, error)
}
