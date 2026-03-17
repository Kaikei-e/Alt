package recall_snooze_usecase

import (
	"alt/domain"
	"alt/port/knowledge_event_port"
	"alt/port/recall_candidate_port"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type RecallSnoozeUsecase struct {
	candidatePort recall_candidate_port.SnoozeRecallCandidatePort
	eventPort     knowledge_event_port.AppendKnowledgeEventPort
}

func NewRecallSnoozeUsecase(
	candidatePort recall_candidate_port.SnoozeRecallCandidatePort,
	eventPort knowledge_event_port.AppendKnowledgeEventPort,
) *RecallSnoozeUsecase {
	return &RecallSnoozeUsecase{
		candidatePort: candidatePort,
		eventPort:     eventPort,
	}
}

func (u *RecallSnoozeUsecase) Execute(ctx context.Context, userID uuid.UUID, tenantID uuid.UUID, itemKey string, snoozeHours int) error {
	if snoozeHours <= 0 {
		snoozeHours = 24
	}
	until := time.Now().Add(time.Duration(snoozeHours) * time.Hour)

	if err := u.candidatePort.SnoozeRecallCandidate(ctx, userID, itemKey, until); err != nil {
		return fmt.Errorf("snooze recall candidate: %w", err)
	}

	payload, _ := json.Marshal(map[string]any{
		"item_key":     itemKey,
		"snooze_hours": snoozeHours,
		"snoozed_until": until.Format(time.RFC3339),
	})

	event := domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    time.Now(),
		TenantID:      tenantID,
		UserID:        &userID,
		ActorType:     domain.ActorUser,
		ActorID:       userID.String(),
		EventType:     domain.EventRecallSnoozed,
		AggregateType: domain.AggregateHomeSession,
		AggregateID:   itemKey,
		DedupeKey:     fmt.Sprintf("recall_snooze:%s:%s:%d", userID, itemKey, time.Now().Unix()),
		Payload:       payload,
	}

	_ = u.eventPort.AppendKnowledgeEvent(ctx, event)
	return nil
}
