package alt_db

import (
	"alt/domain"
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// AppendKnowledgeUserEvent inserts a user interaction event.
// Idempotent when dedupe_key is set (UNIQUE constraint with partial index).
func (r *AltDBRepository) AppendKnowledgeUserEvent(ctx context.Context, event domain.KnowledgeUserEvent) error {
	ctx, span := otel.Tracer("alt-backend").Start(ctx, "db.AppendKnowledgeUserEvent")
	defer span.End()

	query := `INSERT INTO knowledge_user_events
		(user_event_id, occurred_at, user_id, tenant_id,
		 event_type, item_key, payload, dedupe_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT DO NOTHING`

	_, err := r.pool.Exec(ctx, query,
		event.UserEventID, event.OccurredAt, event.UserID, event.TenantID,
		event.EventType, event.ItemKey, event.Payload, event.DedupeKey,
	)
	if err != nil {
		return fmt.Errorf("AppendKnowledgeUserEvent: %w", err)
	}

	span.SetAttributes(attribute.String("event.type", event.EventType))
	return nil
}
