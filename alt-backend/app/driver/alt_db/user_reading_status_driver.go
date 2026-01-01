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

// MarkArticleAsRead marks an article as read for the current user.
// It resolves the article by URL and upserts a record in user_reading_status.
func (r *AltDBRepository) MarkArticleAsRead(ctx context.Context, articleURL url.URL) error {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.SafeError("user context not found", "error", err)
		return errors.New("authentication required")
	}

	// Normalize the input URL
	normalizedURL, err := utils.NormalizeURL(articleURL.String())
	if err != nil {
		logger.SafeError("Error normalizing article URL", "error", err, "articleURL", articleURL.String())
		return err
	}

	// Resolve article ID from URL
	getArticleQuery := `SELECT id FROM articles WHERE url = $1`

	var articleID string
	err = r.pool.QueryRow(ctx, getArticleQuery, normalizedURL).Scan(&articleID)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logger.SafeError("Article not found",
				"normalizedURL", normalizedURL,
				"originalURL", articleURL.String(),
				"user_id", user.UserID)
			return domain.ErrArticleNotFound
		}
		logger.SafeError("Error querying article", "error", err, "normalizedURL", normalizedURL)
		return fmt.Errorf("failed to query article: %w", err)
	}

	logger.SafeInfo("Found matching article",
		"articleID", articleID,
		"normalizedURL", normalizedURL)

	// Start transaction for upsert
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		logger.SafeError("Error beginning transaction", "error", err)
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if err := tx.Rollback(context.Background()); err != nil && err.Error() != "tx is closed" {
			logger.SafeWarn("Error rolling back transaction", "error", err)
		}
	}()

	// Upsert user reading status
	upsertQuery := `
		INSERT INTO user_reading_status (user_id, article_id, is_read, read_at, created_at)
		VALUES ($1, $2, TRUE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id, article_id) DO UPDATE
		SET is_read = TRUE, read_at = CURRENT_TIMESTAMP
	`

	if _, err = tx.Exec(ctx, upsertQuery, user.UserID, articleID); err != nil {
		logger.SafeError("Error updating article reading status",
			"error", err,
			"user_id", user.UserID,
			"article_id", articleID)
		return fmt.Errorf("failed to update article reading status: %w", err)
	}

	if err = tx.Commit(context.Background()); err != nil {
		logger.SafeError("Error committing transaction", "error", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	logger.SafeInfo("article reading status updated successfully",
		"user_id", user.UserID,
		"article_id", articleID,
		"is_read", true)

	return nil
}
