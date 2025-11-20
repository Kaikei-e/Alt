package alt_db

import (
	"alt/utils/logger"
	"context"
	"errors"

	"github.com/google/uuid"
)

// FetchUserFeedIDs retrieves the feed IDs that a user is subscribed to.
func (r *AltDBRepository) FetchUserFeedIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	if r == nil || r.pool == nil {
		return nil, errors.New("database connection not available")
	}

	logger.Logger.Info("Fetching user feed IDs", "user_id", userID)

	query := `
		SELECT DISTINCT uf.feed_id
		FROM user_feeds uf
		WHERE uf.user_id = $1
		ORDER BY uf.feed_id
	`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		logger.Logger.Error("Failed to query user feed IDs", "error", err, "user_id", userID)
		return nil, errors.New("error fetching user feed IDs")
	}
	defer rows.Close()

	var feedIDs []uuid.UUID
	for rows.Next() {
		var feedID uuid.UUID
		if err := rows.Scan(&feedID); err != nil {
			logger.Logger.Error("Failed to scan feed ID", "error", err)
			return nil, errors.New("error scanning feed ID")
		}
		feedIDs = append(feedIDs, feedID)
	}

	if err := rows.Err(); err != nil {
		logger.Logger.Error("Error iterating feed ID rows", "error", err)
		return nil, errors.New("error iterating feed ID rows")
	}

	logger.Logger.Info("Successfully fetched user feed IDs", "count", len(feedIDs), "user_id", userID)
	return feedIDs, nil
}
