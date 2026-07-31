package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"

	"github.com/google/uuid"
)

// FetchSummarizedArticlesCountForUser counts one user's article_summaries
// rows (catalog §2.M W3-M3). Explicit owner, for the reason given on
// FetchTotalArticlesCountForUser.
func (r *DashboardRepository) FetchSummarizedArticlesCountForUser(ctx context.Context, userID uuid.UUID) (int, error) {
	if r == nil || r.pool == nil {
		return 0, errors.New("database connection not available")
	}
	if userID == uuid.Nil {
		return 0, errors.New("authentication required")
	}

	query := `
		SELECT COUNT(*) FROM article_summaries WHERE user_id = $1
	`

	var count int
	if err := r.pool.QueryRow(ctx, query, userID).Scan(&count); err != nil {
		logger.SafeErrorContext(ctx, "failed to fetch summarized articles count", "error", err)
		return 0, errors.New("failed to fetch summarized articles count")
	}

	logger.SafeInfoContext(ctx, "summarized articles count fetched successfully", "count", count)
	return count, nil
}
