package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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
		// Check for duplicate key error (SQLSTATE 23505)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// Duplicate key error - this is a normal case, feed link already exists
			logger.Logger.Info("RSS feed link already exists (duplicate)", "link", link, "sqlstate", pgErr.Code)
			// Commit the transaction since this is not an error condition
			// Note: defer will attempt to rollback, but it will be ignored since tx is closed after commit
			if commitErr := tx.Commit(ctx); commitErr != nil {
				logger.Logger.Error("Error committing transaction after duplicate detection", "error", commitErr)
				return errors.New("error committing transaction after duplicate detection")
			}
			// Set err to nil to prevent defer from rolling back (though it will be ignored anyway)
			err = nil
			return nil // Return nil to indicate success (feed already registered)
		}
		// Other database errors
		logger.Logger.Error("Error registering RSS feed link", "error", err, "link", link)
		return errors.New("failed to register RSS feed link: " + err.Error())
	}

	err = tx.Commit(ctx)
	if err != nil {
		logger.Logger.Error("Error committing transaction", "error", err)
		return pgx.ErrTxClosed
	}

	logger.Logger.Info("RSS feed link registered", "link", link)

	return nil
}
