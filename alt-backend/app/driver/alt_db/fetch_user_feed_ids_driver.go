package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"

	"github.com/google/uuid"
)

// FetchUserFeedIDs retrieves the feed IDs that a user is subscribed to.
// User ID is extracted from the context, following the same pattern as cursor-based endpoints.
func (r *AltDBRepository) FetchUserFeedIDs(ctx context.Context) ([]uuid.UUID, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("database connection not available")
	}

	// Get user from context (same pattern as cursor-based endpoints)
	user, err := domain.GetUserFromContext(ctx)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "user context not found", "error", err)
		return nil, errors.New("authentication required")
	}

	logger.Logger.InfoContext(ctx, "Fetching user feed IDs", "user_id", user.UserID)

	// Get feed IDs from read_status table where user has interacted with articles
	// This represents feeds that the user has subscribed to or has read articles from
	query := `
		SELECT DISTINCT feed_id
		FROM read_status
		WHERE user_id = $1
		ORDER BY feed_id
	`

	rows, err := r.pool.Query(ctx, query, user.UserID)
	if err != nil {
		logger.Logger.ErrorContext(ctx, "Failed to query user feed IDs", "error", err, "user_id", user.UserID)
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

	logger.Logger.InfoContext(ctx, "Successfully fetched user feed IDs", "count", len(feedIDs), "user_id", user.UserID)
	return feedIDs, nil
}
