package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"net/url"

	"github.com/jackc/pgx/v5"
)

func (r *AltDBRepository) UpdateFeedStatus(ctx context.Context, feedURL url.URL) error {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.SafeError("user context not found", "error", err)
		return errors.New("authentication required")
	}

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

	// Upsert read status for the feed
	updateFeedStatusQuery := `
        INSERT INTO read_status (feed_id, user_id, is_read, read_at, created_at)
        VALUES ($1, $2, TRUE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
        ON CONFLICT (feed_id, user_id) DO UPDATE
        SET is_read = TRUE, read_at = CURRENT_TIMESTAMP
        `
	if _, err = tx.Exec(ctx, updateFeedStatusQuery, feedID, user.UserID); err != nil {
		logger.SafeError("Error updating feed status", 
			"error", err, 
			"user_id", user.UserID, 
			"feed_id", feedID)
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		logger.SafeError("Error committing transaction", "error", err)
		return err
	}

	logger.SafeInfo("feed status updated successfully", 
		"user_id", user.UserID, 
		"feed_id", feedID, 
		"is_read", true)
	return nil
}
