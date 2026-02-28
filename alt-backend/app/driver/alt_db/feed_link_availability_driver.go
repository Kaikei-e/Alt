package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"time"

	"github.com/google/uuid"
)

// IncrementFeedLinkFailures increments failure count and returns the updated domain object.
// It uses UPSERT to handle feeds that don't have an availability record yet.
func (r *AltDBRepository) IncrementFeedLinkFailures(ctx context.Context, feedURL, reason string) (*domain.FeedLinkAvailability, error) {
	query := `
		INSERT INTO feed_link_availability (feed_link_id, is_active, consecutive_failures, last_failure_at, last_failure_reason)
		SELECT fl.id, true, 1, NOW(), $2
		FROM feed_links fl WHERE fl.url = $1
		ON CONFLICT (feed_link_id) DO UPDATE SET
			consecutive_failures = feed_link_availability.consecutive_failures + 1,
			last_failure_at = NOW(),
			last_failure_reason = $2
		RETURNING feed_link_id, is_active, consecutive_failures, last_failure_at, last_failure_reason`

	var feedLinkID uuid.UUID
	var isActive bool
	var consecutiveFailures int
	var lastFailureAt *time.Time
	var lastFailureReason *string

	err := r.pool.QueryRow(ctx, query, feedURL, reason).Scan(
		&feedLinkID, &isActive, &consecutiveFailures, &lastFailureAt, &lastFailureReason,
	)
	if err != nil {
		logger.SafeErrorContext(ctx, "Failed to increment feed failures", "url", feedURL, "error", err)
		return nil, err
	}

	return &domain.FeedLinkAvailability{
		FeedLinkID:          feedLinkID,
		IsActive:            isActive,
		ConsecutiveFailures: consecutiveFailures,
		LastFailureAt:       lastFailureAt,
		LastFailureReason:   lastFailureReason,
	}, nil
}

// ResetFeedLinkFailures resets the failure count on successful fetch.
func (r *AltDBRepository) ResetFeedLinkFailures(ctx context.Context, feedURL string) error {
	query := `
		UPDATE feed_link_availability SET consecutive_failures = 0
		WHERE feed_link_id IN (SELECT id FROM feed_links WHERE url = $1)`
	_, err := r.pool.Exec(ctx, query, feedURL)
	return err
}

// DisableFeedLink marks a feed as inactive.
func (r *AltDBRepository) DisableFeedLink(ctx context.Context, feedURL string) error {
	query := `
		UPDATE feed_link_availability SET is_active = false
		WHERE feed_link_id IN (SELECT id FROM feed_links WHERE url = $1)`
	_, err := r.pool.Exec(ctx, query, feedURL)
	if err != nil {
		logger.SafeErrorContext(ctx, "Failed to disable feed link", "url", feedURL, "error", err)
	}
	return err
}
