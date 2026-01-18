package alt_db

import (
	"alt/domain"
	"alt/utils"
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/jackc/pgx/v5"
)

func (r *AltDBRepository) UpdateFeedStatus(ctx context.Context, feedURL url.URL) error {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.SafeErrorContext(ctx, "user context not found", "error", err)
		return errors.New("authentication required")
	}

	// Normalize the input URL
	normalizedInputURL, err := utils.NormalizeURL(feedURL.String())
	if err != nil {
		logger.SafeErrorContext(ctx, "Error normalizing input URL", "error", err, "feedURL", feedURL.String())
		return err
	}

	// OPTIMIZATION: Query feed directly by normalized URL instead of loading all feeds
	// This changes from O(n) to O(1) with the index on feeds.link
	getFeedQuery := `SELECT id FROM feeds WHERE link = $1`

	var feedID string
	err = r.pool.QueryRow(ctx, getFeedQuery, normalizedInputURL).Scan(&feedID)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Return domain error instead of database error
			logger.SafeErrorContext(ctx, "Feed not found",
				"normalizedURL", normalizedInputURL,
				"originalURL", feedURL.String(),
				"user_id", user.UserID)
			return domain.ErrFeedNotFound
		}
		logger.SafeErrorContext(ctx, "Error querying feed", "error", err, "normalizedURL", normalizedInputURL)
		return fmt.Errorf("failed to query feed: %w", err)
	}

	logger.SafeInfoContext(ctx, "Found matching feed",
		"feedID", feedID,
		"normalizedURL", normalizedInputURL)

	// Start transaction for upsert
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		logger.SafeErrorContext(ctx, "Error beginning transaction", "error", err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if err := tx.Rollback(context.Background()); err != nil && err.Error() != "tx is closed" {
			logger.SafeWarnContext(ctx, "Error rolling back transaction", "error", err)
		}
	}()

	// Upsert read status
	updateFeedStatusQuery := `
        INSERT INTO read_status (feed_id, user_id, is_read, read_at, created_at)
        VALUES ($1, $2, TRUE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
        ON CONFLICT (feed_id, user_id) DO UPDATE
        SET is_read = TRUE, read_at = CURRENT_TIMESTAMP
    `

	if _, err = tx.Exec(ctx, updateFeedStatusQuery, feedID, user.UserID); err != nil {
		logger.SafeErrorContext(ctx, "Error updating feed status",
			"error", err,
			"user_id", user.UserID,
			"feed_id", feedID)
		return fmt.Errorf("failed to update feed status: %w", err)
	}

	if err = tx.Commit(context.Background()); err != nil {
		logger.SafeErrorContext(ctx, "Error committing transaction", "error", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	logger.SafeInfoContext(ctx, "feed status updated successfully",
		"user_id", user.UserID,
		"feed_id", feedID,
		"is_read", true)

	return nil
}
