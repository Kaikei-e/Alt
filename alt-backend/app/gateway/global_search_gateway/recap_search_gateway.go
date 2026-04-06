package global_search_gateway

import (
	"alt/domain"
	"alt/port/search_indexer_port"
	"context"
	"log/slog"
)

// RecapSearchGateway implements global_search_port.SearchRecapsPort.
type RecapSearchGateway struct {
	searchIndexer search_indexer_port.SearchIndexerPort
	logger        *slog.Logger
}

// NewRecapSearchGateway creates a new RecapSearchGateway.
func NewRecapSearchGateway(searchIndexer search_indexer_port.SearchIndexerPort) *RecapSearchGateway {
	return &RecapSearchGateway{
		searchIndexer: searchIndexer,
		logger:        slog.Default(),
	}
}

// SearchRecapsForGlobal searches recaps for the global search overview.
func (g *RecapSearchGateway) SearchRecapsForGlobal(ctx context.Context, query string, limit int) (*domain.RecapSearchSection, error) {
	results, totalCount, err := g.searchIndexer.SearchRecapsByQuery(ctx, query, limit)
	if err != nil {
		g.logger.ErrorContext(ctx, "failed to search recaps for global search", "error", err, "query", query)
		return nil, err
	}

	hits := make([]domain.GlobalRecapHit, len(results))
	for i, r := range results {
		hits[i] = domain.GlobalRecapHit{
			ID:         r.JobID + "__" + r.Genre,
			JobID:      r.JobID,
			Genre:      r.Genre,
			Summary:    r.Summary,
			TopTerms:   r.TopTerms,
			Tags:       r.Tags,
			WindowDays: r.WindowDays,
			ExecutedAt: r.ExecutedAt,
		}
	}

	return &domain.RecapSearchSection{
		Hits:           hits,
		EstimatedTotal: totalCount,
		HasMore:        totalCount > int64(limit),
	}, nil
}
