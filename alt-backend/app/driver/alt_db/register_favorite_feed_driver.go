package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (r *AltDBRepository) RegisterFavoriteFeed(ctx context.Context, url string) (err error) {
	// Get user from context for multi-tenant support
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.SafeError("user context not found", "error", err)
		return errors.New("authentication required")
	}

	cleanURL := strings.TrimSpace(url)
	if cleanURL == "" {
		logger.SafeError("cannot register empty favorite feed url")
		return errors.New("empty url")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		logger.SafeError("Error starting transaction", "error", err)
		return pgx.ErrTxClosed
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil && rbErr.Error() != "tx is closed" {
				logger.SafeWarn("Error rolling back transaction", "error", rbErr)
			}
		}
	}()

	var feedID string
	err = tx.QueryRow(ctx, "SELECT id FROM feeds WHERE link = $1", cleanURL).Scan(&feedID)
	if err != nil {
		logger.SafeError("feed not found for URL", "error", err, "url", cleanURL)
		return pgx.ErrNoRows
	}

	// Insert with user_id for multi-tenant support
	// ON CONFLICT now uses composite primary key (user_id, feed_id)
	_, err = tx.Exec(ctx,
		"INSERT INTO favorite_feeds (user_id, feed_id) VALUES ($1, $2) ON CONFLICT (user_id, feed_id) DO NOTHING",
		user.UserID, feedID)
	if err != nil {
		logger.SafeError("Error inserting favorite feed", "error", err, "user_id", user.UserID)
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		logger.SafeError("Error committing transaction", "error", err)
		return err
	}

	return nil
}
