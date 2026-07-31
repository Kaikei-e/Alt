package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"

	"github.com/google/uuid"
)

// FetchUserFeedIDsForUser retrieves the feed IDs a user has read state against
// (catalog §2.M W3-M7) — the Morning Letter's candidate set.
//
// Explicit owner, for the reason given on FetchTotalArticlesCountForUser.
func (r *DashboardRepository) FetchUserFeedIDsForUser(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("database connection not available")
	}
	if userID == uuid.Nil {
		return nil, errors.New("authentication required")
	}

	logger.Logger.InfoContext(ctx, "Fetching user feed IDs", "user_id", userID)

	// Feeds the user has interacted with. DISTINCT is unnecessary because
	// (feed_id, user_id) has a UNIQUE constraint.
	query := `
		SELECT feed_id
		FROM read_status
		WHERE user_id = $1
		ORDER BY feed_id
	`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to query user feed IDs", "error", err, "user_id", userID)
		return nil, errors.New("error fetching user feed IDs")
	}
	defer rows.Close()

	var feedIDs []uuid.UUID
	for rows.Next() {
		var feedID uuid.UUID
		if err := rows.Scan(&feedID); err != nil {
			logger.Logger.ErrorContext(ctx, "Failed to scan feed ID", "error", err)
			return nil, errors.New("error scanning feed ID")
		}
		feedIDs = append(feedIDs, feedID)
	}

	if err := rows.Err(); err != nil {
		logger.Logger.ErrorContext(ctx, "Error iterating feed ID rows", "error", err)
		return nil, errors.New("error iterating feed ID rows")
	}

	logger.Logger.InfoContext(ctx, "Successfully fetched user feed IDs", "count", len(feedIDs), "user_id", userID)
	return feedIDs, nil
}
