package alt_db

import (
	"alt/domain"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

type knowledgeEventExecer interface {
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
}

// appendKnowledgeEventWithExec inserts an event into knowledge_events using the
// provided executor (pool or transaction). It is idempotent on dedupe_key.
func appendKnowledgeEventWithExec(ctx context.Context, execer knowledgeEventExecer, event domain.KnowledgeEvent) error {
	query := `INSERT INTO knowledge_events
		(event_id, occurred_at, tenant_id, user_id, actor_type, actor_id,
		 event_type, aggregate_type, aggregate_id, correlation_id, causation_id,
		 dedupe_key, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (dedupe_key) DO NOTHING`

	_, err := execer.Exec(ctx, query,
		event.EventID, event.OccurredAt, event.TenantID, event.UserID,
		event.ActorType, event.ActorID, event.EventType, event.AggregateType,
		event.AggregateID, event.CorrelationID, event.CausationID,
		event.DedupeKey, event.Payload,
	)
	if err != nil {
		return fmt.Errorf("append knowledge event: %w", err)
	}

	return nil
}

func buildArticleCreatedKnowledgeEvent(articleID uuid.UUID, tenantID uuid.UUID, userID *uuid.UUID, title string, publishedAt *time.Time) (domain.KnowledgeEvent, error) {
	payload := map[string]string{
		"article_id": articleID.String(),
		"title":      title,
		"tenant_id":  tenantID.String(),
	}
	if publishedAt != nil && !publishedAt.IsZero() {
		payload["published_at"] = publishedAt.Format(time.RFC3339)
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return domain.KnowledgeEvent{}, fmt.Errorf("marshal article created payload: %w", err)
	}

	return domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    time.Now(),
		TenantID:      tenantID,
		UserID:        userID,
		ActorType:     domain.ActorService,
		ActorID:       "article-store",
		EventType:     domain.EventArticleCreated,
		AggregateType: domain.AggregateArticle,
		AggregateID:   articleID.String(),
		DedupeKey:     "article-created:" + articleID.String(),
		Payload:       payloadBytes,
	}, nil
}

func buildArticleUpdatedKnowledgeEvent(articleID uuid.UUID, tenantID uuid.UUID, userID *uuid.UUID, title string, publishedAt *time.Time) (domain.KnowledgeEvent, error) {
	payload := map[string]string{
		"article_id": articleID.String(),
		"title":      title,
		"tenant_id":  tenantID.String(),
	}
	if publishedAt != nil && !publishedAt.IsZero() {
		payload["published_at"] = publishedAt.Format(time.RFC3339)
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return domain.KnowledgeEvent{}, fmt.Errorf("marshal article updated payload: %w", err)
	}

	return domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    time.Now(),
		TenantID:      tenantID,
		UserID:        userID,
		ActorType:     domain.ActorService,
		ActorID:       "article-store",
		EventType:     domain.EventArticleUpdated,
		AggregateType: domain.AggregateArticle,
		AggregateID:   articleID.String(),
		DedupeKey:     "article-updated:" + articleID.String() + ":" + uuid.NewString(),
		Payload:       payloadBytes,
	}, nil
}
