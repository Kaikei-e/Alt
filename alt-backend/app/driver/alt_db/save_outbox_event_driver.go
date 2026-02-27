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
func (r *AltDBRepository) SaveOutboxEventWithTx(ctx context.Context, tx pgx.Tx, eventType string, payload []byte) error {
	if _, err := tx.Exec(ctx, insertOutboxQuery, eventType, payload); err != nil {
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

// FetchAndLockPendingOutboxEvents retrieves pending events within a transaction,
// locks them with FOR UPDATE SKIP LOCKED, and atomically sets status to PROCESSING.
// This prevents multiple workers from processing the same event.
func (r *AltDBRepository) FetchAndLockPendingOutboxEvents(ctx context.Context, limit int) ([]OutboxEvent, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT id, event_type, payload, status, created_at
		FROM outbox_events
		WHERE status = 'PENDING'
		ORDER BY created_at ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, limit)
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
		if _, err := tx.Exec(ctx, `UPDATE outbox_events SET status = 'PROCESSING' WHERE id = $1`, e.ID); err != nil {
			return nil, fmt.Errorf("failed to mark event %s as PROCESSING: %w", e.ID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return events, nil
}

// UpdateOutboxEventStatus updates the status of an outbox event.
func (r *AltDBRepository) UpdateOutboxEventStatus(ctx context.Context, id string, status string, errorMessage *string) error {
	var processedAt interface{}
	if status == "PROCESSED" || status == "FAILED" {
		processedAt = time.Now()
	}

	query := `
		UPDATE outbox_events
		SET status = $1, processed_at = $2, error_message = $3
		WHERE id = $4
	`

	if _, err := r.pool.Exec(ctx, query, status, processedAt, errorMessage, id); err != nil {
		return fmt.Errorf("failed to update outbox event status: %w", err)
	}

	return nil
}

// PruneOutboxEvents deletes processed events older than the specified duration.
func (r *AltDBRepository) PruneOutboxEvents(ctx context.Context, olderThan time.Duration) (int64, error) {
	query := `DELETE FROM outbox_events WHERE status = 'PROCESSED' AND processed_at < $1`
	cutoff := time.Now().Add(-olderThan)

	tag, err := r.pool.Exec(ctx, query, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to prune outbox events: %w", err)
	}
	return tag.RowsAffected(), nil
}
