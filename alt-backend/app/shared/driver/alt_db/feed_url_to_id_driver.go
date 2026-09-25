package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrFeedNotFoundByURL is returned when a feed URL is not found in the database.
var ErrFeedNotFoundByURL = errors.New("feed not found by URL")

// GetFeedIDByURL retrieves the feed ID for a given RSS feed URL by joining feed_links.
// This is used by Connect-RPC GetFeedID and FetchFeedTags where the URL is an RSS source URL.
func (r *FeedRepository) GetFeedIDByURL(ctx context.Context, feedURL string) (string, error) {
	if r.pool == nil {
		return "", errors.New("database connection not available")
	}

	query := `SELECT f.id FROM feeds f INNER JOIN feed_links fl ON f.feed_link_id = fl.id WHERE fl.url = $1`

	var feedID string
	err := r.pool.QueryRow(ctx, query, feedURL).Scan(&feedID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logger.SafeInfoContext(ctx, "feed not found by URL", "feedURL", feedURL)
			return "", ErrFeedNotFoundByURL
		}
		logger.SafeErrorContext(ctx, "error getting feed ID by URL", "error", err, "feedURL", feedURL)
		return "", fmt.Errorf("error getting feed ID by URL: %w", err)
	}

	logger.SafeInfoContext(ctx, "retrieved feed ID from database", "feedURL", feedURL, "feedID", feedID)
	return feedID, nil
}
