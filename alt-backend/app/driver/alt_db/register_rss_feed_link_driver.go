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
		logger.Logger.ErrorContext(ctx, "Cannot register empty RSS feed link")
		return errors.New("RSS feed link cannot be empty")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Error starting transaction", "error", err)
		return pgx.ErrTxClosed
	}

	// Ensure transaction is always cleaned up
	defer func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			logger.Logger.WarnContext(ctx, "Error rolling back transaction", "error", err)
		}
	}()

	_, err = tx.Exec(ctx, "INSERT INTO feed_links (url) VALUES ($1)", link)
	if err != nil {
		// Check for duplicate key error (SQLSTATE 23505)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// Duplicate key error - this is a normal case, feed link already exists
			logger.Logger.InfoContext(ctx, "RSS feed link already exists (duplicate)", "link", link, "sqlstate", pgErr.Code)
			// Do not commit, as the transaction is aborted. Rollback is handled by defer or we can just return.
			return nil // Return nil to indicate success (feed already registered)
		}
		// Other database errors
		logger.Logger.ErrorContext(ctx, "Error registering RSS feed link", "error", err, "link", link)
		return errors.New("failed to register RSS feed link: " + err.Error())
	}

	err = tx.Commit(ctx)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Error committing transaction", "error", err)
		return pgx.ErrTxClosed
	}

	logger.Logger.InfoContext(ctx, "RSS feed link registered", "link", link)

	return nil
}
