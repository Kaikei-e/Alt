package port

import (
	"context"
	"search-indexer/domain"
	"time"
)

type SearchEngine interface {
	IndexDocuments(ctx context.Context, docs []domain.SearchDocument) error
	DeleteDocuments(ctx context.Context, ids []string) error
	Search(ctx context.Context, query string, limit int) ([]domain.SearchDocument, error)
	SearchWithFilters(ctx context.Context, query string, filters []string, limit int) ([]domain.SearchDocument, error)
	// SearchWithDateFilter restricts results to documents whose
	// ``published_at`` falls inside the supplied window. Either bound may
	// be nil; when both are nil the engine should behave like Search.
	SearchWithDateFilter(ctx context.Context, query string, publishedAfter, publishedBefore *time.Time, limit int) ([]domain.SearchDocument, error)
	SearchByUserID(ctx context.Context, query string, userID string, limit int) ([]domain.SearchDocument, error)
	SearchByUserIDWithPagination(ctx context.Context, query string, userID string, offset, limit int64) ([]domain.SearchDocument, int64, error)
	EnsureIndex(ctx context.Context) error
	RegisterSynonyms(ctx context.Context, synonyms map[string][]string) error
	// PruneTaskHistory deletes finished Meilisearch tasks older than
	// olderThan. See bootstrap.runTaskPruneLoop for why this must run
	// periodically: Meilisearch's own automatic cleanup only triggers once
	// total stored tasks reach 1M, which never happens here because a few
	// thousand large settingsUpdate payloads exhaust the task database's
	// byte budget long before that count.
	PruneTaskHistory(ctx context.Context, olderThan time.Duration) error
}
