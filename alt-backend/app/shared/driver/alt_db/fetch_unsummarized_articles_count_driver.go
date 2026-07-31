package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"

	"github.com/google/uuid"
)

// FetchUnsummarizedArticlesCountForUser counts one user's live articles with
// no summary (catalog §2.M W3-M4).
//
// Its own query rather than a subtraction: a summary can outlive the article
// it describes, so total minus summarized is a different number.
func (r *DashboardRepository) FetchUnsummarizedArticlesCountForUser(ctx context.Context, userID uuid.UUID) (int, error) {
	if r == nil || r.pool == nil {
		return 0, errors.New("database connection not available")
	}
	if userID == uuid.Nil {
		return 0, errors.New("authentication required")
	}

	query := `
		SELECT COUNT(*)
		FROM articles a
		LEFT JOIN article_summaries s ON a.id = s.article_id AND s.user_id = a.user_id
		WHERE a.user_id = $1 AND a.deleted_at IS NULL AND s.article_id IS NULL
	`

	var count int
	if err := r.pool.QueryRow(ctx, query, userID).Scan(&count); err != nil {
		logger.SafeErrorContext(ctx, "failed to fetch unsummarized articles count", "error", err)
		return 0, errors.New("failed to fetch unsummarized articles count")
	}

	logger.SafeInfoContext(ctx, "unsummarized articles count fetched successfully", "count", count)
	return count, nil
}
