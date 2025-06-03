package alt_db

import (
	"alt/utils/logger"
	"context"
)

func (r *AltDBRepository) RegisterRSSFeedLink(ctx context.Context, link string) error {
	_, err := r.db.Exec(ctx, "INSERT INTO feed_links (url) VALUES ($1)", link)
	if err != nil {
		logger.Logger.Error("Error registering RSS feed link", "error", err)
		return err
	}

	logger.Logger.Info("RSS feed link registered", "link", link)

	return nil
}
