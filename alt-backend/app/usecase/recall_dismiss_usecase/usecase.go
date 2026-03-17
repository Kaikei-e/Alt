package recall_dismiss_usecase

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

type RecallDismissUsecase struct {
	candidatePort recall_candidate_port.DismissRecallCandidatePort
	eventPort     knowledge_event_port.AppendKnowledgeEventPort
}

func NewRecallDismissUsecase(
	candidatePort recall_candidate_port.DismissRecallCandidatePort,
	eventPort knowledge_event_port.AppendKnowledgeEventPort,
) *RecallDismissUsecase {
	return &RecallDismissUsecase{
		candidatePort: candidatePort,
		eventPort:     eventPort,
	}
}

func (u *RecallDismissUsecase) Execute(ctx context.Context, userID uuid.UUID, tenantID uuid.UUID, itemKey string) error {
	if err := u.candidatePort.DismissRecallCandidate(ctx, userID, itemKey); err != nil {
		return fmt.Errorf("dismiss recall candidate: %w", err)
	}

	payload, _ := json.Marshal(map[string]any{
		"item_key": itemKey,
	})

	event := domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    time.Now(),
		TenantID:      tenantID,
		UserID:        &userID,
		ActorType:     domain.ActorUser,
		ActorID:       userID.String(),
		EventType:     domain.EventRecallDismissed,
		AggregateType: domain.AggregateHomeSession,
		AggregateID:   itemKey,
		DedupeKey:     fmt.Sprintf("recall_dismiss:%s:%s:%d", userID, itemKey, time.Now().Unix()),
		Payload:       payload,
	}

	_ = u.eventPort.AppendKnowledgeEvent(ctx, event)
	return nil
}
