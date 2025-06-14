package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (r *AltDBRepository) RegisterRSSFeedLink(ctx context.Context, link string) error {
	// Validate that the link is not empty or whitespace-only
	if strings.TrimSpace(link) == "" {
		logger.Logger.Error("Cannot register empty RSS feed link")
		return errors.New("RSS feed link cannot be empty")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		logger.Logger.Error("Error starting transaction", "error", err)
		return pgx.ErrTxClosed
	}

	// Ensure transaction is always cleaned up
	defer func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			logger.Logger.Warn("Error rolling back transaction", "error", err)
		}
	}()

	_, err = tx.Exec(ctx, "INSERT INTO feed_links (url) VALUES ($1)", link)
	if err != nil {
		logger.Logger.Error("Error registering RSS feed link", "error", err)
		return pgx.ErrTxClosed
	}

	err = tx.Commit(ctx)
	if err != nil {
		logger.Logger.Error("Error committing transaction", "error", err)
		return pgx.ErrTxClosed
	}

	logger.Logger.Info("RSS feed link registered", "link", link)

	return nil
}
