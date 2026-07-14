package article_content_cache_port

import (
	"alt/domain"
	"context"

	"github.com/google/uuid"
)

type ArticleContentCachePort interface {
	GetArticles(ctx context.Context, articleIDs []uuid.UUID) ([]*domain.Article, error)
	InvalidateArticle(ctx context.Context, articleID uuid.UUID) error
}
