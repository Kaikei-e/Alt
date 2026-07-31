package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// RegisterFavoriteFeed stars the feed at a URL for one user.
//
// The SELECT stays, unlike the read-status writes capability catalog §4-5
// collapsed onto a single statement. Here the insert is ON CONFLICT DO
// NOTHING, so a zero RowsAffected() means either "no such feed" or "already a
// favourite" — a NotFound and a success — and the two cannot be told apart
// from the count. The pair is what the transaction is for: two statements a
// caller must not see half of, where UpdateFeedStatus's transaction wrapped
// one.
//
// userID is an argument rather than a context lookup for the same reason as
// MarkArticleAsRead: over Connect the peer certificate names alt-backend, not
// the person whose favourites these are.
func (r *FeedRepository) RegisterFavoriteFeed(ctx context.Context, url string, userID uuid.UUID) (err error) {
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
		userID, feedID)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error inserting favorite feed", "error", err, "user_id", userID)
		return fmt.Errorf("insert favorite feed: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		logger.SafeErrorContext(ctx, "Error committing transaction", "error", err)
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}

// RemoveFavoriteFeed unstars it. pgx.ErrNoRows covers both a URL with no feed
// and a feed the user had not starred; the caller has never distinguished the
// two, and the gateway maps both to the same NotFound.
func (r *FeedRepository) RemoveFavoriteFeed(ctx context.Context, url string, userID uuid.UUID) (err error) {
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
		userID, feedID)
	if err != nil {
		logger.SafeErrorContext(ctx, "Error deleting favorite feed", "error", err, "user_id", userID)
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
