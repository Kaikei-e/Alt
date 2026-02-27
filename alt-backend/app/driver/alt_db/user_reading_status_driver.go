package alt_db

import (
	"alt/domain"
	"alt/utils"
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"
	"net/url"
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

	// Single-query upsert: resolve feed by URL and insert/update read_status atomically.
	// Eliminates the need for a separate SELECT + transaction.
	upsertQuery := `
		INSERT INTO read_status (feed_id, user_id, is_read, read_at, created_at)
		SELECT f.id, $2, TRUE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		FROM feeds f WHERE f.link = $1
		ON CONFLICT (feed_id, user_id) DO UPDATE
		SET is_read = TRUE, read_at = CURRENT_TIMESTAMP
	`

	tag, err := r.pool.Exec(ctx, upsertQuery, normalizedURL, user.UserID)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error updating feed read status",
			"error", err,
			"user_id", user.UserID,
			"normalizedURL", normalizedURL)
		return fmt.Errorf("failed to update feed read status: %w", err)
	}

	if tag.RowsAffected() == 0 {
		logger.SafeErrorContext(ctx, "Feed not found",
			"normalizedURL", normalizedURL,
			"originalURL", originalURL,
			"user_id", user.UserID)
		return domain.ErrFeedNotFound
	}

	logger.SafeInfoContext(ctx, "feed read status updated successfully",
		"user_id", user.UserID,
		"normalizedURL", normalizedURL,
		"is_read", true)

	return nil
}
