package alt_db

import (
	"alt/driver/models"
	"alt/utils/logger"
	"context"
	"errors"
)

func (r *AltDBRepository) RegisterSingleFeed(ctx context.Context, feed *models.Feed) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		logger.Logger.Error("Error starting transaction", "error", err)
		return errors.New("error starting transaction")
	}

	defer func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			logger.Logger.Warn("Error rolling back transaction", "error", err)
		}
	}()

	var existingID string
	err = tx.QueryRow(ctx, "SELECT id FROM feeds WHERE link = $1", feed.Link).Scan(&existingID)
	if err == nil {
		_, err = tx.Exec(ctx, "UPDATE feeds SET title = $1, description = $2, pub_date = $3, updated_at = $4 WHERE link = $5",
			feed.Title, feed.Description, feed.PubDate, feed.UpdatedAt, feed.Link)
		if err != nil {
			logger.Logger.Error("Error updating existing feed", "error", err)
			return errors.New("error updating existing feed")
		}
		logger.Logger.Info("Existing feed updated", "link", feed.Link)
	} else {
		_, err = tx.Exec(ctx, "INSERT INTO feeds (title, description, link, pub_date, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6)",
			feed.Title, feed.Description, feed.Link, feed.PubDate, feed.CreatedAt, feed.UpdatedAt)
		if err != nil {
			logger.Logger.Error("Error inserting new feed", "error", err)
			return errors.New("error inserting new feed")
		}
		logger.Logger.Info("New feed inserted", "link", feed.Link)
	}

	err = tx.Commit(ctx)
	if err != nil {
		logger.Logger.Error("Error committing transaction", "error", err)
		return errors.New("error committing transaction")
	}

	return nil
}

func (r *AltDBRepository) RegisterMultipleFeeds(ctx context.Context, feeds []models.Feed) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		logger.Logger.Error("Error starting transaction", "error", err)
		return errors.New("error starting transaction")
	}

	defer func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			logger.Logger.Warn("Error rolling back transaction", "error", err)
		}
	}()

	for _, feed := range feeds {
		var existingID string
		err = tx.QueryRow(ctx, "SELECT id FROM feeds WHERE link = $1", feed.Link).Scan(&existingID)
		if err == nil {
			_, err = tx.Exec(ctx, "UPDATE feeds SET title = $1, description = $2, pub_date = $3, updated_at = $4 WHERE link = $5",
				feed.Title, feed.Description, feed.PubDate, feed.UpdatedAt, feed.Link)
			if err != nil {
				logger.Logger.Error("Error updating existing feed", "error", err)
				return errors.New("error updating existing feed")
			}
			logger.Logger.Info("Existing feed updated", "link", feed.Link)
		} else {
			_, err = tx.Exec(ctx, "INSERT INTO feeds (title, description, link, pub_date, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6)",
				feed.Title, feed.Description, feed.Link, feed.PubDate, feed.CreatedAt, feed.UpdatedAt)
			if err != nil {
				logger.Logger.Error("Error inserting new feed", "error", err)
				return errors.New("error inserting new feed")
			}
			logger.Logger.Info("New feed inserted", "link", feed.Link)
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		logger.Logger.Error("Error committing transaction", "error", err)
		return errors.New("error committing transaction")
	}

	logger.Logger.Info("Multiple feeds registered successfully", "count", len(feeds))
	return nil
}
