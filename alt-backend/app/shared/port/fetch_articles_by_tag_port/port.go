package fetch_articles_by_tag_port

import (
	"alt/domain"
	"context"
	"time"
)

// FetchArticlesByTagPort defines the interface for fetching articles by tag.
type FetchArticlesByTagPort interface {
	FetchArticlesByTag(ctx context.Context, tagID string, cursor *time.Time, limit int) ([]*domain.TagTrailArticle, error)
	FetchArticlesByTagName(ctx context.Context, tagName string, cursor *time.Time, limit int) ([]*domain.TagTrailArticle, error)
}
