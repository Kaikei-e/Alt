package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// incrementFeedLinkFailuresQuery bumps the consecutive failure run, creating
// the availability row for links registered after the availability migration.
//
// The INSERT ... SELECT matches on feed_links.url, so an unsubscribed URL
// selects nothing, inserts nothing, and RETURNING yields no row — which is how
// callers learn the URL is unknown rather than being told it is healthy.
const incrementFeedLinkFailuresQuery = `
		INSERT INTO feed_link_availability (feed_link_id, is_active, consecutive_failures, last_failure_at, last_failure_reason)
		SELECT fl.id, true, 1, NOW(), $2
		FROM feed_links fl WHERE fl.url = $1
		ON CONFLICT (feed_link_id) DO UPDATE SET
			consecutive_failures = feed_link_availability.consecutive_failures + 1,
			last_failure_at = NOW(),
			last_failure_reason = $2
		RETURNING feed_link_id, is_active, consecutive_failures, last_failure_at, last_failure_reason`

const resetFeedLinkFailuresQuery = `
		INSERT INTO feed_link_availability (feed_link_id, is_active, consecutive_failures)
		SELECT id, true, 0 FROM feed_links WHERE url = $1
		ON CONFLICT (feed_link_id) DO UPDATE SET
			is_active = true,
			consecutive_failures = 0,
			last_failure_at = NULL,
			last_failure_reason = NULL`

// disableFeedLinkQuery matches with IN rather than = because feed_links has no
// unique constraint on url and duplicate rows exist in production data.
const disableFeedLinkQuery = `
		UPDATE feed_link_availability SET is_active = false
		WHERE feed_link_id IN (SELECT id FROM feed_links WHERE url = $1)`

// IncrementFeedLinkFailures increments failure count and returns the updated domain object.
// It uses UPSERT to handle feeds that don't have an availability record yet.
//
// Prefer RecordFeedLinkFailure: on its own this leaves the caller holding a
// read-modify-write, which is what capability catalog §4-4 is about.
func (r *FeedRepository) IncrementFeedLinkFailures(ctx context.Context, feedURL, reason string) (*domain.FeedLinkAvailability, error) {
	availability, err := scanFeedLinkAvailability(
		r.pool.QueryRow(ctx, incrementFeedLinkFailuresQuery, feedURL, reason))
	if err != nil {
		logger.SafeErrorContext(ctx, "Failed to increment feed failures", "url", feedURL, "error", err)
		return nil, fmt.Errorf("increment feed link failures: %w", err)
	}
	return availability, nil
}

// RecordFeedLinkFailure increments the consecutive failure run and, in the same
// transaction, disables the link once the run reaches disableAfter.
//
// This is the capability the collector actually needs, and it is one procedure
// rather than two because the decision reads what the increment just wrote.
// With the two split, the caller had to fetch the count, compare it, and issue
// a second statement; two collectors polling the same broken feed could each
// see a count one below the threshold and neither disable it. Moving the
// caller into another process (ADR-000954) would have stretched that window
// across a network round trip, so the read-modify-write moves to the side of
// the boundary that can hold a transaction around it.
//
// disableAfter is an argument, not a constant here, because how many failures
// to tolerate is the operator's policy; a non-positive value means never
// disable. The second return value reports the transition to inactive, not the
// state: a caller that logged the state would repeat the alert on every poll
// of an already-dead feed.
func (r *FeedRepository) RecordFeedLinkFailure(ctx context.Context, feedURL, reason string, disableAfter int) (*domain.FeedLinkAvailability, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("begin record feed link failure tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			logger.SafeWarnContext(ctx, "rollback failed", "error", rbErr)
		}
	}()

	availability, err := scanFeedLinkAvailability(
		tx.QueryRow(ctx, incrementFeedLinkFailuresQuery, feedURL, reason))
	if err != nil {
		logger.SafeErrorContext(ctx, "Failed to increment feed failures", "url", feedURL, "error", err)
		return nil, false, fmt.Errorf("increment feed link failures: %w", err)
	}

	// The threshold comparison itself stays in domain.FeedLinkAvailability so
	// it reads the same here as at any caller still on the split path. The
	// IsActive guard is this method's own: ShouldDisable answers "has it failed
	// enough", which stays true for every later poll of an already-dead feed,
	// and re-issuing the UPDATE for those would report a transition that
	// already happened.
	disabledNow := disableAfter > 0 && availability.IsActive && availability.ShouldDisable(disableAfter)
	if disabledNow {
		if _, execErr := tx.Exec(ctx, disableFeedLinkQuery, feedURL); execErr != nil {
			logger.SafeErrorContext(ctx, "Failed to disable feed link", "url", feedURL, "error", execErr)
			return nil, false, fmt.Errorf("disable feed link: %w", execErr)
		}
		// The RETURNING clause above ran before the UPDATE, so it still
		// reports is_active = true. Handing back that snapshot would tell the
		// caller the feed is live at the exact moment it stopped being polled.
		availability.IsActive = false
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("commit record feed link failure tx: %w", err)
	}
	return availability, disabledNow, nil
}

// rowScanner is the one method both the pool's and the transaction's QueryRow
// results provide, so the increment's scan is written once for both paths.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanFeedLinkAvailability(row rowScanner) (*domain.FeedLinkAvailability, error) {
	var feedLinkID uuid.UUID
	var isActive bool
	var consecutiveFailures int
	var lastFailureAt *time.Time
	var lastFailureReason *string

	if err := row.Scan(&feedLinkID, &isActive, &consecutiveFailures, &lastFailureAt, &lastFailureReason); err != nil {
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
// It uses UPSERT to create the availability row if it doesn't exist yet
// (e.g., feeds registered after the initial migration).
func (r *FeedRepository) ResetFeedLinkFailures(ctx context.Context, feedURL string) error {
	_, err := r.pool.Exec(ctx, resetFeedLinkFailuresQuery, feedURL)
	if err != nil {
		return fmt.Errorf("reset feed link failures: %w", err)
	}
	return nil
}

// DisableFeedLink marks a feed as inactive.
func (r *FeedRepository) DisableFeedLink(ctx context.Context, feedURL string) error {
	_, err := r.pool.Exec(ctx, disableFeedLinkQuery, feedURL)
	if err != nil {
		logger.SafeErrorContext(ctx, "Failed to disable feed link", "url", feedURL, "error", err)
		return fmt.Errorf("disable feed link: %w", err)
	}
	return nil
}
