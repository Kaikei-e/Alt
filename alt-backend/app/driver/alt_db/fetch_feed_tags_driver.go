package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"time"
)

// FetchFeedTags retrieves tags associated with a specific feed through article_tags table
// Uses INNER JOIN to fetch tags that are actually associated with articles from the feed
func (r *AltDBRepository) FetchFeedTags(ctx context.Context, feedID string, cursor *time.Time, limit int) ([]*domain.FeedTag, error) {
	if r.pool == nil {
		return nil, errors.New("database connection not available")
	}

	// Cursor-based pagination query (similar to existing feed patterns)
	var query string
	var args []interface{}

	if cursor == nil {
		// First page - no cursor
		query = `
			SELECT DISTINCT t.id, t.tag_name, t.created_at
			FROM feed_tags t
			INNER JOIN article_tags at ON t.id = at.feed_tag_id
			INNER JOIN articles a ON at.article_id = a.id
			WHERE a.feed_id = $1
			ORDER BY t.created_at DESC, t.id DESC
			LIMIT $2
		`
		args = []interface{}{feedID, limit}
	} else {
		// Subsequent pages - use cursor
		query = `
			SELECT DISTINCT t.id, t.tag_name, t.created_at
			FROM feed_tags t
			INNER JOIN article_tags at ON t.id = at.feed_tag_id
			INNER JOIN articles a ON at.article_id = a.id
			WHERE a.feed_id = $1
			AND t.created_at < $2
			ORDER BY t.created_at DESC, t.id DESC
			LIMIT $3
		`
		args = []interface{}{feedID, cursor, limit}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		logger.Logger.Error("error fetching feed tags", "error", err, "feedID", feedID)
		return nil, errors.New("error fetching feed tags")
	}
	defer rows.Close()

	var tags []*domain.FeedTag
	for rows.Next() {
		var tag domain.FeedTag
		err := rows.Scan(&tag.ID, &tag.TagName, &tag.CreatedAt)
		if err != nil {
			logger.Logger.Error("error scanning feed tag", "error", err)
			return nil, errors.New("error scanning feed tags")
		}
		tags = append(tags, &tag)
	}

	logger.Logger.Info("fetched feed tags from database", "feedID", feedID, "count", len(tags))
	return tags, nil
}
