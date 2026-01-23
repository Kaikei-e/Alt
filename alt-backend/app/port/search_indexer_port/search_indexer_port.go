package search_indexer_port

import (
	"alt/domain"
	"context"
)

type SearchIndexerPort interface {
	SearchArticles(ctx context.Context, query string, userID string) ([]domain.SearchIndexerArticleHit, error)
	SearchArticlesWithPagination(ctx context.Context, query string, userID string, offset int, limit int) ([]domain.SearchIndexerArticleHit, int64, error)
}
