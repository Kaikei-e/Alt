package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"
)

func (r *AltDBRepository) FetchTotalArticlesCount(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM articles`

	var count int
	err := r.pool.QueryRow(ctx, query).Scan(&count)
	if err != nil {
		logger.SafeError("failed to fetch total articles count", "error", err)
		return 0, errors.New("failed to fetch total articles count")
	}

	logger.SafeInfo("total articles count fetched successfully", "count", count)
	return count, nil
}
