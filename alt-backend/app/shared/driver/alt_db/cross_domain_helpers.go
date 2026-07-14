package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// getFeedIDByArticleURL is a package-level helper that resolves a feed ID from an article URL.
// Used by ArticleRepository.SaveArticle to resolve feed ownership without cross-domain coupling.
func getFeedIDByArticleURL(ctx context.Context, pool PgxIface, articleURL string) (string, error) {
	if pool == nil {
		return "", errors.New("database connection not available")
	}

	query := `SELECT id FROM feeds WHERE website_url = $1`

	var feedID string
	err := pool.QueryRow(ctx, query, articleURL).Scan(&feedID)
	if err != nil {
		logger.SafeErrorContext(ctx, "error getting feed ID by article URL", "error", err, "articleURL", articleURL)
		return "", errors.New("error getting feed ID by article URL")
	}

	logger.SafeInfoContext(ctx, "retrieved feed ID by article URL", "articleURL", articleURL, "feedID", feedID)
	return feedID, nil
}

// saveOutboxEventWithTx is a package-level helper that inserts an outbox event within a transaction.
// Used by ArticleRepository.SaveArticle to publish domain events without cross-domain coupling.
func saveOutboxEventWithTx(ctx context.Context, tx pgx.Tx, eventType string, payload []byte) error {
	if _, err := tx.Exec(ctx, insertOutboxQuery, eventType, string(payload)); err != nil {
		err = fmt.Errorf("failed to insert outbox event: %w", err)
		logger.SafeErrorContext(ctx, "failed to save outbox event", "event_type", eventType, "error", err)
		return err
	}
	return nil
}
