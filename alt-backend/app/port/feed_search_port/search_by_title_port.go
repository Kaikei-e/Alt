package feed_search_port

import (
	"alt/domain"
	"context"
)

type SearchByTitlePort interface {
	SearchByTitle(ctx context.Context, query string) ([]*domain.FeedItem, error)
}
