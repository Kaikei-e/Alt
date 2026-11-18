package alt_db

import (
	"alt/domain"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// FetchSubscriptions returns all feed_links with subscription status for a given user.
func (r *SubscriptionRepository) FetchSubscriptions(ctx context.Context, userID uuid.UUID) ([]*domain.FeedSource, error) {
	query := `
		SELECT fl.id, fl.url,
		       COALESCE(ufs.feed_link_id IS NOT NULL, FALSE) AS is_subscribed,
		       COALESCE(ufs.created_at, NOW()) AS created_at,
		       ufs.daily_cap
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
		if err := rows.Scan(&s.ID, &s.URL, &s.IsSubscribed, &createdAt, &s.DailyCap); err != nil {
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
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return domain.ErrFeedNotFound
		}
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

// DeleteSubscription deletes a user feed subscription.
func (r *SubscriptionRepository) DeleteSubscription(ctx context.Context, userID uuid.UUID, feedLinkID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, deleteSubscriptionQuery, userID, feedLinkID)
	if err != nil {
		return fmt.Errorf("delete subscription: %w", err)
	}
	return nil
}

// SetSubscriptionDailyCap updates or clears the daily unread cap on an existing
// user subscription, returning domain.ErrFeedNotFound if the user is not subscribed.
func (r *SubscriptionRepository) SetSubscriptionDailyCap(ctx context.Context, userID uuid.UUID, feedLinkID uuid.UUID, dailyCap *int) error {
	if dailyCap != nil && *dailyCap <= 0 {
		return fmt.Errorf("invalid daily_cap %d: must be positive", *dailyCap)
	}
	const query = `
		UPDATE user_feed_subscriptions
		SET daily_cap = $3
		WHERE user_id = $1 AND feed_link_id = $2
	`
	tag, err := r.pool.Exec(ctx, query, userID, feedLinkID, dailyCap)
	if err != nil {
		return fmt.Errorf("set subscription daily cap: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrFeedNotFound
	}
	return nil
}
