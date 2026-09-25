package alt_db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type FeedPageRow struct {
	FeedID      uuid.UUID
	Title       string
	Description string
	Link        string
	PubDate     time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ArticleID   *string
	OgImageURL  *string
}

func (r *FeedRepository) FetchFeedsByFeedLinkID(ctx context.Context, feedLinkID uuid.UUID) ([]*FeedPageRow, error) {
	query := `
		SELECT f.id, f.title, f.description, f.website_url, f.pub_date, f.created_at, f.updated_at,
		       (SELECT a.id FROM articles a WHERE a.feed_id = f.id AND a.deleted_at IS NULL
		        ORDER BY a.created_at DESC LIMIT 1) AS article_id,
		       f.og_image_url
		FROM feeds f
		WHERE f.feed_link_id = $1
		ORDER BY f.created_at DESC, f.id DESC
		LIMIT 200
	`

	rows, err := r.pool.Query(ctx, query, feedLinkID)
	if err != nil {
		return nil, fmt.Errorf("query feeds by feed_link_id: %w", err)
	}
	defer rows.Close()

	result := make([]*FeedPageRow, 0)
	for rows.Next() {
		var row FeedPageRow
		if err := rows.Scan(&row.FeedID, &row.Title, &row.Description, &row.Link, &row.PubDate, &row.CreatedAt, &row.UpdatedAt, &row.ArticleID, &row.OgImageURL); err != nil {
			return nil, fmt.Errorf("scan feed page row: %w", err)
		}
		result = append(result, &row)
	}
	return result, rows.Err()
}

func (r *FeedRepository) GetUserSubscriptions(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	query := `SELECT feed_link_id FROM user_feed_subscriptions WHERE user_id = $1`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("query user subscriptions: %w", err)
	}
	defer rows.Close()

	result := make([]uuid.UUID, 0)
	for rows.Next() {
		var feedLinkID uuid.UUID
		if err := rows.Scan(&feedLinkID); err != nil {
			return nil, fmt.Errorf("scan user subscription: %w", err)
		}
		result = append(result, feedLinkID)
	}
	return result, rows.Err()
}
