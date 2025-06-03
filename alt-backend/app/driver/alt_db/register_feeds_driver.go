package alt_db

import (
	"alt/driver/models"
	"alt/utils/logger"
	"context"
)

func (r *AltDBRepository) RegisterSingleFeed(ctx context.Context, feed *models.Feed) error {
	_, err := r.db.Exec(ctx, "INSERT INTO feeds (title, description, link, pub_date, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6)", feed.Title, feed.Description, feed.Link, feed.PubDate, feed.CreatedAt, feed.UpdatedAt)
	if err != nil {
		logger.Logger.Error("Error registering single feed link", "error", err)
		return err
	}
	logger.Logger.Info("Single feed link registered", "link", feed.Link)

	return nil
}

func (r *AltDBRepository) RegisterMultipleFeeds(ctx context.Context, feeds []models.Feed) error {
	for _, feed := range feeds {
		err := r.RegisterSingleFeed(ctx, &feed)
		if err != nil {
			logger.Logger.Error("Error registering multiple feeds", "error", err)
			return err
		}
	}
	return nil
}
