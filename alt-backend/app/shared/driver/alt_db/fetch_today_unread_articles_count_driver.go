package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// FetchTodayUnreadArticlesCountForUser counts feeds created since `since` that
// the user has not marked read (catalog §2.M W3-M5).
//
// `since` is the caller's: "today" is a wall-clock question and this side has
// no timezone for the reader.
func (r *ArticleRepository) FetchTodayUnreadArticlesCountForUser(ctx context.Context, userID uuid.UUID, since time.Time) (int, error) {
	if r == nil || r.pool == nil {
		return 0, errors.New("database connection not available")
	}
	if userID == uuid.Nil {
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
	if err := r.pool.QueryRow(ctx, query, since, userID).Scan(&count); err != nil {
		logger.SafeErrorContext(ctx, "failed to fetch today's unread articles count",
			"error", err,
			"user_id", userID)
		return 0, errors.New("failed to fetch today's unread articles count")
	}

	logger.SafeInfoContext(ctx, "today's unread articles count fetched successfully",
		"count", count,
		"user_id", userID)
	return count, nil
}
