package knowledge_loop_usecase

import (
	"alt/domain"
	"alt/port/knowledge_event_port"
	"alt/port/knowledge_loop_port"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// TransitionKnowledgeLoopUsecase orchestrates the write path for a Loop stage transition.
//
// Flow (ADR-000831 §5, §8 and knowledge-loop-canonical-contract §8):
//  1. Validate inputs (format, UUIDv7 window, dwell-target coupling).
//  2. Classify the transition to its canonical event_type. Forbidden transitions are
//     rejected before any side effect.
//  3. Apply the server-side rate limit (canonical contract §8.4). Observed events
//     throttle per (user, entry, lens); all Loop events count against the user
//     global ceiling of 600/minute.
//  4. Reserve the idempotency barrier (ingest-only; not part of projection).
//  5. If the reservation is fresh, append the Loop event into knowledge_events.
//     The projector later consumes the event to update session_state / entries / surfaces.
//
// Single emission rule (ADR-000831 §3.8): transitions originating from /loop emit
// only the Loop event; they must NOT also emit HomeItem* events. /feeds → HomeItem*,
// /loop → KnowledgeLoop* — never both for the same user intent.
type TransitionKnowledgeLoopUsecase struct {
	dedupePort  knowledge_loop_port.ReserveTransitionIdempotencyPort
	appendPort  knowledge_event_port.AppendKnowledgeEventPort
	rateLimiter *LoopRateLimiter
	nowFunc     func() time.Time
}

// NewTransitionKnowledgeLoopUsecase wires the usecase. nowFunc is injectable to keep tests deterministic.
// appendPort may be nil in degraded wiring contexts (tests without event append verification),
// in which case the append step is silently skipped so the idempotency reserve still runs.
// rateLimiter may be nil in tests that exercise unrelated paths; production wiring
// always supplies a shared limiter so the 600/minute ceiling holds across connections.
func NewTransitionKnowledgeLoopUsecase(
	dedupePort knowledge_loop_port.ReserveTransitionIdempotencyPort,
	appendPort knowledge_event_port.AppendKnowledgeEventPort,
	rateLimiter *LoopRateLimiter,
	nowFunc func() time.Time,
) *TransitionKnowledgeLoopUsecase {
	if nowFunc == nil {
		nowFunc = time.Now
	}
	return &TransitionKnowledgeLoopUsecase{
		dedupePort:  dedupePort,
		appendPort:  appendPort,
		rateLimiter: rateLimiter,
		nowFunc:     nowFunc,
	}
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

// Execute validates the request, classifies the transition, reserves the idempotency
// key, and appends the corresponding Loop event. On duplicate key (within TTL),
// returns the cached response with accepted=false so the caller can surface the
// original outcome without re-emitting the event.
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

	// Classify before any side effect so forbidden transitions never hit the dedupe table.
	eventType, err := ClassifyTransitionEvent(in.FromStage, in.ToStage, in.Trigger)
	if err != nil {
		return nil, err
	}

	// Rate limit (canonical contract §8.4). Observed events use the per-entry
	// throttle; other transitions go straight to the global ceiling. Rejection
	// surfaces as ErrRateLimited so the handler can map it to CodeResourceExhausted
	// and the BFF to HTTP 429 (ADR-000839 classification table).
	if u.rateLimiter != nil {
		now := u.nowFunc()
		var allowed bool
		var reason string
		if eventType == domain.EventKnowledgeLoopObserved {
			allowed, reason = u.rateLimiter.AllowObserve(in.UserID, in.LensModeID, in.EntryKey, now)
		} else {
			allowed, reason = u.rateLimiter.AllowGlobal(in.UserID, now)
		}
		if !allowed {
			return nil, fmt.Errorf("transition_knowledge_loop: %w: %s", ErrRateLimited, reason)
		}
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

	if u.appendPort != nil {
		event, buildErr := buildTransitionEvent(eventType, in, u.nowFunc())
		if buildErr != nil {
			return nil, fmt.Errorf("transition_knowledge_loop: build event: %w", buildErr)
		}
		if err := u.appendPort.AppendKnowledgeEvent(ctx, event); err != nil {
			return nil, fmt.Errorf("transition_knowledge_loop: append event: %w", ClassifyDriverError(err))
		}
	}

	return &TransitionResult{Accepted: true}, nil
}

// buildTransitionEvent constructs the KnowledgeEvent for append. The payload is
// reproject-safe: it carries every field needed to recompute session_state
// deltas without reading latest projection state.
//
// dedupe_key equals client_transition_id so the slow-path knowledge_events unique
// index is unified with the fast-path knowledge_loop_transition_dedupes barrier.
func buildTransitionEvent(eventType string, in TransitionInput, occurredAt time.Time) (domain.KnowledgeEvent, error) {
	payload, err := json.Marshal(map[string]any{
		"entry_key":                    in.EntryKey,
		"lens_mode_id":                 in.LensModeID,
		"from_stage":                   in.FromStage,
		"to_stage":                     in.ToStage,
		"trigger":                      in.Trigger,
		"observed_projection_revision": in.ObservedProjRevision,
		"client_transition_id":         in.ClientTransitionID,
	})
	if err != nil {
		return domain.KnowledgeEvent{}, err
	}

	userID := in.UserID
	return domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    occurredAt,
		TenantID:      in.TenantID,
		UserID:        &userID,
		ActorType:     domain.ActorUser,
		ActorID:       in.UserID.String(),
		EventType:     eventType,
		AggregateType: domain.AggregateLoopSession,
		AggregateID:   in.EntryKey,
		DedupeKey:     in.ClientTransitionID,
		Payload:       payload,
	}, nil
}
