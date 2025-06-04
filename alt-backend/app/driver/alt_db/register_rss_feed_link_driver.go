package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"
)

func (r *AltDBRepository) RegisterRSSFeedLink(ctx context.Context, link string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		logger.Logger.Error("Error starting transaction", "error", err)
		return errors.New("error starting transaction")
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, "INSERT INTO feed_links (url) VALUES ($1)", link)
	if err != nil {
		logger.Logger.Error("Error registering RSS feed link", "error", err)
		return errors.New("error registering RSS feed link")
	}

	logger.Logger.Info("RSS feed link registered", "link", link)

	return nil
}
