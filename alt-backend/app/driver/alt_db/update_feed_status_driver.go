package alt_db

import (
	"alt/utils/logger"
	"context"
	"net/url"

	"github.com/jackc/pgx/v5"
)

func (r *AltDBRepository) UpdateFeedStatus(ctx context.Context, feedURL url.URL) error {
	identifyFeedQuery := `
		SELECT id FROM feeds WHERE link = $1
	`
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		logger.SafeError("Error beginning transaction", "error", err)
		return pgx.ErrTxClosed
	}

	var feedID string
	err = tx.QueryRow(ctx, identifyFeedQuery, feedURL.String()).Scan(&feedID)
	if err != nil {
		logger.SafeError("Error identifying feed", "error", err, "feedURL", feedURL.String())
		return pgx.ErrNoRows
	}

	// Ensure transaction is always cleaned up
	defer func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			logger.SafeWarn("Error rolling back transaction", "error", err)
		}
	}()

	// Option C: Safe UPSERT pattern using merge-style approach
	// Uses a default user_id to match the current table schema: (feed_id, user_id) composite
	defaultUserID := "00000000-0000-0000-0000-000000000001"
	
	updateFeedStatusQuery := `
		WITH upsert AS (
			UPDATE read_status 
			SET is_read = TRUE, updated_at = CURRENT_TIMESTAMP
			WHERE feed_id = $1 AND user_id = $2
			RETURNING *
		)
		INSERT INTO read_status (feed_id, user_id, is_read, created_at, updated_at)
		SELECT $1, $2, TRUE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		WHERE NOT EXISTS (SELECT * FROM upsert)
	`
	_, err = tx.Exec(ctx, updateFeedStatusQuery, feedID, defaultUserID)
	if err != nil {
		logger.SafeError("Error updating feed status", "error", err, "feedID", feedID)
		return err
	}

	err = tx.Commit(ctx)
	if err != nil {
		logger.SafeError("Error committing transaction", "error", err)
		return err
	}

	return nil
}
