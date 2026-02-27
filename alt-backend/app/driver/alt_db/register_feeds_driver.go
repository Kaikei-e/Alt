package alt_db

import (
	"alt/driver/models"
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func (r *AltDBRepository) RegisterSingleFeed(ctx context.Context, feed *models.Feed) error {
	// Use ON CONFLICT for atomic upsert, eliminating TOCTOU race condition.
	// Same pattern as RegisterMultipleFeeds.
	const upsertQuery = `
		INSERT INTO feeds (title, description, link, pub_date, created_at, updated_at, feed_link_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (link) DO UPDATE SET
			title = EXCLUDED.title,
			description = EXCLUDED.description,
			pub_date = EXCLUDED.pub_date,
			updated_at = EXCLUDED.updated_at,
			feed_link_id = COALESCE(feeds.feed_link_id, EXCLUDED.feed_link_id)
	`

	if _, err := r.pool.Exec(ctx, upsertQuery,
		feed.Title, feed.Description, feed.Link, feed.PubDate, feed.CreatedAt, feed.UpdatedAt, feed.FeedLinkID,
	); err != nil {
		return fmt.Errorf("upsert feed: %w", err)
	}

	logger.Logger.InfoContext(ctx, "Feed upserted", "link", feed.Link)
	return nil
}

func (r *AltDBRepository) RegisterMultipleFeeds(ctx context.Context, feeds []models.Feed) error {
	if len(feeds) == 0 {
		return nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	defer func() {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			logger.Logger.WarnContext(ctx, "rollback failed", "error", rbErr)
		}
	}()

	// Batch UPSERT: eliminates N+1 SELECT→INSERT/UPDATE pattern
	// COALESCE preserves existing feed_link_id if already set (prevents overwrite)
	const upsertQuery = `
		INSERT INTO feeds (title, description, link, pub_date, created_at, updated_at, feed_link_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (link) DO UPDATE SET
			title = EXCLUDED.title,
			description = EXCLUDED.description,
			pub_date = EXCLUDED.pub_date,
			updated_at = EXCLUDED.updated_at,
			feed_link_id = COALESCE(feeds.feed_link_id, EXCLUDED.feed_link_id)
	`

	batch := &pgx.Batch{}
	for _, feed := range feeds {
		batch.Queue(upsertQuery, feed.Title, feed.Description, feed.Link, feed.PubDate, feed.CreatedAt, feed.UpdatedAt, feed.FeedLinkID)
	}

	br := tx.SendBatch(ctx, batch)
	for range feeds {
		if _, err := br.Exec(); err != nil {
			br.Close()
			return fmt.Errorf("batch upsert feed: %w", err)
		}
	}
	if err := br.Close(); err != nil {
		return fmt.Errorf("close batch: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	logger.Logger.InfoContext(ctx, "Multiple feeds registered successfully", "count", len(feeds))
	return nil
}
