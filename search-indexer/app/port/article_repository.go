package port

import (
	"context"
	"search-indexer/domain"
	"time"
)

type ArticleRepository interface {
	GetArticlesWithTags(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]*domain.Article, *time.Time, string, error)
}

type ConfigRepository interface {
	LoadSearchIndexerConfig() (*domain.SearchIndexerConfig, error)
}

type RepositoryError struct {
	Op  string
	Err string
}

func (e *RepositoryError) Error() string {
	return e.Op + ": " + e.Err
}
