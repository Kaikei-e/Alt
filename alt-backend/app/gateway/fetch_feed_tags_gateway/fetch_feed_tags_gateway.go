package fetch_feed_tags_gateway

import (
	"alt/domain"
	"alt/driver/alt_db"
	"alt/utils/logger"
	"context"
	"errors"
	"time"
)

type FetchFeedTagsGateway struct {
	alt_db *alt_db.AltDBRepository
}

func NewFetchFeedTagsGateway(alt_db *alt_db.AltDBRepository) *FetchFeedTagsGateway {
	return &FetchFeedTagsGateway{
		alt_db: alt_db,
	}
}

func (g *FetchFeedTagsGateway) FetchFeedTags(ctx context.Context, feedID string, cursor *time.Time, limit int) ([]*domain.FeedTag, error) {
	if g.alt_db == nil {
		return nil, errors.New("database connection not available")
	}

	logger.Logger.Info("fetching feed tags", "feedID", feedID, "cursor", cursor, "limit", limit)

	// Call driver layer to fetch tags
	tags, err := g.alt_db.FetchFeedTags(ctx, feedID, cursor, limit)
	if err != nil {
		logger.Logger.Error("failed to fetch feed tags", "error", err, "feedID", feedID)
		return nil, errors.New("error fetching feed tags")
	}

	logger.Logger.Info("successfully fetched feed tags", "count", len(tags))
	return tags, nil
}