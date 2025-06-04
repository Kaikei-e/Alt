package alt_db

import (
	"alt/driver/models"
	"alt/utils/logger"
	"context"
	"errors"
)

func (r *AltDBRepository) RegisterSingleFeed(ctx context.Context, feed *models.Feed) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		logger.Logger.Error("Error starting transaction", "error", err)
		return errors.New("error starting transaction")
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, "INSERT INTO feeds (title, description, link, pub_date, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (link) DO NOTHING", feed.Title, feed.Description, feed.Link, feed.PubDate, feed.CreatedAt, feed.UpdatedAt)
	if err != nil {
		logger.Logger.Error("Error registering single feed link", "error", err)
		return errors.New("error registering single feed link")
	}
	logger.Logger.Info("Single feed link registered", "link", feed.Link)

	err = tx.Commit(ctx)
	if err != nil {
		logger.Logger.Error("Error committing transaction", "error", err)
		return errors.New("error committing transaction")
	}

	return nil
}

func (r *AltDBRepository) RegisterMultipleFeeds(ctx context.Context, feeds []models.Feed) error {
	for _, feed := range feeds {
		err := r.RegisterSingleFeed(ctx, &feed)
		if err != nil {
			logger.Logger.Error("Error registering multiple feeds", "error", err)
			return errors.New("error registering multiple feeds")
		}
	}
	return nil
}
