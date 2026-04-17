package alt_db

import (
	"alt/driver/models"
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type FeedRegistrationResult struct {
	ArticleID string
	Created   bool
}

func (r *FeedRepository) RegisterSingleFeed(ctx context.Context, feed *models.Feed) error {
	// Use ON CONFLICT for atomic upsert, eliminating TOCTOU race condition.
	// Same pattern as RegisterMultipleFeeds.
	const upsertQuery = `
		INSERT INTO feeds (title, description, link, pub_date, created_at, updated_at, feed_link_id, og_image_url)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (link) DO UPDATE SET
			title = EXCLUDED.title,
			description = EXCLUDED.description,
			pub_date = EXCLUDED.pub_date,
			updated_at = EXCLUDED.updated_at,
			feed_link_id = COALESCE(feeds.feed_link_id, EXCLUDED.feed_link_id),
			og_image_url = COALESCE(EXCLUDED.og_image_url, feeds.og_image_url)
	`

	if _, err := r.pool.Exec(ctx, upsertQuery,
		feed.Title, feed.Description, feed.Link, feed.PubDate, feed.CreatedAt, feed.UpdatedAt, feed.FeedLinkID, feed.OgImageURL,
	); err != nil {
		return fmt.Errorf("upsert feed: %w", err)
	}

	logger.Logger.InfoContext(ctx, "Feed upserted", "link", feed.Link)
	return nil
}

func (r *FeedRepository) RegisterMultipleFeeds(ctx context.Context, feeds []models.Feed) ([]string, error) {
	results, err := r.RegisterMultipleFeedsWithState(ctx, feeds)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(results))
	for _, result := range results {
		ids = append(ids, result.ArticleID)
	}

	return ids, nil
}

func (r *FeedRepository) RegisterMultipleFeedsWithState(ctx context.Context, feeds []models.Feed) ([]FeedRegistrationResult, error) {
	if len(feeds) == 0 {
		return nil, nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}

	defer func() {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			logger.Logger.WarnContext(ctx, "rollback failed", "error", rbErr)
		}
	}()

	// Batch UPSERT: eliminates N+1 SELECT→INSERT/UPDATE pattern
	// COALESCE preserves existing og_image_url/feed_link_id if already set
	// RETURNING id: returns the id for both INSERT and ON CONFLICT UPDATE cases
	const upsertQuery = `
		INSERT INTO feeds (title, description, link, pub_date, created_at, updated_at, feed_link_id, og_image_url)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (link) DO UPDATE SET
			title = EXCLUDED.title,
			description = EXCLUDED.description,
			pub_date = EXCLUDED.pub_date,
			updated_at = EXCLUDED.updated_at,
			feed_link_id = COALESCE(feeds.feed_link_id, EXCLUDED.feed_link_id),
			og_image_url = COALESCE(EXCLUDED.og_image_url, feeds.og_image_url)
		RETURNING id, (xmax = 0) AS created
	`

	batch := &pgx.Batch{}
	for _, feed := range feeds {
		batch.Queue(upsertQuery, feed.Title, feed.Description, feed.Link, feed.PubDate, feed.CreatedAt, feed.UpdatedAt, feed.FeedLinkID, feed.OgImageURL)
	}

	br := tx.SendBatch(ctx, batch)
	results := make([]FeedRegistrationResult, 0, len(feeds))
	for range feeds {
		var result FeedRegistrationResult
		if err := br.QueryRow().Scan(&result.ArticleID, &result.Created); err != nil {
			_ = br.Close() //#nosec G104 -- Scan error already surfaced; Close error is noise
			return nil, fmt.Errorf("batch upsert feed: %w", err)
		}
		results = append(results, result)
	}
	if err := br.Close(); err != nil {
		return nil, fmt.Errorf("close batch: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	logger.Logger.InfoContext(ctx, "Multiple feeds registered successfully", "count", len(feeds))
	return results, nil
}
