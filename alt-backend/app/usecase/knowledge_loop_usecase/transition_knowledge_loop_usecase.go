package knowledge_loop_usecase

import (
	"alt/port/knowledge_loop_port"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// TransitionKnowledgeLoopUsecase orchestrates the write path for a Loop stage transition.
// It enforces the UUIDv7 idempotency barrier and validates DWELL→OBSERVE coupling.
// Event append itself lives in the KnowledgeLoopProjectorJob / event append driver; this
// usecase produces the validated request that the handler emits.
type TransitionKnowledgeLoopUsecase struct {
	dedupePort knowledge_loop_port.ReserveTransitionIdempotencyPort
	nowFunc    func() time.Time
}

// NewTransitionKnowledgeLoopUsecase wires the usecase. nowFunc is injectable to keep tests deterministic.
func NewTransitionKnowledgeLoopUsecase(
	dedupePort knowledge_loop_port.ReserveTransitionIdempotencyPort,
	nowFunc func() time.Time,
) *TransitionKnowledgeLoopUsecase {
	if nowFunc == nil {
		nowFunc = time.Now
	}
	return &TransitionKnowledgeLoopUsecase{dedupePort: dedupePort, nowFunc: nowFunc}
}

// TransitionInput is the validated usecase input.
type TransitionInput struct {
	TenantID             uuid.UUID
	UserID               uuid.UUID
	LensModeID           string
	ClientTransitionID   string
	EntryKey             string
	FromStage            string // raw proto enum name (e.g. "LOOP_STAGE_OBSERVE")
	ToStage              string
	Trigger              string // raw proto enum name (e.g. "TRANSITION_TRIGGER_DWELL")
	ObservedProjRevision int64
}

// TransitionResult mirrors the proto response contract.
type TransitionResult struct {
	Accepted          bool
	CanonicalEntryKey *string
	Message           *string
}

// Execute validates the request and reserves the idempotency key. On duplicate key (within TTL),
// returns the cached response with accepted=false so the caller can surface the original outcome.
// On fresh claim, returns accepted=true; the handler / downstream projector are responsible for
// appending the corresponding KnowledgeLoop* event.
func (u *TransitionKnowledgeLoopUsecase) Execute(ctx context.Context, in TransitionInput) (*TransitionResult, error) {
	if err := ValidateKeyFormat("lens_mode_id", in.LensModeID); err != nil {
		return nil, err
	}
	if err := ValidateKeyFormat("entry_key", in.EntryKey); err != nil {
		return nil, err
	}
	if err := ValidateClientTransitionID(in.ClientTransitionID, u.nowFunc()); err != nil {
		return nil, err
	}
	if err := ValidateObservedProjectionRevision(in.ObservedProjRevision); err != nil {
		return nil, err
	}
	if err := ValidateDwellTriggerTarget(in.Trigger, in.ToStage); err != nil {
		return nil, err
	}

	reserved, cached, err := u.dedupePort.ReserveTransitionIdempotency(ctx, in.UserID, in.ClientTransitionID)
	if err != nil {
		return nil, fmt.Errorf("transition_knowledge_loop: reserve idempotency: %w", ClassifyDriverError(err))
	}
	if !reserved {
		msg := "duplicate client_transition_id; returning cached response"
		res := &TransitionResult{Accepted: false, Message: &msg}
		if cached != nil && cached.CanonicalEntryKey != nil {
			res.CanonicalEntryKey = cached.CanonicalEntryKey
		}
		return res, nil
	}
	return &TransitionResult{Accepted: true}, nil
}
