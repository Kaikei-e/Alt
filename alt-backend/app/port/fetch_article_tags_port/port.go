package fetch_article_tags_port

import (
	"alt/domain"
	"context"
)

// FetchArticleTagsPort defines the interface for fetching tags of a specific article.
type FetchArticleTagsPort interface {
	FetchArticleTags(ctx context.Context, articleID string) ([]*domain.FeedTag, error)
}
