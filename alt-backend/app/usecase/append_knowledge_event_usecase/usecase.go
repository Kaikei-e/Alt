package append_knowledge_event_usecase

import (
	"alt/domain"
	"alt/port/knowledge_event_port"
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// AppendKnowledgeEventUsecase appends events to the knowledge event store.
type AppendKnowledgeEventUsecase struct {
	eventPort knowledge_event_port.AppendKnowledgeEventPort
}

// NewAppendKnowledgeEventUsecase creates a new AppendKnowledgeEventUsecase.
func NewAppendKnowledgeEventUsecase(
	eventPort knowledge_event_port.AppendKnowledgeEventPort,
) *AppendKnowledgeEventUsecase {
	return &AppendKnowledgeEventUsecase{eventPort: eventPort}
}

// Execute appends a knowledge event.
func (u *AppendKnowledgeEventUsecase) Execute(ctx context.Context, event domain.KnowledgeEvent) error {
	if event.EventType == "" {
		return errors.New("event_type is required")
	}
	if event.AggregateType == "" {
		return errors.New("aggregate_type is required")
	}
	if event.AggregateID == "" {
		return errors.New("aggregate_id is required")
	}

	// Generate ID and timestamp if not set
	if event.EventID == uuid.Nil {
		event.EventID = uuid.New()
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now()
	}
	if event.DedupeKey == "" {
		event.DedupeKey = event.EventType + ":" + event.AggregateID + ":" + event.EventID.String()
	}

	return u.eventPort.AppendKnowledgeEvent(ctx, event)
}
