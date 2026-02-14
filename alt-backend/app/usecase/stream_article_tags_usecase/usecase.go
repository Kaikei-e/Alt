package stream_article_tags_usecase

import (
	"alt/domain"
	"alt/port/cached_article_tags_port"
	"alt/port/fetch_article_tags_port"
	"context"
)

// TagsResult holds the result of tag fetching with source info.
type TagsResult struct {
	Tags     []*domain.FeedTag
	IsCached bool
}

// StreamArticleTagsUsecase handles the two-step tag resolution:
// 1. Check cache (DB-only) for existing tags
// 2. If not found, trigger on-the-fly generation via the full port
type StreamArticleTagsUsecase struct {
	cachedPort    cached_article_tags_port.CachedArticleTagsPort
	generatorPort fetch_article_tags_port.FetchArticleTagsPort
}

// NewStreamArticleTagsUsecase creates a new usecase instance.
func NewStreamArticleTagsUsecase(
	cachedPort cached_article_tags_port.CachedArticleTagsPort,
	generatorPort fetch_article_tags_port.FetchArticleTagsPort,
) *StreamArticleTagsUsecase {
	return &StreamArticleTagsUsecase{
		cachedPort:    cachedPort,
		generatorPort: generatorPort,
	}
}

// Execute fetches tags for an article, returning cached tags if available,
// otherwise triggering on-the-fly generation.
func (u *StreamArticleTagsUsecase) Execute(ctx context.Context, articleID string) (*TagsResult, error) {
	// 1. Check DB for cached tags (no side effects)
	if u.cachedPort != nil {
		cachedTags, err := u.cachedPort.FetchCachedArticleTags(ctx, articleID)
		if err == nil && len(cachedTags) > 0 {
			return &TagsResult{Tags: cachedTags, IsCached: true}, nil
		}
		// On error or empty, continue to generation
	}

	// 2. Trigger on-the-fly generation via full port (includes DB check + mq-hub)
	if u.generatorPort != nil {
		tags, err := u.generatorPort.FetchArticleTags(ctx, articleID)
		if err != nil {
			return &TagsResult{Tags: []*domain.FeedTag{}, IsCached: false}, nil
		}
		return &TagsResult{Tags: tags, IsCached: false}, nil
	}

	return &TagsResult{Tags: []*domain.FeedTag{}, IsCached: false}, nil
}
