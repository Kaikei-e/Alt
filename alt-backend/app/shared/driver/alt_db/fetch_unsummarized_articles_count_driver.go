package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
)

func (r *DashboardRepository) FetchUnsummarizedArticlesCount(ctx context.Context) (int, error) {
	if r == nil || r.pool == nil {
		return 0, errors.New("database connection not available")
	}

	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		return 0, errors.New("authentication required")
	}

	query := `
		SELECT COUNT(*)
		FROM articles a
		LEFT JOIN article_summaries s ON a.id = s.article_id AND s.user_id = a.user_id
		WHERE a.user_id = $1 AND a.deleted_at IS NULL AND s.article_id IS NULL
	`

	var count int
	err = r.pool.QueryRow(ctx, query, user.UserID).Scan(&count)
	if err != nil {
		logger.SafeErrorContext(ctx, "failed to fetch unsummarized articles count", "error", err)
		return 0, errors.New("failed to fetch unsummarized articles count")
	}

	logger.SafeInfoContext(ctx, "unsummarized articles count fetched successfully", "count", count)
	return count, nil
}
