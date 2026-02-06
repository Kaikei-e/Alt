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
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	defer func() {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			logger.Logger.WarnContext(ctx, "rollback failed", "error", rbErr)
		}
	}()

	var existingID string
	err = tx.QueryRow(ctx, "SELECT id FROM feeds WHERE link = $1", feed.Link).Scan(&existingID)
	if err == nil {
		_, err = tx.Exec(ctx, "UPDATE feeds SET title = $1, description = $2, pub_date = $3, updated_at = $4 WHERE link = $5",
			feed.Title, feed.Description, feed.PubDate, feed.UpdatedAt, feed.Link)
		if err != nil {
			return fmt.Errorf("update existing feed: %w", err)
		}
		logger.Logger.InfoContext(ctx, "Existing feed updated", "link", feed.Link)
	} else {
		_, err = tx.Exec(ctx, "INSERT INTO feeds (title, description, link, pub_date, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6)",
			feed.Title, feed.Description, feed.Link, feed.PubDate, feed.CreatedAt, feed.UpdatedAt)
		if err != nil {
			return fmt.Errorf("insert new feed: %w", err)
		}
		logger.Logger.InfoContext(ctx, "New feed inserted", "link", feed.Link)
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

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
	const upsertQuery = `
		INSERT INTO feeds (title, description, link, pub_date, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (link) DO UPDATE SET
			title = EXCLUDED.title,
			description = EXCLUDED.description,
			pub_date = EXCLUDED.pub_date,
			updated_at = EXCLUDED.updated_at
	`

	batch := &pgx.Batch{}
	for _, feed := range feeds {
		batch.Queue(upsertQuery, feed.Title, feed.Description, feed.Link, feed.PubDate, feed.CreatedAt, feed.UpdatedAt)
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
