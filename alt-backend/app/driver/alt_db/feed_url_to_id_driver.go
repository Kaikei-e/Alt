package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"
)

// GetFeedIDByArticleURL retrieves the feed ID for a given article URL by looking up feeds.link.
// This is used by SaveArticle where the URL is an article page URL stored in feeds.link.
func (r *AltDBRepository) GetFeedIDByArticleURL(ctx context.Context, articleURL string) (string, error) {
	if r.pool == nil {
		return "", errors.New("database connection not available")
	}

	query := `SELECT id FROM feeds WHERE link = $1`

	var feedID string
	err := r.pool.QueryRow(ctx, query, articleURL).Scan(&feedID)
	if err != nil {
		logger.SafeErrorContext(ctx, "error getting feed ID by article URL", "error", err, "articleURL", articleURL)
		return "", errors.New("error getting feed ID by article URL")
	}

	logger.SafeInfoContext(ctx, "retrieved feed ID by article URL", "articleURL", articleURL, "feedID", feedID)
	return feedID, nil
}

// GetFeedIDByURL retrieves the feed ID for a given RSS feed URL by joining feed_links.
// This is used by Connect-RPC GetFeedID and FetchFeedTags where the URL is an RSS source URL.
func (r *AltDBRepository) GetFeedIDByURL(ctx context.Context, feedURL string) (string, error) {
	if r.pool == nil {
		return "", errors.New("database connection not available")
	}

	query := `SELECT f.id FROM feeds f INNER JOIN feed_links fl ON f.feed_link_id = fl.id WHERE fl.url = $1`

	var feedID string
	err := r.pool.QueryRow(ctx, query, feedURL).Scan(&feedID)
	if err != nil {
		logger.SafeErrorContext(ctx, "error getting feed ID by URL", "error", err, "feedURL", feedURL)
		return "", errors.New("error getting feed ID by URL")
	}

	logger.SafeInfoContext(ctx, "retrieved feed ID from database", "feedURL", feedURL, "feedID", feedID)
	return feedID, nil
}
