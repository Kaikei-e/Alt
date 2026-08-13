package recall_dismiss_usecase

import (
	"alt/domain"
	"alt/orchestrator/port/recall_candidate_port"
	"alt/shared/port/knowledge_event_port"
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
	// occurredAt is the origination time of this command, minted once here and
	// forwarded both to the direct recall_candidate_view mutation and the
	// appended knowledge event, so the two writes agree on a single wall-clock
	// moment instead of drifting across two separate time.Now() calls.
	occurredAt := time.Now()

	payload, _ := json.Marshal(map[string]any{
		"item_key": itemKey,
	})

	event := domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    occurredAt,
		TenantID:      tenantID,
		UserID:        &userID,
		ActorType:     domain.ActorUser,
		ActorID:       userID.String(),
		EventType:     domain.EventRecallDismissed,
		AggregateType: domain.AggregateHomeSession,
		AggregateID:   itemKey,
		DedupeKey:     fmt.Sprintf("recall_dismiss:%s:%s:%d", userID, itemKey, occurredAt.Unix()),
		Payload:       payload,
	}

	// The event is the durable write: recall_candidate_view is TRUNCATEd and
	// refolded from the event log on reproject, so a dismiss that only reached
	// the read model resurfaces. Append first and fail the command if it does
	// not land — the dedupe key makes a client retry idempotent.
	if _, err := u.eventPort.AppendKnowledgeEvent(ctx, event); err != nil {
		return fmt.Errorf("append recall dismissed event: %w", err)
	}

	if err := u.candidatePort.DismissRecallCandidate(ctx, userID, itemKey, occurredAt); err != nil {
		return fmt.Errorf("dismiss recall candidate: %w", err)
	}

	return nil
}
