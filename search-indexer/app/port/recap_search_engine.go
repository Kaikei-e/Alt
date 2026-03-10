package port

import (
	"context"
	"search-indexer/domain"
)

// RecapSearchEngine provides Meilisearch operations for recap documents.
type RecapSearchEngine interface {
	EnsureRecapIndex(ctx context.Context) error
	IndexRecapDocuments(ctx context.Context, docs []domain.RecapDocument) error
	SearchRecaps(ctx context.Context, query string, limit int) ([]domain.RecapDocument, int64, error)
}
