package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"
)

func (r *AltDBRepository) FetchUnsummarizedArticlesCount(ctx context.Context) (int, error) {
	query := `
		SELECT COUNT(*) - (SELECT COUNT(*) FROM article_summaries)
		FROM articles
	`

	var count int
	err := r.pool.QueryRow(ctx, query).Scan(&count)
	if err != nil {
		logger.SafeError("failed to fetch unsummarized feeds count", "error", err)
		return 0, errors.New("failed to fetch unsummarized feeds count")
	}

	logger.SafeInfo("unsummarized feeds count fetched successfully", "count", count)
	return count, nil
}
