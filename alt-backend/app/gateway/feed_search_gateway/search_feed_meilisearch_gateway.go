package feed_search_gateway

import (
	"alt/domain"
	"alt/port/search_indexer_port"
	"alt/utils/logger"
	"context"
	"fmt"
)

type SearchFeedMeilisearchGateway struct {
	searchIndexerPort search_indexer_port.SearchIndexerPort
}

func NewSearchFeedMeilisearchGateway(searchIndexerPort search_indexer_port.SearchIndexerPort) *SearchFeedMeilisearchGateway {
	return &SearchFeedMeilisearchGateway{
		searchIndexerPort: searchIndexerPort,
	}
}

func (g *SearchFeedMeilisearchGateway) SearchFeeds(ctx context.Context, query string) ([]domain.SearchArticleHit, error) {
	// contextからuser取得
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.GlobalContext.WithContext(ctx).Error("user context not found", "error", err)
		return nil, fmt.Errorf("authentication required: %w", err)
	}

	logger.GlobalContext.WithContext(ctx).Info("searching feeds via search-indexer",
		"query", query,
		"user_id", user.UserID)

	// user_idをportに渡す
	hits, err := g.searchIndexerPort.SearchArticles(ctx, query, user.UserID.String())
	if err != nil {
		logger.GlobalContext.WithContext(ctx).Error("failed to search articles",
			"error", err,
			"query", query,
			"user_id", user.UserID)
		return nil, err
	}

	logger.GlobalContext.WithContext(ctx).Info("search-indexer search completed",
		"query", query,
		"user_id", user.UserID,
		"hits_count", len(hits))

	results := make([]domain.SearchArticleHit, len(hits))
	for i, hit := range hits {
		results[i] = domain.SearchArticleHit{
			ID:      hit.ID,
			Title:   hit.Title,
			Content: hit.Content,
			Tags:    hit.Tags,
		}
	}

	return results, nil
}

func (g *SearchFeedMeilisearchGateway) SearchFeedsWithPagination(ctx context.Context, query string, offset int, limit int) ([]domain.SearchArticleHit, int, error) {
	// contextからuser取得
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.GlobalContext.WithContext(ctx).Error("user context not found", "error", err)
		return nil, 0, fmt.Errorf("authentication required: %w", err)
	}

	logger.GlobalContext.WithContext(ctx).Info("searching feeds via search-indexer with pagination",
		"query", query,
		"user_id", user.UserID,
		"offset", offset,
		"limit", limit)

	// Get all results first (temporary implementation)
	// TODO: Extend search-indexer API to support offset/limit for better performance
	hits, err := g.searchIndexerPort.SearchArticles(ctx, query, user.UserID.String())
	if err != nil {
		logger.GlobalContext.WithContext(ctx).Error("failed to search articles",
			"error", err,
			"query", query,
			"user_id", user.UserID)
		return nil, 0, err
	}

	totalCount := len(hits)

	// Apply pagination by slicing
	start := offset
	if start > totalCount {
		start = totalCount
	}
	end := start + limit
	if end > totalCount {
		end = totalCount
	}

	var paginatedHits []domain.SearchIndexerArticleHit
	if start < totalCount {
		paginatedHits = hits[start:end]
	} else {
		paginatedHits = []domain.SearchIndexerArticleHit{}
	}

	results := make([]domain.SearchArticleHit, len(paginatedHits))
	for i, hit := range paginatedHits {
		results[i] = domain.SearchArticleHit{
			ID:      hit.ID,
			Title:   hit.Title,
			Content: hit.Content,
			Tags:    hit.Tags,
		}
	}

	logger.GlobalContext.WithContext(ctx).Info("search-indexer search with pagination completed",
		"query", query,
		"user_id", user.UserID,
		"offset", offset,
		"limit", limit,
		"total_count", totalCount,
		"returned_count", len(results))

	return results, totalCount, nil
}
