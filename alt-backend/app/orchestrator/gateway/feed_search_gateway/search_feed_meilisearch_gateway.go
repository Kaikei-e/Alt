package feed_search_gateway

import (
	"alt/domain"
	"alt/orchestrator/port/search_indexer_port"
	"alt/utils/logger"
	"context"
	"fmt"
)

type SearchFeedMeilisearchGateway struct {
	searchIndexerPort search_indexer_port.ArticleSearchPort
}

func NewSearchFeedMeilisearchGateway(searchIndexerPort search_indexer_port.ArticleSearchPort) *SearchFeedMeilisearchGateway {
	return &SearchFeedMeilisearchGateway{
		searchIndexerPort: searchIndexerPort,
	}
}

func (g *SearchFeedMeilisearchGateway) SearchFeeds(ctx context.Context, query string) ([]domain.SearchArticleHit, error) {
	// contextからuser取得
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.GlobalContext.WithContext(ctx).ErrorContext(ctx, "user context not found", "error", err)
		return nil, fmt.Errorf("authentication required: %w", err)
	}

	logger.GlobalContext.WithContext(ctx).InfoContext(ctx, "searching feeds via search-indexer",
		"query", query,
		"user_id", user.UserID)

	// user_idをportに渡す
	hits, err := g.searchIndexerPort.SearchArticles(ctx, query, user.UserID.String())
	if err != nil {
		logger.GlobalContext.WithContext(ctx).ErrorContext(ctx, "failed to search articles",
			"error", err,
			"query", query,
			"user_id", user.UserID)
		return nil, err
	}

	logger.GlobalContext.WithContext(ctx).InfoContext(ctx, "search-indexer search completed",
		"query", query,
		"user_id", user.UserID,
		"hits_count", len(hits))

	results := mapIndexerHitsToArticleHits(hits)

	return results, nil
}

func (g *SearchFeedMeilisearchGateway) SearchFeedsWithPagination(ctx context.Context, query string, offset int, limit int) ([]domain.SearchArticleHit, int, error) {
	// contextからuser取得
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.GlobalContext.WithContext(ctx).ErrorContext(ctx, "user context not found", "error", err)
		return nil, 0, fmt.Errorf("authentication required: %w", err)
	}

	logger.GlobalContext.WithContext(ctx).InfoContext(ctx, "searching feeds via search-indexer with pagination",
		"query", query,
		"user_id", user.UserID,
		"offset", offset,
		"limit", limit)

	// Use pagination-aware search method
	hits, estimatedTotal, err := g.searchIndexerPort.SearchArticlesWithPagination(ctx, query, user.UserID.String(), offset, limit)
	if err != nil {
		logger.GlobalContext.WithContext(ctx).ErrorContext(ctx, "failed to search articles",
			"error", err,
			"query", query,
			"user_id", user.UserID)
		return nil, 0, err
	}

	// Use estimated total from Meilisearch for proper pagination
	totalCount := int(estimatedTotal)

	results := mapIndexerHitsToArticleHits(hits)

	logger.GlobalContext.WithContext(ctx).InfoContext(ctx, "search-indexer search with pagination completed",
		"query", query,
		"user_id", user.UserID,
		"offset", offset,
		"limit", limit,
		"total_count", totalCount,
		"returned_count", len(results))

	return results, totalCount, nil
}

// mapIndexerHitsToArticleHits maps search indexer hits to domain article hits.
func mapIndexerHitsToArticleHits(hits []domain.SearchIndexerArticleHit) []domain.SearchArticleHit {
	results := make([]domain.SearchArticleHit, len(hits))
	for i, hit := range hits {
		results[i] = domain.SearchArticleHit(hit)
	}
	return results
}
