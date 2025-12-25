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
func (r *AltDBRepository) UpsertArticleWithTx(ctx context.Context, tx pgx.Tx, title, content, url string, userID uuid.UUID, feedID *uuid.UUID) (uuid.UUID, error) {
	var articleID uuid.UUID
	var feedIDValue interface{}
	if feedID != nil {
		feedIDValue = *feedID
	} else {
		feedIDValue = nil
	}

	cleanTitle := strings.TrimSpace(title)
	cleanContent := strings.TrimSpace(content)
	cleanURL := strings.TrimSpace(url)

	if err := tx.QueryRow(ctx, upsertArticleQuery, cleanTitle, cleanContent, cleanURL, userID, feedIDValue).Scan(&articleID); err != nil {
		err = fmt.Errorf("upsert article content: %w", err)
		logger.SafeError("failed to save article", "url", cleanURL, "error", err)
		return uuid.Nil, err
	}

	return articleID, nil
}
