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
	EnsureIndex(ctx context.Context) error
	RegisterSynonyms(ctx context.Context, synonyms map[string][]string) error
}

type SearchEngineError struct {
	Op  string
	Err string
}

func (e *SearchEngineError) Error() string {
	return e.Op + ": " + e.Err
}
