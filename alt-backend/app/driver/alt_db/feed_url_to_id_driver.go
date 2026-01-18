package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"
)

// GetFeedIDByURL retrieves the feed ID for a given feed URL from the feeds table
func (r *AltDBRepository) GetFeedIDByURL(ctx context.Context, feedURL string) (string, error) {
	if r.pool == nil {
		return "", errors.New("database connection not available")
	}

	// Query to get feed ID by URL (following existing pattern from update_feed_status_driver.go)
	query := `SELECT id FROM feeds WHERE link = $1`

	var feedID string
	err := r.pool.QueryRow(ctx, query, feedURL).Scan(&feedID)
	if err != nil {
		logger.SafeErrorContext(ctx, "error getting feed ID by URL", "error", err, "feedURL", feedURL)
		return "", errors.New("error getting feed ID by URL")
	}

	logger.SafeInfoContext(ctx, "retrieved feed ID from database", "feedURL", feedURL, "feedID", feedID)
	return feedID, nil
}
