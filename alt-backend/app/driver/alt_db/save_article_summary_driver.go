package alt_db

import (
	"alt/utils/logger"
	"context"
	"fmt"
)

// SaveArticleSummary saves an article summary to the database
// If a summary already exists for the article, it will be updated
func (r *AltDBRepository) SaveArticleSummary(ctx context.Context, articleID string, articleTitle string, summary string) error {
	if articleID == "" {
		return fmt.Errorf("article_id is required")
	}
	if summary == "" {
		return fmt.Errorf("summary cannot be empty")
	}

	query := `
		INSERT INTO article_summaries (article_id, article_title, summary_japanese)
		VALUES ($1, $2, $3)
		ON CONFLICT (article_id)
		DO UPDATE SET
			article_title = EXCLUDED.article_title,
			summary_japanese = EXCLUDED.summary_japanese,
			created_at = CURRENT_TIMESTAMP
	`

	commandTag, err := r.pool.Exec(ctx, query, articleID, articleTitle, summary)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to save article summary", "error", err, "article_id", articleID)
		return fmt.Errorf("failed to save article summary: %w", err)
	}

	if commandTag.RowsAffected() == 0 {
		logger.Logger.WarnContext(ctx, "No rows affected when saving article summary", "article_id", articleID)
	} else {
		logger.Logger.InfoContext(ctx, "Article summary saved successfully", "article_id", articleID, "rows_affected", commandTag.RowsAffected())
	}

	return nil
}
