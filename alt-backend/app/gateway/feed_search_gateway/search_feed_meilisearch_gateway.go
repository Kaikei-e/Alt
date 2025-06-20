package feed_search_gateway

import (
	"alt/domain"
	"alt/port/search_indexer_port"
	"context"
	"log/slog"
)

type SearchFeedMeilisearchGateway struct {
	searchIndexerPort search_indexer_port.SearchIndexerPort
	logger            *slog.Logger
}

func NewSearchFeedMeilisearchGateway(searchIndexerPort search_indexer_port.SearchIndexerPort) *SearchFeedMeilisearchGateway {
	return &SearchFeedMeilisearchGateway{
		searchIndexerPort: searchIndexerPort,
		logger:            slog.Default(),
	}
}

func (g *SearchFeedMeilisearchGateway) SearchFeeds(ctx context.Context, query string) ([]domain.SearchArticleHit, error) {
	g.logger.Info("searching feeds via search-indexer",
		"query", query)

	hits, err := g.searchIndexerPort.SearchArticles(ctx, query)
	if err != nil {
		g.logger.Error("failed to search articles",
			"error", err,
			"query", query)
		return nil, err
	}

	g.logger.Info("search-indexer search completed",
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