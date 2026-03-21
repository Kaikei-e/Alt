package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
)

func (r *AltDBRepository) FetchTotalArticlesCount(ctx context.Context) (int, error) {
	if r == nil || r.pool == nil {
		return 0, errors.New("database connection not available")
	}

	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return 0, errors.New("authentication required")
	}

	query := `SELECT COUNT(*) FROM articles WHERE user_id = $1 AND deleted_at IS NULL`

	var count int
	err = r.pool.QueryRow(ctx, query, user.UserID).Scan(&count)
	if err != nil {
		logger.SafeErrorContext(ctx, "failed to fetch total articles count", "error", err)
		return 0, errors.New("failed to fetch total articles count")
	}

	logger.SafeInfoContext(ctx, "total articles count fetched successfully", "count", count)
	return count, nil
}
