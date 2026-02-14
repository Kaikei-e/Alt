package cached_article_tags_port

import (
	"alt/domain"
	"context"
)

// CachedArticleTagsPort defines the interface for fetching only cached (DB-stored) tags without triggering generation.
type CachedArticleTagsPort interface {
	FetchCachedArticleTags(ctx context.Context, articleID string) ([]*domain.FeedTag, error)
}
