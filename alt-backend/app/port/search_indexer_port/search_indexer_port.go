package search_indexer_port

import (
	"alt/domain"
	"context"
)

type SearchIndexerPort interface {
	SearchArticles(ctx context.Context, query string, userID string) ([]domain.SearchIndexerArticleHit, error)
}
