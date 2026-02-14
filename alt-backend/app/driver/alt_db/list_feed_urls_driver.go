package alt_db

import (
	"context"
	"errors"
)

// InternalFeedURL represents a feed ID/URL pair.
type InternalFeedURL struct {
	FeedID string
	URL    string
}

// ListFeedURLs returns feed URLs with cursor-based pagination.
// The cursor is the last feed ID from the previous page.
func (r *AltDBRepository) ListFeedURLs(ctx context.Context, cursor string, limit int) ([]InternalFeedURL, string, bool, error) {
	if r.pool == nil {
		return nil, "", false, errors.New("database connection not available")
	}

	// Fetch limit+1 to determine if there are more results.
	var query string
	var args []any

	if cursor == "" {
		query = `
			SELECT f.id, fl.url
			FROM feeds f
			JOIN feed_links fl ON f.link = fl.url
			LEFT JOIN feed_link_availability fla ON fl.id = fla.feed_link_id
			WHERE fla.is_active IS NULL OR fla.is_active = true
			ORDER BY f.id ASC
			LIMIT $1
		`
		args = []any{limit + 1}
	} else {
		query = `
			SELECT f.id, fl.url
			FROM feeds f
			JOIN feed_links fl ON f.link = fl.url
			LEFT JOIN feed_link_availability fla ON fl.id = fla.feed_link_id
			WHERE (fla.is_active IS NULL OR fla.is_active = true)
			  AND f.id > $1
			ORDER BY f.id ASC
			LIMIT $2
		`
		args = []any{cursor, limit + 1}
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, "", false, err
	}
	defer rows.Close()

	var feeds []InternalFeedURL
	for rows.Next() {
		var f InternalFeedURL
		if err := rows.Scan(&f.FeedID, &f.URL); err != nil {
			return nil, "", false, err
		}
		feeds = append(feeds, f)
	}
	if err := rows.Err(); err != nil {
		return nil, "", false, err
	}

	hasMore := len(feeds) > limit
	if hasMore {
		feeds = feeds[:limit]
	}

	var nextCursor string
	if len(feeds) > 0 {
		nextCursor = feeds[len(feeds)-1].FeedID
	}

	return feeds, nextCursor, hasMore, nil
}
