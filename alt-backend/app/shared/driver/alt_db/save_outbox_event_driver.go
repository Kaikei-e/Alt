package alt_db

import (
	"alt/utils/logger"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const insertOutboxQuery = `
	INSERT INTO outbox_events (event_type, payload)
	VALUES ($1, $2)
`

// SaveOutboxEventWithTx inserts an event into the outbox table using a provided transaction.
func (r *OutboxRepository) SaveOutboxEventWithTx(ctx context.Context, tx pgx.Tx, eventType string, payload []byte) error {
	if _, err := tx.Exec(ctx, insertOutboxQuery, eventType, string(payload)); err != nil {
		err = fmt.Errorf("failed to insert outbox event: %w", err)
		// We can't log article_id easily here without parsing payload, so general error log
		logger.SafeErrorContext(ctx, "failed to save outbox event", "event_type", eventType, "error", err)
		return err
	}
	return nil
}

// OutboxEvent represents a row in the outbox_events table.
type OutboxEvent struct {
	ID        string    `json:"id"`
	EventType string    `json:"event_type"`
	Payload   []byte    `json:"payload"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// outboxClaimLease is how long a claim owns a row before another worker may
// take it back. It has to exceed the outbox-worker job's own timeout plus the
// detached status write that follows it, or a batch that is merely slow — ten
// heavy articles through the local embedder — would be stolen from a live
// worker and delivered twice.
const outboxClaimLease = 15 * time.Minute

// claimOutboxBatchQuery selects the rows this worker is about to own.
//
// Fresh work and crash reclaim are the same predicate, as in push_deliveries:
// a PROCESSING row whose lease has elapsed is due again, so a harvester killed
// mid-batch recovers on the next tick instead of stranding its rows forever.
// There is no separate reclaim sweeper, and therefore none to forget to
// schedule. Terminal rows (PROCESSED, FAILED) never match.
//
// The order stays created_at rather than the lease horizon: an outbox delivers
// oldest event first, and a reclaimed row is older than everything queued
// behind it.
const claimOutboxBatchQuery = `
	SELECT id, event_type, payload, status, created_at
	FROM outbox_events
	WHERE status IN ('PENDING', 'PROCESSING')
	  AND next_attempt_at <= clock_timestamp()
	ORDER BY created_at ASC
	LIMIT $1
	FOR UPDATE SKIP LOCKED
`

// takeOutboxLeaseQuery marks a selected row claimed and pushes its lease
// horizon out by the lease window.
const takeOutboxLeaseQuery = `
	UPDATE outbox_events
	SET status = 'PROCESSING',
	    next_attempt_at = clock_timestamp() + make_interval(secs => $2::double precision)
	WHERE id = $1
`

// FetchAndLockPendingOutboxEvents retrieves due events within a transaction,
// locks them with FOR UPDATE SKIP LOCKED, and atomically sets status to PROCESSING.
// This prevents multiple workers from processing the same event.
func (r *OutboxRepository) FetchAndLockPendingOutboxEvents(ctx context.Context, limit int) ([]OutboxEvent, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, claimOutboxBatchQuery, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pending outbox events: %w", err)
	}
	defer rows.Close()

	var events []OutboxEvent
	for rows.Next() {
		var e OutboxEvent
		var id uuid.UUID
		if err := rows.Scan(&id, &e.EventType, &e.Payload, &e.Status, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan outbox event: %w", err)
		}
		e.ID = id.String()
		events = append(events, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate outbox events: %w", err)
	}

	// Atomically mark all selected events as PROCESSING within the same transaction
	for _, e := range events {
		if _, err := tx.Exec(ctx, takeOutboxLeaseQuery, e.ID, outboxClaimLease.Seconds()); err != nil {
			return nil, fmt.Errorf("failed to mark event %s as PROCESSING: %w", e.ID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return events, nil
}

// UpdateOutboxEventStatus updates the status of an outbox event.
func (r *OutboxRepository) UpdateOutboxEventStatus(ctx context.Context, id string, status string, errorMessage *string) error {
	var processedAt interface{}
	if status == "PROCESSED" || status == "FAILED" {
		processedAt = time.Now()
	}

	// A release back to PENDING drops the lease with it: the worker releases a
	// row so the next tick retries it, and leaving the claim's horizon in place
	// would make every retry wait out the whole lease window instead.
	query := `
		UPDATE outbox_events
		SET status = $1,
		    processed_at = $2,
		    error_message = $3,
		    next_attempt_at = CASE WHEN $1 = 'PENDING' THEN clock_timestamp() ELSE next_attempt_at END
		WHERE id = $4
	`

	if _, err := r.pool.Exec(ctx, query, status, processedAt, errorMessage, id); err != nil {
		return fmt.Errorf("failed to update outbox event status: %w", err)
	}

	return nil
}

// PruneOutboxEvents deletes processed events older than the specified duration.
func (r *OutboxRepository) PruneOutboxEvents(ctx context.Context, olderThan time.Duration) (int64, error) {
	query := `DELETE FROM outbox_events WHERE status = 'PROCESSED' AND processed_at < $1`
	cutoff := time.Now().Add(-olderThan)

	tag, err := r.pool.Exec(ctx, query, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to prune outbox events: %w", err)
	}
	return tag.RowsAffected(), nil
}
