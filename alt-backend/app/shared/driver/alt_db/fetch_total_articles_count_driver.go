package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"

	"github.com/google/uuid"
)

// FetchTotalArticlesCountForUser counts one user's live articles.
//
// The owner is an argument rather than something read from the request
// context, which is what changed when ADR-000954 Wave 3 batch 5 moved this
// count behind DataHubService (catalog §2.M W3-M2). alt-data-hub sees a peer
// certificate naming alt-backend and nothing about whose dashboard is being
// drawn, so the tenant has to travel as data.
//
// The zero UUID is refused rather than treated as "everyone". user_id is the
// only predicate separating tenants here, so an unset owner would return 0
// and look exactly like an empty account.
func (r *ArticleRepository) FetchTotalArticlesCountForUser(ctx context.Context, userID uuid.UUID) (int, error) {
	if r == nil || r.pool == nil {
		return 0, errors.New("database connection not available")
	}
	if userID == uuid.Nil {
		return 0, errors.New("authentication required")
	}

	query := `SELECT COUNT(*) FROM articles WHERE user_id = $1 AND deleted_at IS NULL`

	var count int
	if err := r.pool.QueryRow(ctx, query, userID).Scan(&count); err != nil {
		logger.SafeErrorContext(ctx, "failed to fetch total articles count", "error", err)
		return 0, errors.New("failed to fetch total articles count")
	}

	logger.SafeInfoContext(ctx, "total articles count fetched successfully", "count", count)
	return count, nil
}
