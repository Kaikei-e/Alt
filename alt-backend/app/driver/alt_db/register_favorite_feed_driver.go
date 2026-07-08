package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (r *FeedRepository) RegisterFavoriteFeed(ctx context.Context, url string) (err error) {
	// Get user from context for multi-tenant support
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.SafeErrorContext(ctx, "user context not found", "error", err)
		return errors.New("authentication required")
	}

	cleanURL := strings.TrimSpace(url)
	if cleanURL == "" {
		logger.SafeErrorContext(ctx, "cannot register empty favorite feed url")
		return errors.New("empty url")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error starting transaction", "error", err)
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
				logger.SafeWarnContext(ctx, "Error rolling back transaction", "error", rbErr)
			}
		}
	}()

	var feedID string
	err = tx.QueryRow(ctx, "SELECT id FROM feeds WHERE website_url = $1", cleanURL).Scan(&feedID)
	if err != nil {
		logger.SafeErrorContext(ctx, "feed not found for URL", "error", err, "url", cleanURL)
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("query feed by url: %w", err)
	}

	// Insert with user_id for multi-tenant support
	// ON CONFLICT now uses composite primary key (user_id, feed_id)
	_, err = tx.Exec(ctx,
		"INSERT INTO favorite_feeds (user_id, feed_id) VALUES ($1, $2) ON CONFLICT (user_id, feed_id) DO NOTHING",
		user.UserID, feedID)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error inserting favorite feed", "error", err, "user_id", user.UserID)
		return fmt.Errorf("insert favorite feed: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		logger.SafeErrorContext(ctx, "Error committing transaction", "error", err)
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}

func (r *FeedRepository) RemoveFavoriteFeed(ctx context.Context, url string) (err error) {
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.SafeErrorContext(ctx, "user context not found", "error", err)
		return errors.New("authentication required")
	}

	cleanURL := strings.TrimSpace(url)
	if cleanURL == "" {
		logger.SafeErrorContext(ctx, "cannot remove empty favorite feed url")
		return errors.New("empty url")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error starting transaction", "error", err)
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
				logger.SafeWarnContext(ctx, "Error rolling back transaction", "error", rbErr)
			}
		}
	}()

	var feedID string
	err = tx.QueryRow(ctx, "SELECT id FROM feeds WHERE website_url = $1", cleanURL).Scan(&feedID)
	if err != nil {
		logger.SafeErrorContext(ctx, "feed not found for URL", "error", err, "url", cleanURL)
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("query feed by url: %w", err)
	}

	result, err := tx.Exec(ctx,
		"DELETE FROM favorite_feeds WHERE user_id = $1 AND feed_id = $2",
		user.UserID, feedID)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error deleting favorite feed", "error", err, "user_id", user.UserID)
		return fmt.Errorf("delete favorite feed: %w", err)
	}

	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	if err = tx.Commit(ctx); err != nil {
		logger.SafeErrorContext(ctx, "Error committing transaction", "error", err)
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}
