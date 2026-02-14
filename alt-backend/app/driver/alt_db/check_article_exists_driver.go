package alt_db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// CheckArticleExistsByURL checks if an article exists by URL and feed_id.
// Returns the article ID if found.
func (r *AltDBRepository) CheckArticleExistsByURL(ctx context.Context, url string, feedID string) (bool, string, error) {
	if r.pool == nil {
		return false, "", errors.New("database connection not available")
	}

	query := `SELECT id FROM articles WHERE url = $1 AND feed_id = $2 AND deleted_at IS NULL LIMIT 1`

	var articleID string
	err := r.pool.QueryRow(ctx, query, url, feedID).Scan(&articleID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, "", nil
		}
		return false, "", err
	}

	return true, articleID, nil
}
