package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (r *AltDBRepository) RegisterFavoriteFeed(ctx context.Context, url string) (err error) {
	cleanURL := strings.TrimSpace(url)
	if cleanURL == "" {
		logger.SafeError("cannot register empty favorite feed url")
		return errors.New("empty url")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		logger.SafeError("Error starting transaction", "error", err)
		return pgx.ErrTxClosed
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil && rbErr.Error() != "tx is closed" {
				logger.SafeWarn("Error rolling back transaction", "error", rbErr)
			}
		}
	}()

	var feedID string
	err = tx.QueryRow(ctx, "SELECT id FROM feeds WHERE link = $1", cleanURL).Scan(&feedID)
	if err != nil {
		logger.SafeError("feed not found for URL", "error", err, "url", cleanURL)
		return pgx.ErrNoRows
	}

	_, err = tx.Exec(ctx, "INSERT INTO favorite_feeds (feed_id) VALUES ($1) ON CONFLICT DO NOTHING", feedID)
	if err != nil {
		logger.SafeError("Error inserting favorite feed", "error", err)
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		logger.SafeError("Error committing transaction", "error", err)
		return err
	}

	return nil
}
