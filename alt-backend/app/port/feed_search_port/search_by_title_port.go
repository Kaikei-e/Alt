package feed_search_port

import (
	"alt/domain"
	"context"
)

type SearchByTitlePort interface {
	SearchFeedsByTitle(ctx context.Context, query string, userID string) ([]*domain.FeedItem, error)
}

type SearchFeedPort interface {
	SearchFeeds(ctx context.Context, query string) ([]domain.SearchArticleHit, error)
}
