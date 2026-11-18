package reading_queue_usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"alt/domain"
	"alt/shared/port/knowledge_event_port"
)

type UnqueueArticleUsecase struct {
	appendPort knowledge_event_port.AppendKnowledgeEventPort
	now        func() time.Time
}

func NewUnqueueArticleUsecase(
	appendPort knowledge_event_port.AppendKnowledgeEventPort,
	now func() time.Time,
) *UnqueueArticleUsecase {
	if appendPort == nil {
		panic("reading_queue_usecase: appendPort is required")
	}
	if now == nil {
		panic("reading_queue_usecase: now clock is required")
	}
	return &UnqueueArticleUsecase{
		appendPort: appendPort,
		now:        now,
	}
}

type readingUnqueuedPayload struct {
	ClientEventID    string `json:"client_event_id"`
	ArticleID        string `json:"article_id"`
	ReceivedAt       string `json:"received_at"`
	ClientOccurredAt string `json:"client_occurred_at,omitempty"`
}

func (u *UnqueueArticleUsecase) Execute(
	ctx context.Context,
	userID uuid.UUID,
	tenantID uuid.UUID,
	rawArticleID string,
	rawClientEventID string,
	occurredAt time.Time,
) error {
	articleID, err := uuid.Parse(rawArticleID)
	if err != nil {
		return fmt.Errorf("%w: invalid article_id", ErrInvalidInput)
	}
	clientEventID, err := uuid.Parse(rawClientEventID)
	if err != nil {
		return fmt.Errorf("%w: invalid client_event_id", ErrInvalidInput)
	}
	if occurredAt.IsZero() {
		return fmt.Errorf("%w: occurred_at is required", ErrInvalidInput)
	}

	receivedAt := u.now().UTC()
	clampedOccurredAt, clientOccurredAt := domain.ClampReadingOccurredAt(occurredAt, receivedAt)

	payloadBytes, err := json.Marshal(readingUnqueuedPayload{
		ClientEventID:    clientEventID.String(),
		ArticleID:        articleID.String(),
		ReceivedAt:       receivedAt.Format(time.RFC3339),
		ClientOccurredAt: clientOccurredAt,
	})
	if err != nil {
		return fmt.Errorf("marshal reading.unqueued.v1 payload: %w", err)
	}

	uid := userID
	ke := domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    clampedOccurredAt,
		TenantID:      tenantID,
		UserID:        &uid,
		ActorType:     domain.ActorUser,
		ActorID:       userID.String(),
		EventType:     domain.EventReadingUnqueued,
		AggregateType: domain.AggregateReadingSession,
		AggregateID:   "article:" + articleID.String(),
		DedupeKey:     domain.EventReadingUnqueued + ":" + clientEventID.String(),
		Payload:       payloadBytes,
	}

	if _, err := u.appendPort.AppendKnowledgeEvent(ctx, ke); err != nil {
		return fmt.Errorf("unqueue article: %w", err)
	}
	return nil
}
