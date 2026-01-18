package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"
)

func (r *AltDBRepository) FetchUnsummarizedArticlesCount(ctx context.Context) (int, error) {
	if r == nil || r.pool == nil {
		return 0, errors.New("database connection not available")
	}

	query := `
		SELECT COUNT(*)
		FROM articles a
		LEFT JOIN article_summaries s ON a.id = s.article_id
		WHERE s.article_id IS NULL
	`

	var count int
	err := r.pool.QueryRow(ctx, query).Scan(&count)
	if err != nil {
		logger.SafeErrorContext(ctx, "failed to fetch unsummarized articles count", "error", err)
		return 0, errors.New("failed to fetch unsummarized articles count")
	}

	logger.SafeInfoContext(ctx, "unsummarized articles count fetched successfully", "count", count)
	return count, nil
}
