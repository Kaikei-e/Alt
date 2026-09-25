package alt_db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

func (r *FeedRepository) GetReadFeedIDs(ctx context.Context, userID uuid.UUID, feedIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	query := `
		SELECT feed_id FROM read_status
		WHERE user_id = $1 AND feed_id = ANY($2::uuid[]) AND is_read = TRUE
	`

	feedIDStrings := make([]string, 0, len(feedIDs))
	for _, feedID := range feedIDs {
		feedIDStrings = append(feedIDStrings, feedID.String())
	}

	rows, err := r.pool.Query(ctx, query, userID, feedIDStrings)
	if err != nil {
		return nil, fmt.Errorf("query read feed ids: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID]bool, len(feedIDs))
	for rows.Next() {
		var feedID uuid.UUID
		if err := rows.Scan(&feedID); err != nil {
			return nil, fmt.Errorf("scan read feed id: %w", err)
		}
		result[feedID] = true
	}
	return result, rows.Err()
}

// maxReadFeedIDs bounds the result set to prevent unbounded growth.
const maxReadFeedIDs = 10000

func (r *FeedRepository) GetAllReadFeedIDs(ctx context.Context, userID uuid.UUID, since *time.Time) (map[uuid.UUID]bool, error) {
	if since == nil {
		query := `
		SELECT feed_id FROM read_status
		WHERE user_id = $1 AND is_read = TRUE
		ORDER BY read_at DESC
		LIMIT $2
	`

		rows, err := r.pool.Query(ctx, query, userID, maxReadFeedIDs)
		if err != nil {
			return nil, fmt.Errorf("query all read feed ids: %w", err)
		}
		defer rows.Close()

		result := make(map[uuid.UUID]bool)
		for rows.Next() {
			var feedID uuid.UUID
			if err := rows.Scan(&feedID); err != nil {
				return nil, fmt.Errorf("scan read feed id: %w", err)
			}
			result[feedID] = true
		}
		return result, rows.Err()
	}

	// read_status.read_at is TIMESTAMP without time zone stored as UTC; convert the timestamptz parameter to UTC for an unambiguous comparison.
	query := `
		SELECT feed_id FROM read_status
		WHERE user_id = $1 AND is_read = TRUE AND read_at >= ($2::timestamptz AT TIME ZONE 'UTC')
		ORDER BY read_at DESC
		LIMIT $3
	`

	rows, err := r.pool.Query(ctx, query, userID, *since, maxReadFeedIDs)
	if err != nil {
		return nil, fmt.Errorf("query all read feed ids: %w", err)
	}
	defer rows.Close()

	result := make(map[uuid.UUID]bool)
	for rows.Next() {
		var feedID uuid.UUID
		if err := rows.Scan(&feedID); err != nil {
			return nil, fmt.Errorf("scan read feed id: %w", err)
		}
		result[feedID] = true
	}
	return result, rows.Err()
}
