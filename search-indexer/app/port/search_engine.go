package port

import (
	"context"
	"search-indexer/domain"
)

type SearchEngine interface {
	IndexDocuments(ctx context.Context, docs []domain.SearchDocument) error
	DeleteDocuments(ctx context.Context, ids []string) error
	Search(ctx context.Context, query string, limit int) ([]domain.SearchDocument, error)
	SearchWithFilters(ctx context.Context, query string, filters []string, limit int) ([]domain.SearchDocument, error)
	SearchByUserID(ctx context.Context, query string, userID string, limit int) ([]domain.SearchDocument, error)
	SearchByUserIDWithPagination(ctx context.Context, query string, userID string, offset, limit int64) ([]domain.SearchDocument, int64, error)
	EnsureIndex(ctx context.Context) error
	RegisterSynonyms(ctx context.Context, synonyms map[string][]string) error
}
