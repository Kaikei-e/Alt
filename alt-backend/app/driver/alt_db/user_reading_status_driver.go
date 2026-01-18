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

// MarkArticleAsRead marks a feed as read for the current user.
// It resolves the feed by URL (feeds.link) and upserts a record in read_status.
// Note: This function is named MarkArticleAsRead for API compatibility, but it
// operates on feeds (not articles) because not all feeds have corresponding articles.
func (r *AltDBRepository) MarkArticleAsRead(ctx context.Context, articleURL url.URL) error {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.SafeErrorContext(ctx, "user context not found", "error", err)
		return errors.New("authentication required")
	}

	// Zero-trust: Normalize the input URL (removes UTM parameters, trailing slashes, etc.)
	originalURL := articleURL.String()
	normalizedURL, err := utils.NormalizeURL(originalURL)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error normalizing feed URL", "error", err, "feedURL", originalURL)
		return err
	}

	// Resolve feed ID from URL using normalized URL only (DB should have normalized URLs)
	getFeedQuery := `SELECT id FROM feeds WHERE link = $1 LIMIT 1`

	var feedID string
	err = r.pool.QueryRow(ctx, getFeedQuery, normalizedURL).Scan(&feedID)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logger.SafeErrorContext(ctx, "Feed not found",
				"normalizedURL", normalizedURL,
				"originalURL", originalURL,
				"user_id", user.UserID)
			return domain.ErrFeedNotFound
		}
		logger.SafeErrorContext(ctx, "Error querying feed", "error", err, "normalizedURL", normalizedURL)
		return fmt.Errorf("failed to query feed: %w", err)
	}

	logger.SafeInfoContext(ctx, "Found matching feed",
		"feedID", feedID,
		"normalizedURL", normalizedURL)

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
	upsertQuery := `
		INSERT INTO read_status (feed_id, user_id, is_read, read_at, created_at)
		VALUES ($1, $2, TRUE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (feed_id, user_id) DO UPDATE
		SET is_read = TRUE, read_at = CURRENT_TIMESTAMP
	`

	if _, err = tx.Exec(ctx, upsertQuery, feedID, user.UserID); err != nil {
		logger.SafeErrorContext(ctx, "Error updating feed read status",
			"error", err,
			"user_id", user.UserID,
			"feed_id", feedID)
		return fmt.Errorf("failed to update feed read status: %w", err)
	}

	if err = tx.Commit(context.Background()); err != nil {
		logger.SafeErrorContext(ctx, "Error committing transaction", "error", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	logger.SafeInfoContext(ctx, "feed read status updated successfully",
		"user_id", user.UserID,
		"feed_id", feedID,
		"is_read", true)

	return nil
}
