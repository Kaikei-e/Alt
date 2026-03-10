package gateway

import (
	"context"
	"fmt"
	"search-indexer/domain"
	"search-indexer/driver/recap_api"
)

// RecapRepositoryGateway converts recap-worker API responses to domain models.
type RecapRepositoryGateway struct {
	client *recap_api.Client
}

// NewRecapRepositoryGateway creates a new gateway.
func NewRecapRepositoryGateway(client *recap_api.Client) *RecapRepositoryGateway {
	return &RecapRepositoryGateway{client: client}
}

// GetIndexableGenres fetches recap genres and converts to domain models.
func (g *RecapRepositoryGateway) GetIndexableGenres(ctx context.Context, since string, limit int) ([]domain.RecapDocument, bool, error) {
	resp, err := g.client.GetIndexableGenres(ctx, since, limit)
	if err != nil {
		return nil, false, fmt.Errorf("recap repository: %w", err)
	}

	docs := make([]domain.RecapDocument, len(resp.Results))
	for i, item := range resp.Results {
		docs[i] = domain.RecapDocument{
			ID:         item.JobID + "__" + item.Genre,
			JobID:      item.JobID,
			ExecutedAt: item.ExecutedAt,
			WindowDays: item.WindowDays,
			Genre:      item.Genre,
			Summary:    item.Summary,
			TopTerms:   item.TopTerms,
			Tags:       item.Tags,
			Bullets:    item.Bullets,
		}
	}

	return docs, resp.HasMore, nil
}
