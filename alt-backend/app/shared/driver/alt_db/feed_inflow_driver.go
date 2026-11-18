package alt_db

import (
	"alt/domain"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// GetFeedInflowByUser returns stored item inflow counts over [from, to] for each
// feed link the user currently subscribes to.
func (r *FeedRepository) GetFeedInflowByUser(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]domain.FeedInflow, error) {
	const query = `
		SELECT ufs.feed_link_id,
		       fl.url,
		       ufs.daily_cap,
		       COUNT(f.id)::bigint AS inflow_count
		FROM user_feed_subscriptions ufs
		INNER JOIN feed_links fl ON fl.id = ufs.feed_link_id
		LEFT JOIN feeds f
		  ON f.feed_link_id = ufs.feed_link_id
		 AND f.created_at >= $2
		 AND f.created_at <= $3
		WHERE ufs.user_id = $1
		GROUP BY ufs.feed_link_id, fl.url, ufs.daily_cap
		ORDER BY fl.url ASC, ufs.feed_link_id ASC
	`

	rows, err := r.pool.Query(ctx, query, userID, from.UTC(), to.UTC())
	if err != nil {
		return nil, fmt.Errorf("get feed inflow by user: %w", err)
	}
	defer rows.Close()

	var result []domain.FeedInflow
	for rows.Next() {
		var item domain.FeedInflow
		if err := rows.Scan(&item.FeedLinkID, &item.FeedURL, &item.DailyCap, &item.InflowCount); err != nil {
			return nil, fmt.Errorf("scan feed inflow: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate feed inflow: %w", err)
	}
	return result, nil
}

// BatchGetFeedLinkIDsByArticleIDs maps article IDs belonging to userID to their feed_link_id.
func (r *FeedRepository) BatchGetFeedLinkIDsByArticleIDs(ctx context.Context, userID uuid.UUID, articleIDs []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	out := make(map[uuid.UUID]uuid.UUID)
	if len(articleIDs) == 0 {
		return out, nil
	}

	// Convert []uuid.UUID to []string for PgBouncer simple protocol compatibility.
	stringIDs := make([]string, len(articleIDs))
	for i, id := range articleIDs {
		stringIDs[i] = id.String()
	}

	const query = `
		SELECT a.id, f.feed_link_id
		FROM articles a
		INNER JOIN feeds f ON a.feed_id = f.id
		WHERE a.user_id = $1
		  AND a.id = ANY($2::uuid[])
		  AND a.deleted_at IS NULL
		  AND f.feed_link_id IS NOT NULL
	`

	rows, err := r.pool.Query(ctx, query, userID, stringIDs)
	if err != nil {
		return nil, fmt.Errorf("batch get feed link ids by article ids: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var articleID, feedLinkID uuid.UUID
		if err := rows.Scan(&articleID, &feedLinkID); err != nil {
			return nil, fmt.Errorf("scan article feed link id: %w", err)
		}
		out[articleID] = feedLinkID
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate article feed link ids: %w", err)
	}
	return out, nil
}
