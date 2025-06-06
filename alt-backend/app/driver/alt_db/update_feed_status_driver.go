package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"
	"net/url"

	"github.com/jackc/pgx/v5"
)

func (r *AltDBRepository) UpdateFeedStatus(ctx context.Context, feedURL url.URL) error {
	identifyFeedQuery := `
		SELECT id FROM feeds WHERE feed_url = $1
	`
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		logger.Logger.Error("Error beginning transaction", "error", err)
		return err
	}
	defer tx.Rollback(ctx)

	var feedID string
	err = tx.QueryRow(ctx, identifyFeedQuery, feedURL.String()).Scan(&feedID)
	if err != nil {
		logger.Logger.Error("Error identifying feed", "error", err, "feedURL", feedURL.String())
		return errors.New("feed not found")
	}

	updateFeedStatusQuery := `
		INSERT INTO read_status (feed_id, is_read)
		VALUES ($1, TRUE)
		ON CONFLICT (feed_id) DO UPDATE SET is_read = TRUE
	`
	_, err = tx.Exec(ctx, updateFeedStatusQuery, feedID)
	if err != nil {
		logger.Logger.Error("Error updating feed status", "error", err, "feedID", feedID)
		return err
	}

	err = tx.Commit(ctx)
	if err != nil {
		logger.Logger.Error("Error committing transaction", "error", err)
		return err
	}

	return err
}
