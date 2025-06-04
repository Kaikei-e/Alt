package fetch_feed_port

import (
	"alt/domain"
	"context"
)

type FetchSingleFeedPort interface {
	FetchSingleFeed(ctx context.Context) (*domain.RSSFeed, error)
}

type FetchFeedsPort interface {
	FetchFeeds(ctx context.Context, link string) ([]*domain.FeedItem, error)
	FetchFeedsList(ctx context.Context) ([]*domain.FeedItem, error)
	FetchFeedsListLimit(ctx context.Context, offset int) ([]*domain.FeedItem, error)
	FetchFeedsListPage(ctx context.Context, page int) ([]*domain.FeedItem, error)
}
