package alt_db

import (
	"alt/utils"
	"alt/utils/logger"
	"context"
	"errors"
	"time"
)

func (r *AltDBRepository) FetchTodayUnreadArticlesCount(ctx context.Context, since time.Time) (int, error) {
	query := `
                        SELECT COUNT(*)
                        FROM feeds f
                        LEFT JOIN read_status rs ON rs.feed_id = f.id AND rs.user_id = $2
                        WHERE f.created_at >= $1
                        AND (rs.feed_id IS NULL OR rs.is_read = FALSE)
    `

	var count int
	if err := r.pool.QueryRow(ctx, query, since, utils.DUMMY_USER_ID).Scan(&count); err != nil {
		logger.SafeError("failed to fetch today's unread articles count", "error", err)
		return 0, errors.New("failed to fetch today's unread articles count")
	}

	logger.SafeInfo("today's unread articles count fetched successfully", "count", count)
	return count, nil
}
