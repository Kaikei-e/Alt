package alt_db

import (
	"alt/utils/logger"
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// UpsertArticleWithTx inserts or updates an article using a provided transaction.
// The returned bool is true when a new row was inserted.
func (r *ArticleRepository) UpsertArticleWithTx(ctx context.Context, tx pgx.Tx, title, content, url string, userID uuid.UUID, feedID *uuid.UUID) (uuid.UUID, bool, error) {
	var articleID uuid.UUID
	var created bool
	var feedIDValue interface{}
	if feedID != nil {
		feedIDValue = *feedID
	} else {
		feedIDValue = nil
	}

	cleanTitle := strings.TrimSpace(title)
	cleanContent := strings.TrimSpace(content)
	cleanURL := strings.TrimSpace(url)

	if err := tx.QueryRow(ctx, upsertArticleQuery, cleanTitle, cleanContent, cleanURL, userID, feedIDValue).Scan(&articleID, &created); err != nil {
		err = fmt.Errorf("upsert article content: %w", err)
		logger.SafeErrorContext(ctx, "failed to save article", "url", cleanURL, "error", err)
		return uuid.Nil, false, err
	}

	return articleID, created, nil
}
