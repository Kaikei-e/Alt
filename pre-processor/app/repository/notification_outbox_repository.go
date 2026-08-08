package repository

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"pre-processor/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NotificationOutboxRepository is the relay's view of notification_outbox:
// take ownership of due rows, then record what happened to each one.
//
// Every instant these statements write or compare against comes from
// clock_timestamp() inside Postgres, matching the column defaults the table
// was created with. Durations cross the boundary instead of absolute times, so
// a relay whose host clock drifts from the database's cannot claim rows early,
// hold a lease it thinks is longer than it is, or schedule a retry into the
// past.
type NotificationOutboxRepository interface {
	// ClaimBatch takes ownership of up to limit due rows and returns them with
	// the claim already committed, so the caller can never hold a transaction
	// open across the forward RPC.
	ClaimBatch(ctx context.Context, lockedBy string, limit int, lease time.Duration) ([]domain.NotificationOutboxRow, error)
	MarkForwarded(ctx context.Context, id string) error
	MarkAttemptFailed(ctx context.Context, id string, retryIn time.Duration, reason string) error
	MarkDead(ctx context.Context, id string, reason string) error
	// OldestPendingAge reports how long the oldest un-forwarded row has been
	// waiting, or 0 when the backlog is empty.
	OldestPendingAge(ctx context.Context) (time.Duration, error)
}

// claimNotificationBatchQuery takes the lease and hands back the work in one
// statement.
//
// It matches non-terminal rows whose next_attempt_at is due, which covers
// fresh work and crash reclaim together: the claim pushes next_attempt_at
// forward by the lease, so a row orphaned by a relay that died mid-forward
// re-enters this same query once the lease expires. There is no separate
// reclaim sweeper to deploy or forget. attempts advances here rather than at
// completion for the same reason — otherwise a relay that dies on every
// attempt would loop on the row forever without ever reaching 'dead'.
const claimNotificationBatchQuery = `
		UPDATE notification_outbox
		SET state = 'forwarding',
		    attempts = attempts + 1,
		    locked_by = $1,
		    next_attempt_at = clock_timestamp() + make_interval(secs => $2)
		WHERE id IN (
			SELECT id FROM notification_outbox
			WHERE state IN ('pending', 'forwarding')
			  AND next_attempt_at <= clock_timestamp()
			ORDER BY next_attempt_at, id
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, dedupe_key, user_id, kind, payload, occurred_at, attempts
	`

const markNotificationForwardedQuery = `
		UPDATE notification_outbox
		SET state = 'forwarded', forwarded_at = clock_timestamp(), locked_by = NULL, last_error = NULL
		WHERE id = $1
	`

const markNotificationAttemptFailedQuery = `
		UPDATE notification_outbox
		SET state = 'pending',
		    next_attempt_at = clock_timestamp() + make_interval(secs => $1),
		    last_error = $2,
		    locked_by = NULL
		WHERE id = $3
	`

const markNotificationDeadQuery = `
		UPDATE notification_outbox
		SET state = 'dead', last_error = $1, locked_by = NULL
		WHERE id = $2
	`

// oldestPendingNotificationAgeQuery reports the backlog age as a number in
// every case: COALESCE turns the NULL an empty backlog produces into 0, so the
// caller always has a value to publish rather than a reason to skip the gauge.
const oldestPendingNotificationAgeQuery = `
		SELECT COALESCE(EXTRACT(EPOCH FROM (clock_timestamp() - MIN(created_at))), 0)
		FROM notification_outbox
		WHERE state IN ('pending', 'forwarding')
	`

type notificationOutboxRepository struct {
	db     dbExecutor
	logger *slog.Logger
}

// NewNotificationOutboxRepository creates the relay-side repository over
// pre-processor-db.
//
// db is taken as the concrete *pgxpool.Pool and only assigned when non-nil for
// the same reason NewSummarizeJobRepository does it: a nil *pgxpool.Pool
// assigned into an interface field yields a non-nil interface wrapping a nil
// pointer, and every nil guard below would silently stop working.
func NewNotificationOutboxRepository(db *pgxpool.Pool, logger *slog.Logger) NotificationOutboxRepository {
	repo := &notificationOutboxRepository{logger: logger}
	if db != nil {
		repo.db = db
	}
	return repo
}

func (r *notificationOutboxRepository) ClaimBatch(ctx context.Context, lockedBy string, limit int, lease time.Duration) ([]domain.NotificationOutboxRow, error) {
	if lockedBy == "" {
		return nil, fmt.Errorf("lockedBy cannot be empty: it is the only link from a stuck row to the process holding it")
	}
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive")
	}
	if r.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin claim transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	rows, err := tx.Query(ctx, claimNotificationBatchQuery, lockedBy, lease.Seconds(), limit)
	if err != nil {
		return nil, fmt.Errorf("claim notification batch: %w", err)
	}

	claimed := make([]domain.NotificationOutboxRow, 0, limit)
	for rows.Next() {
		var row domain.NotificationOutboxRow
		if err := rows.Scan(&row.ID, &row.DedupeKey, &row.UserID, &row.Kind, &row.Payload, &row.OccurredAt, &row.Attempts); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan claimed notification: %w", err)
		}
		claimed = append(claimed, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate claimed notifications: %w", err)
	}

	// Committed here, before the caller sees a single row: the forward RPC
	// must never run inside this transaction.
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit claim transaction: %w", err)
	}

	return claimed, nil
}

func (r *notificationOutboxRepository) MarkForwarded(ctx context.Context, id string) error {
	if r.db == nil {
		return fmt.Errorf("database connection is nil")
	}
	if _, err := r.db.Exec(ctx, markNotificationForwardedQuery, id); err != nil {
		return fmt.Errorf("mark notification forwarded: %w", err)
	}
	return nil
}

func (r *notificationOutboxRepository) MarkAttemptFailed(ctx context.Context, id string, retryIn time.Duration, reason string) error {
	if r.db == nil {
		return fmt.Errorf("database connection is nil")
	}
	if retryIn < 0 {
		retryIn = 0
	}
	if _, err := r.db.Exec(ctx, markNotificationAttemptFailedQuery, retryIn.Seconds(), reason, id); err != nil {
		return fmt.Errorf("mark notification attempt failed: %w", err)
	}
	return nil
}

func (r *notificationOutboxRepository) MarkDead(ctx context.Context, id string, reason string) error {
	if r.db == nil {
		return fmt.Errorf("database connection is nil")
	}
	if _, err := r.db.Exec(ctx, markNotificationDeadQuery, reason, id); err != nil {
		return fmt.Errorf("mark notification dead: %w", err)
	}
	return nil
}

func (r *notificationOutboxRepository) OldestPendingAge(ctx context.Context) (time.Duration, error) {
	if r.db == nil {
		return 0, fmt.Errorf("database connection is nil")
	}

	var seconds float64
	if err := r.db.QueryRow(ctx, oldestPendingNotificationAgeQuery).Scan(&seconds); err != nil {
		return 0, fmt.Errorf("read oldest pending notification age: %w", err)
	}
	if seconds < 0 {
		seconds = 0
	}
	return time.Duration(seconds * float64(time.Second)), nil
}
