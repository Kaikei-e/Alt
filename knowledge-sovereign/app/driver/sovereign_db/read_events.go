package sovereign_db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// KnowledgeEvent represents a single event in the knowledge event store.
type KnowledgeEvent struct {
	EventID       uuid.UUID
	EventSeq      int64
	OccurredAt    time.Time
	TenantID      uuid.UUID
	UserID        *uuid.UUID
	ActorType     string
	ActorID       string
	EventType     string
	AggregateType string
	AggregateID   string
	CorrelationID *uuid.UUID
	CausationID   *uuid.UUID
	DedupeKey     string
	Payload       json.RawMessage
}

// ListKnowledgeEventsSince returns events after the given sequence number.
func (r *Repository) ListKnowledgeEventsSince(ctx context.Context, afterSeq int64, limit int) ([]KnowledgeEvent, error) {
	query := `SELECT event_id, event_seq, occurred_at, tenant_id, user_id,
		actor_type, actor_id, event_type, aggregate_type, aggregate_id,
		correlation_id, causation_id, dedupe_key, payload
		FROM knowledge_events WHERE event_seq > $1
		ORDER BY event_seq ASC LIMIT $2`

	return r.scanEvents(ctx, query, afterSeq, limit)
}

// ListKnowledgeEventsSinceForUser returns events for a specific user after the given sequence number.
func (r *Repository) ListKnowledgeEventsSinceForUser(ctx context.Context, userID uuid.UUID, afterSeq int64, limit int) ([]KnowledgeEvent, error) {
	query := `SELECT event_id, event_seq, occurred_at, tenant_id, user_id,
		actor_type, actor_id, event_type, aggregate_type, aggregate_id,
		correlation_id, causation_id, dedupe_key, payload
		FROM knowledge_events WHERE event_seq > $1 AND (user_id = $2 OR user_id IS NULL)
		ORDER BY event_seq ASC LIMIT $3`

	return r.scanEvents(ctx, query, afterSeq, userID, limit)
}

// GetLatestKnowledgeEventSeqForUser returns the latest event sequence number for a user.
func (r *Repository) GetLatestKnowledgeEventSeqForUser(ctx context.Context, userID uuid.UUID) (int64, error) {
	query := `SELECT COALESCE(MAX(event_seq), 0) FROM knowledge_events WHERE user_id = $1 OR user_id IS NULL`
	var seq int64
	if err := r.pool.QueryRow(ctx, query, userID).Scan(&seq); err != nil {
		return 0, fmt.Errorf("GetLatestKnowledgeEventSeqForUser: %w", err)
	}
	return seq, nil
}

// AppendKnowledgeEvent inserts a new knowledge event using a dedupe registry
// for idempotency. The dedupe registry is a non-partitioned table that holds
// the global UNIQUE constraint on dedupe_key, which cannot be placed on the
// partitioned knowledge_events table directly (PostgreSQL requires partition
// key in all UNIQUE constraints).
//
// Flow:
//  1. INSERT into knowledge_event_dedupes (ON CONFLICT DO NOTHING)
//  2. If dedupe INSERT affected 0 rows → duplicate, return 0 (idempotent)
//  3. Otherwise INSERT into knowledge_events and return event_seq
func (r *Repository) AppendKnowledgeEvent(ctx context.Context, event KnowledgeEvent) (int64, error) {
	// Step 1: Check dedupe registry
	dedupeQuery := `INSERT INTO knowledge_event_dedupes (dedupe_key, event_id, occurred_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (dedupe_key) DO NOTHING`

	tag, err := r.pool.Exec(ctx, dedupeQuery, event.DedupeKey, event.EventID, event.OccurredAt)
	if err != nil {
		return 0, fmt.Errorf("AppendKnowledgeEvent dedupe check: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return 0, nil // duplicate, idempotent
	}

	// Step 2: Insert event
	eventQuery := `INSERT INTO knowledge_events
		(event_id, occurred_at, tenant_id, user_id, actor_type, actor_id,
		 event_type, aggregate_type, aggregate_id, correlation_id, causation_id,
		 dedupe_key, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING event_seq`

	var eventSeq int64
	err = r.pool.QueryRow(ctx, eventQuery,
		event.EventID, event.OccurredAt, event.TenantID, event.UserID,
		event.ActorType, event.ActorID, event.EventType,
		event.AggregateType, event.AggregateID,
		event.CorrelationID, event.CausationID,
		event.DedupeKey, event.Payload,
	).Scan(&eventSeq)
	if err != nil {
		return 0, fmt.Errorf("AppendKnowledgeEvent insert: %w", err)
	}
	return eventSeq, nil
}

// KnowledgeUserEvent represents a user interaction event (seen, opened, etc.).
type KnowledgeUserEvent struct {
	UserEventID uuid.UUID
	OccurredAt  time.Time
	UserID      uuid.UUID
	TenantID    uuid.UUID
	EventType   string
	ItemKey     string
	Payload     json.RawMessage
	DedupeKey   string
}

// AppendKnowledgeUserEvent inserts a user event with deduplication.
// Uses the unique index on (dedupe_key, occurred_at) for partitioned table compatibility.
// PostgreSQL requires ON CONFLICT columns to match a unique constraint exactly.
func (r *Repository) AppendKnowledgeUserEvent(ctx context.Context, event KnowledgeUserEvent) error {
	query := `INSERT INTO knowledge_user_events
		(user_event_id, occurred_at, user_id, tenant_id, event_type, item_key, payload, dedupe_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (dedupe_key, occurred_at) WHERE dedupe_key != '' DO NOTHING`

	_, err := r.pool.Exec(ctx, query,
		event.UserEventID, event.OccurredAt, event.UserID, event.TenantID,
		event.EventType, event.ItemKey, event.Payload, event.DedupeKey,
	)
	if err != nil {
		return fmt.Errorf("AppendKnowledgeUserEvent: %w", err)
	}
	return nil
}

func (r *Repository) scanEvents(ctx context.Context, query string, args ...interface{}) ([]KnowledgeEvent, error) {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("scanEvents: %w", err)
	}
	defer rows.Close()

	var events []KnowledgeEvent
	for rows.Next() {
		var e KnowledgeEvent
		if err := rows.Scan(
			&e.EventID, &e.EventSeq, &e.OccurredAt, &e.TenantID, &e.UserID,
			&e.ActorType, &e.ActorID, &e.EventType, &e.AggregateType, &e.AggregateID,
			&e.CorrelationID, &e.CausationID, &e.DedupeKey, &e.Payload,
		); err != nil {
			return nil, fmt.Errorf("scanEvents scan: %w", err)
		}
		events = append(events, e)
	}
	return events, nil
}
