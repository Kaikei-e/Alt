package alt_db

import (
	"alt/domain"
	"alt/utils"
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

	// Normalize the input URL to match against database URLs
	normalizedInputURL, err := utils.NormalizeURL(feedURL.String())
	if err != nil {
		logger.SafeError("Error normalizing input URL", "error", err, "feedURL", feedURL.String())
		return err
	}

	// Get all feeds and find matching normalized URL
	getAllFeedsQuery := `SELECT id, link FROM feeds`

	rows, err := r.pool.Query(ctx, getAllFeedsQuery)
	if err != nil {
		logger.SafeError("Error querying feeds", "error", err)
		return err
	}
	defer rows.Close()

	// Find matching feed by comparing normalized URLs
	var feedID string
	var foundMatch bool

	for rows.Next() {
		var dbFeedID, dbFeedLink string
		if err := rows.Scan(&dbFeedID, &dbFeedLink); err != nil {
			logger.SafeError("Error scanning feed row", "error", err)
			continue
		}

		// Normalize the database URL
		normalizedDBURL, err := utils.NormalizeURL(dbFeedLink)
		if err != nil {
			logger.SafeInfo("Error normalizing database URL", "error", err, "dbFeedLink", dbFeedLink)
			continue
		}

		// Compare normalized URLs (case-insensitive for percent-encoding)
		if utils.URLsEqual(normalizedDBURL, normalizedInputURL) {
			feedID = dbFeedID
			foundMatch = true
			logger.SafeInfo("Found matching feed",
				"feedID", feedID,
				"inputURL", normalizedInputURL,
				"dbURL", normalizedDBURL)
			break
		}
	}

	if !foundMatch {
		logger.SafeError("Feed not found after URL normalization",
			"normalizedInputURL", normalizedInputURL,
			"originalInputURL", feedURL.String())
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
