package feed_url_to_id_gateway

import (
	"alt/driver/alt_db"
	"alt/utils/logger"
	"context"
	"errors"
)

type FeedURLToIDGateway struct {
	alt_db *alt_db.AltDBRepository
}

func NewFeedURLToIDGateway(alt_db *alt_db.AltDBRepository) *FeedURLToIDGateway {
	return &FeedURLToIDGateway{
		alt_db: alt_db,
	}
}

func (g *FeedURLToIDGateway) GetFeedIDByURL(ctx context.Context, feedURL string) (string, error) {
	if g.alt_db == nil {
		return "", errors.New("database connection not available")
	}

	logger.Logger.Info("getting feed ID by URL", "feedURL", feedURL)

	// Call driver layer to get feed ID
	feedID, err := g.alt_db.GetFeedIDByURL(ctx, feedURL)
	if err != nil {
		logger.Logger.Error("failed to get feed ID by URL", "error", err, "feedURL", feedURL)
		return "", errors.New("error getting feed ID by URL")
	}

	logger.Logger.Info("successfully retrieved feed ID", "feedID", feedID)
	return feedID, nil
}
