package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"
)

func (r *AltDBRepository) FetchSummarizedArticlesCount(ctx context.Context) (int, error) {
	query := `
		SELECT COUNT(*) FROM article_summaries
	`

	var count int
	err := r.pool.QueryRow(ctx, query).Scan(&count)
	if err != nil {
		logger.SafeError("failed to fetch summarized articles count", "error", err)
		return 0, errors.New("failed to fetch summarized articles count")
	}

	logger.SafeInfo("summarized articles count fetched successfully", "count", count)
	return count, nil
}
