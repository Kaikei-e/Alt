package feed_search_gateway

import (
	"alt/domain"
	"alt/port/search_indexer_port"
	"alt/utils/logger"
	"context"
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
	logger.GlobalContext.WithContext(ctx).Info("searching feeds via search-indexer",
		"query", query)

	hits, err := g.searchIndexerPort.SearchArticles(ctx, query)
	if err != nil {
		logger.GlobalContext.WithContext(ctx).Error("failed to search articles",
			"error", err,
			"query", query)
		return nil, err
	}

	logger.GlobalContext.WithContext(ctx).Info("search-indexer search completed",
		"query", query,
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
