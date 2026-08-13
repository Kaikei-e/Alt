package recall_snooze_usecase

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
	// occurredAt is the origination time of this command, minted once here
	// and forwarded both to the direct recall_candidate_view mutation and the
	// appended knowledge event, so the two writes agree on a single wall-clock
	// moment instead of drifting across two separate time.Now() calls.
	occurredAt := time.Now()
	until := occurredAt.Add(time.Duration(snoozeHours) * time.Hour)

	payload, _ := json.Marshal(map[string]any{
		"item_key":      itemKey,
		"snooze_hours":  snoozeHours,
		"snoozed_until": until.Format(time.RFC3339),
	})

	event := domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    occurredAt,
		TenantID:      tenantID,
		UserID:        &userID,
		ActorType:     domain.ActorUser,
		ActorID:       userID.String(),
		EventType:     domain.EventRecallSnoozed,
		AggregateType: domain.AggregateHomeSession,
		AggregateID:   itemKey,
		DedupeKey:     fmt.Sprintf("recall_snooze:%s:%s:%d", userID, itemKey, occurredAt.Unix()),
		Payload:       payload,
	}

	// The event is the durable write: snoozed_until is refolded from the event
	// log on reproject, so a snooze that only reached recall_candidate_view
	// reverts to NULL and the item resurfaces. Append first and fail the
	// command if it does not land — the dedupe key makes a client retry
	// idempotent.
	if _, err := u.eventPort.AppendKnowledgeEvent(ctx, event); err != nil {
		return fmt.Errorf("append recall snoozed event: %w", err)
	}

	if err := u.candidatePort.SnoozeRecallCandidate(ctx, userID, itemKey, until, occurredAt); err != nil {
		return fmt.Errorf("snooze recall candidate: %w", err)
	}

	return nil
}
