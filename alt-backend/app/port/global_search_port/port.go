package global_search_port

import (
	"alt/domain"
	"context"
)

//go:generate mockgen -source=port.go -destination=../../mocks/mock_global_search_port.go -package=mocks

// SearchArticlesPort searches articles for the global search overview.
type SearchArticlesPort interface {
	SearchArticlesForGlobal(ctx context.Context, query string, userID string, limit int) (*domain.ArticleSearchSection, error)
}

// SearchRecapsPort searches recaps for the global search overview.
type SearchRecapsPort interface {
	SearchRecapsForGlobal(ctx context.Context, query string, limit int) (*domain.RecapSearchSection, error)
}

// SearchTagsPort searches tags by prefix for the global search overview.
type SearchTagsPort interface {
	SearchTagsByPrefix(ctx context.Context, prefix string, limit int) (*domain.TagSearchSection, error)
}
