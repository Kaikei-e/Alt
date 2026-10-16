package alt_db

import (
	"alt/domain"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// FetchSubscriptions returns all feed_links with subscription status for a given user.
func (r *SubscriptionRepository) FetchSubscriptions(ctx context.Context, userID uuid.UUID) ([]*domain.FeedSource, error) {
	query := `
		SELECT fl.id, fl.url,
		       COALESCE(ufs.feed_link_id IS NOT NULL, FALSE) AS is_subscribed,
		       COALESCE(ufs.created_at, NOW()) AS created_at
		FROM feed_links fl
		LEFT JOIN user_feed_subscriptions ufs
		    ON ufs.feed_link_id = fl.id AND ufs.user_id = $1
		ORDER BY fl.url ASC
	`

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("fetch subscriptions: %w", err)
	}
	defer rows.Close()

	var sources []*domain.FeedSource
	for rows.Next() {
		var s domain.FeedSource
		var createdAt time.Time
		if err := rows.Scan(&s.ID, &s.URL, &s.IsSubscribed, &createdAt); err != nil {
			return nil, fmt.Errorf("scan subscription: %w", err)
		}
		s.CreatedAt = createdAt
		sources = append(sources, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subscriptions: %w", err)
	}

	return sources, nil
}

// InsertSubscription inserts a user feed subscription.
func (r *SubscriptionRepository) InsertSubscription(ctx context.Context, userID uuid.UUID, feedLinkID uuid.UUID) error {
	query := `
		INSERT INTO user_feed_subscriptions (user_id, feed_link_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`
	_, err := r.pool.Exec(ctx, query, userID, feedLinkID)
	if err != nil {
		return fmt.Errorf("insert subscription: %w", err)
	}
	return nil
}

const deleteSubscriptionQuery = `
		DELETE FROM user_feed_subscriptions
		WHERE user_id = $1 AND feed_link_id = $2
	`

const deleteOrphanFeedLinkQuery = `
		DELETE FROM feed_links
		WHERE id = $1
		  AND NOT EXISTS (
			SELECT 1 FROM user_feed_subscriptions WHERE feed_link_id = $1
		  )
	`

// DeleteSubscription deletes a user feed subscription and garbage-collects the
// master feed_links row only when no other user remains subscribed.
func (r *SubscriptionRepository) DeleteSubscription(ctx context.Context, userID uuid.UUID, feedLinkID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, deleteSubscriptionQuery, userID, feedLinkID)
	if err != nil {
		return fmt.Errorf("delete subscription: %w", err)
	}
	if _, err := r.pool.Exec(ctx, deleteOrphanFeedLinkQuery, feedLinkID); err != nil {
		return fmt.Errorf("delete orphan feed link: %w", err)
	}
	return nil
}
