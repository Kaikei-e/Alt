package alt_db

import (
	"alt/utils/logger"
	"context"

	"github.com/jackc/pgx/v5"
)

func (r *AltDBRepository) RegisterRSSFeedLink(ctx context.Context, link string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		logger.Logger.Error("Error starting transaction", "error", err)
		return pgx.ErrTxClosed
	}

	_, err = tx.Exec(ctx, "INSERT INTO feed_links (url) VALUES ($1)", link)
	if err != nil {
		logger.Logger.Error("Error registering RSS feed link", "error", err)
		return pgx.ErrTxClosed
	}

	err = tx.Commit(ctx)
	if err != nil {
		err = tx.Rollback(ctx)
		if err != nil {
			logger.Logger.Error("Error rolling back transaction", "error", err)
			return pgx.ErrTxClosed
		}
		logger.Logger.Error("Error committing transaction", "error", err)
		return pgx.ErrTxClosed
	}

	logger.Logger.Info("RSS feed link registered", "link", link)

	return nil
}
