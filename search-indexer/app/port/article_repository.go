package port

import (
	"context"
	"search-indexer/domain"
	"time"
)

type ArticleRepository interface {
	GetArticlesWithTags(ctx context.Context, lastCreatedAt *time.Time, lastID string, limit int) ([]*domain.Article, *time.Time, string, error)
	GetArticlesWithTagsForward(ctx context.Context, incrementalMark *time.Time, lastCreatedAt *time.Time, lastID string, limit int) ([]*domain.Article, *time.Time, string, error)
	GetDeletedArticles(ctx context.Context, lastDeletedAt *time.Time, limit int) ([]string, *time.Time, error)
	GetLatestCreatedAt(ctx context.Context) (*time.Time, error)
	// GetArticleByID retrieves a single article with tags by its ID.
	GetArticleByID(ctx context.Context, articleID string) (*domain.Article, error)
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
