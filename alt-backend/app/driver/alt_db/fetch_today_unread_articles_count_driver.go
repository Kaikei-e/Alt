package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"time"
)

func (r *AltDBRepository) FetchTodayUnreadArticlesCount(ctx context.Context, since time.Time) (int, error) {
	// コンテキストからユーザー情報を取得
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.SafeError("user context not found", "error", err)
		return 0, errors.New("authentication required")
	}

	query := `
        SELECT COUNT(*)
        FROM feeds f
        LEFT JOIN read_status rs ON rs.feed_id = f.id AND rs.user_id = $2
        WHERE f.created_at >= $1
        AND (rs.feed_id IS NULL OR rs.is_read = FALSE)
    `

	var count int
	if err := r.pool.QueryRow(ctx, query, since, user.UserID).Scan(&count); err != nil {
		logger.SafeError("failed to fetch today's unread articles count",
			"error", err,
			"user_id", user.UserID)
		return 0, errors.New("failed to fetch today's unread articles count")
	}

	logger.SafeInfo("today's unread articles count fetched successfully",
		"count", count,
		"user_id", user.UserID)
	return count, nil
}
