package append_knowledge_event_usecase

import (
	"alt/domain"
	"alt/shared/port/knowledge_event_port"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	normalized, err := normalizeEvent(event, time.Now())
	if err != nil {
		return err
	}

	_, err = u.eventPort.AppendKnowledgeEvent(ctx, normalized)
	if err != nil {
		return fmt.Errorf("append knowledge event: %w", err)
	}
	return nil
}

// normalizeEvent validates and defaults an event's ID, timestamp, and dedupe key.
func normalizeEvent(event domain.KnowledgeEvent, now time.Time) (domain.KnowledgeEvent, error) {
	if event.EventType == "" {
		return domain.KnowledgeEvent{}, errors.New("event_type is required")
	}
	if event.AggregateType == "" {
		return domain.KnowledgeEvent{}, errors.New("aggregate_type is required")
	}
	if event.AggregateID == "" {
		return domain.KnowledgeEvent{}, errors.New("aggregate_id is required")
	}

	// Generate ID and timestamp if not set
	if event.EventID == uuid.Nil {
		event.EventID = uuid.New()
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = now
	}
	if event.DedupeKey == "" {
		event.DedupeKey = event.EventType + ":" + event.AggregateID + ":" + event.EventID.String()
	}

	// Validate ReasonMerged payload has required fields
	if event.EventType == domain.EventReasonMerged {
		if err := validateReasonMergedPayload(event.Payload); err != nil {
			return domain.KnowledgeEvent{}, err
		}
	}

	return event, nil
}

// reasonMergedPayload mirrors the projector's expected shape.
type reasonMergedPayload struct {
	ArticleID        string   `json:"article_id"`
	ItemKey          string   `json:"item_key"`
	AddedCodes       []string `json:"added_codes"`
	PreviousWhyCodes []string `json:"previous_why_codes"`
}

func validateReasonMergedPayload(raw json.RawMessage) error {
	if len(raw) == 0 {
		return errors.New("ReasonMerged payload is required")
	}
	var p reasonMergedPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return errors.New("ReasonMerged payload is invalid JSON")
	}
	if p.ArticleID == "" {
		return errors.New("ReasonMerged payload requires article_id")
	}
	if len(p.AddedCodes) == 0 {
		return errors.New("ReasonMerged payload requires at least one added_code")
	}
	return nil
}
