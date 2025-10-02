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

	// Get feed ID from feed URL
	// Handle URL normalization - match with or without trailing slash
	identifyFeedQuery := `
                SELECT id FROM feeds
                WHERE link = $1 OR link = $1 || '/' OR link = RTRIM($1, '/')
                LIMIT 1
        `

	// First, check if feed exists WITHOUT starting a transaction
	var feedID string
	err = r.pool.QueryRow(ctx, identifyFeedQuery, feedURL.String()).Scan(&feedID)
	if err != nil {
		logger.SafeError("Error identifying feed", "error", err, "feedURL", feedURL.String())
		return pgx.ErrNoRows
	}

	// Only start transaction after we know the feed exists
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		logger.SafeError("Error beginning transaction", "error", err)
		return pgx.ErrTxClosed
	}

	// Ensure transaction is always cleaned up
	// Use context.Background() for rollback to ensure it completes even if request context is cancelled
	defer func() {
		if err := tx.Rollback(context.Background()); err != nil && err.Error() != "tx is closed" {
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

	// Use context.Background() for commit to ensure it completes even if request context is cancelled
	if err = tx.Commit(context.Background()); err != nil {
		logger.SafeError("Error committing transaction", "error", err)
		return err
	}

	logger.SafeInfo("feed status updated successfully", 
		"user_id", user.UserID, 
		"feed_id", feedID, 
		"is_read", true)
	return nil
}
