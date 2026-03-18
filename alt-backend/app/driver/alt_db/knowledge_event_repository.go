package alt_db

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// AppendKnowledgeEvent inserts an event into knowledge_events.
// Idempotent: ON CONFLICT (dedupe_key) DO NOTHING.
func (r *AltDBRepository) AppendKnowledgeEvent(ctx context.Context, event domain.KnowledgeEvent) error {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.AppendKnowledgeEvent")
	defer span.End()

	err := appendKnowledgeEventWithExec(ctx, r.pool, event)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			logger.Logger.ErrorContext(ctx, "failed to append knowledge event",
				"error", err, "pg_code", pgErr.Code, "dedupe_key", event.DedupeKey)
		}
		return fmt.Errorf("AppendKnowledgeEvent: %w", err)
	}

	span.SetAttributes(attribute.String("event.type", event.EventType))
	return nil
}

// ListKnowledgeEventsSinceForUser returns events scoped to a specific user (or system events with NULL user_id).
func (r *AltDBRepository) ListKnowledgeEventsSinceForUser(ctx context.Context, userID uuid.UUID, afterSeq int64, limit int) ([]domain.KnowledgeEvent, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.ListKnowledgeEventsSinceForUser")
	defer span.End()

	query := `SELECT event_id, event_seq, occurred_at, tenant_id, user_id,
		actor_type, actor_id, event_type, aggregate_type, aggregate_id,
		correlation_id, causation_id, dedupe_key, payload
		FROM knowledge_events
		WHERE event_seq > $1 AND (user_id = $2 OR user_id IS NULL)
		ORDER BY event_seq ASC
		LIMIT $3`

	rows, err := r.pool.Query(ctx, query, afterSeq, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("ListKnowledgeEventsSinceForUser: %w", err)
	}
	defer rows.Close()

	var events []domain.KnowledgeEvent
	for rows.Next() {
		var e domain.KnowledgeEvent
		err := rows.Scan(
			&e.EventID, &e.EventSeq, &e.OccurredAt, &e.TenantID, &e.UserID,
			&e.ActorType, &e.ActorID, &e.EventType, &e.AggregateType, &e.AggregateID,
			&e.CorrelationID, &e.CausationID, &e.DedupeKey, &e.Payload,
		)
		if err != nil {
			return nil, fmt.Errorf("ListKnowledgeEventsSinceForUser scan: %w", err)
		}
		events = append(events, e)
	}

	span.SetAttributes(attribute.Int("db.row_count", len(events)))
	return events, nil
}

// ListKnowledgeEventsSince returns events with event_seq > afterSeq, ordered by event_seq ASC.
func (r *AltDBRepository) ListKnowledgeEventsSince(ctx context.Context, afterSeq int64, limit int) ([]domain.KnowledgeEvent, error) {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.ListKnowledgeEventsSince")
	defer span.End()

	query := `SELECT event_id, event_seq, occurred_at, tenant_id, user_id,
		actor_type, actor_id, event_type, aggregate_type, aggregate_id,
		correlation_id, causation_id, dedupe_key, payload
		FROM knowledge_events
		WHERE event_seq > $1
		ORDER BY event_seq ASC
		LIMIT $2`

	rows, err := r.pool.Query(ctx, query, afterSeq, limit)
	if err != nil {
		return nil, fmt.Errorf("ListKnowledgeEventsSince: %w", err)
	}
	defer rows.Close()

	var events []domain.KnowledgeEvent
	for rows.Next() {
		var e domain.KnowledgeEvent
		err := rows.Scan(
			&e.EventID, &e.EventSeq, &e.OccurredAt, &e.TenantID, &e.UserID,
			&e.ActorType, &e.ActorID, &e.EventType, &e.AggregateType, &e.AggregateID,
			&e.CorrelationID, &e.CausationID, &e.DedupeKey, &e.Payload,
		)
		if err != nil {
			return nil, fmt.Errorf("ListKnowledgeEventsSince scan: %w", err)
		}
		events = append(events, e)
	}

	span.SetAttributes(attribute.Int("db.row_count", len(events)))
	return events, nil
}
