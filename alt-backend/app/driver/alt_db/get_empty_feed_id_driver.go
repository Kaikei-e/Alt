package alt_db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// GetEmptyFeedID returns a feed ID that has no articles for the given feed URL.
// Returns empty string if all feeds for this URL already have articles.
func (r *AltDBRepository) GetEmptyFeedID(ctx context.Context, feedURL string) (string, error) {
	if r.pool == nil {
		return "", errors.New("database connection not available")
	}

	query := `
		SELECT f.id
		FROM feeds f
		JOIN feed_links fl ON f.feed_link_id = fl.id
		WHERE fl.url = $1
		AND NOT EXISTS (SELECT 1 FROM articles a WHERE a.feed_id = f.id AND a.deleted_at IS NULL)
		LIMIT 1`

	var feedID string
	err := r.pool.QueryRow(ctx, query, feedURL).Scan(&feedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil // no empty feed found
	}
	if err != nil {
		return "", fmt.Errorf("get empty feed ID: %w", err)
	}

	return feedID, nil
}
