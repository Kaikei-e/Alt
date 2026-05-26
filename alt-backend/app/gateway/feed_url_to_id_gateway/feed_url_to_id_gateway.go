package feed_url_to_id_gateway

import (
	"alt/driver/alt_db"
	"alt/utils/feed_url_normalize"
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"
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

	logger.Logger.InfoContext(ctx, "getting feed ID by URL", "feedURL", feedURL)

	// Call driver layer to get feed ID — literal match first to preserve
	// behaviour for any URL the registrar stored in non-canonical form.
	feedID, err := g.alt_db.GetFeedIDByURL(ctx, feedURL)
	if err == nil {
		logger.Logger.InfoContext(ctx, "successfully retrieved feed ID", "feedID", feedID)
		return feedID, nil
	}
	if !errors.Is(err, alt_db.ErrFeedNotFoundByURL) {
		logger.Logger.ErrorContext(ctx, "failed to get feed ID by URL", "error", err, "feedURL", feedURL)
		return "", fmt.Errorf("GetFeedID: %w", err)
	}

	// Pillar 4 defense in depth: retry once with the canonical normalization.
	// The 2026-05-26 incident is feed-not-registered, but this branch makes
	// future trailing-slash / case / port drift a no-op instead of a 262-line
	// burst the operator has to parse out of the log.
	normalized := feed_url_normalize.Normalize(feedURL)
	if normalized == "" || normalized == feedURL {
		logger.Logger.InfoContext(ctx, "feed not registered", "feedURL", feedURL)
		return "", err
	}
	feedID, err2 := g.alt_db.GetFeedIDByURL(ctx, normalized)
	if err2 == nil {
		logger.Logger.InfoContext(ctx, "successfully retrieved feed ID via normalized fallback",
			"feedURL", feedURL, "normalized", normalized, "feedID", feedID)
		return feedID, nil
	}
	if !errors.Is(err2, alt_db.ErrFeedNotFoundByURL) {
		logger.Logger.ErrorContext(ctx, "failed to get feed ID by normalized URL",
			"error", err2, "feedURL", feedURL, "normalized", normalized)
		return "", fmt.Errorf("GetFeedID (normalized): %w", err2)
	}
	logger.Logger.InfoContext(ctx, "feed not registered", "feedURL", feedURL, "normalized", normalized)
	return "", err
}
