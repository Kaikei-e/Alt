package fetch_tag_cloud_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/utils/logger"
	"context"
	"errors"
)

// FetchTagCloudGateway implements fetch_tag_cloud_port.FetchTagCloudPort.
type FetchTagCloudGateway struct {
	alt_db *alt_db.AltDBRepository
}

// NewFetchTagCloudGateway creates a new FetchTagCloudGateway.
func NewFetchTagCloudGateway(alt_db *alt_db.AltDBRepository) *FetchTagCloudGateway {
	return &FetchTagCloudGateway{
		alt_db: alt_db,
	}
}

// FetchTagCloud fetches tag cloud data from the database.
func (g *FetchTagCloudGateway) FetchTagCloud(ctx context.Context, limit int) ([]*domain.TagCloudItem, error) {
	if g.alt_db == nil {
		return nil, errors.New("database connection not available")
	}

	logger.Logger.InfoContext(ctx, "fetching tag cloud", "limit", limit)

	items, err := g.alt_db.FetchTagCloud(ctx, limit)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to fetch tag cloud", "error", err)
		return nil, errors.New("error fetching tag cloud")
	}

	logger.Logger.InfoContext(ctx, "successfully fetched tag cloud", "count", len(items))
	return items, nil
}

// FetchTagCooccurrences fetches tag co-occurrence data from the database.
func (g *FetchTagCloudGateway) FetchTagCooccurrences(ctx context.Context, tagNames []string) ([]*domain.TagCooccurrence, error) {
	if g.alt_db == nil {
		return nil, errors.New("database connection not available")
	}

	items, err := g.alt_db.FetchTagCooccurrences(ctx, tagNames)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "failed to fetch tag cooccurrences", "error", err)
		return nil, errors.New("error fetching tag cooccurrences")
	}

	return items, nil
}
