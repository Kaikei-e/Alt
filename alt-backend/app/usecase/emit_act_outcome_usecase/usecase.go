// Package emit_act_outcome_usecase appends a system-emitted
// KnowledgeLoopActOutcome event when an alt-backend view tracker observes
// downstream engagement on a prior KnowledgeLoopActed event (ADR-000908
// §Δ1). The cron-driven `no_engagement` fallback path lives in
// knowledge-sovereign's act_outcome_cron and is intentionally separated
// from this immediate path so wall-clock decisions stay out of business
// facts.
//
// Single emission: this usecase emits `knowledge_loop.act_outcome.v1`
// only — never another `knowledge_loop.acted.v1`. The acted event was
// already appended on the user's Open/Ask/Save action; the outcome event
// is the closure signal, not a repeat.
package emit_act_outcome_usecase

import (
	"alt/domain"
	"alt/port/knowledge_event_port"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Allowed outcome labels the alt-backend view tracker may emit. The
// system-only `no_engagement` label is deliberately excluded — it is
// reserved for act_outcome_cron's 7-day fallback path, and accepting it
// here would let a malicious frontend forge the cron's signal.
var allowedOutcomes = map[string]struct{}{
	"engaged":         {},
	"deep_engagement": {},
	"accepted_change": {},
	"stale_save":      {},
}

// Thresholds for DeriveOutcomeKind (ADR-000908 §Δ1).
const (
	engagedDwellMs       = 30_000 // ≥ 30s article dwell
	deepEngagementAskMin = 3      // ≥ 3 ask conversation turns
)

// DeriveOutcomeKind classifies a view-tracker observation onto the
// ActOutcomeKind enum. The function is pure: same inputs always yield the
// same (outcome, emit) pair regardless of when it runs. Returns ("unspecified",
// false) when neither threshold is cleared so the caller can skip the emit.
//
// Priority order: deep_engagement beats engaged when both thresholds clear,
// because the conversation-turn signal is a stronger indicator of
// understanding than dwell alone.
func DeriveOutcomeKind(dwellMs int64, askTurns int32) (string, bool) {
	if askTurns >= deepEngagementAskMin {
		return "deep_engagement", true
	}
	if dwellMs >= engagedDwellMs {
		return "engaged", true
	}
	return "unspecified", false
}

// Input is the usecase contract. The caller (handler) must have already
// authenticated the user and resolved tenant_id from the JWT — this
// usecase does not re-validate identity.
type Input struct {
	TenantID     uuid.UUID
	UserID       uuid.UUID
	LensModeID   string
	ActedEventID uuid.UUID
	EntryKey     string
	Outcome      string
	// ObservedAt is event-time. The handler binds it from the originating
	// acted event's occurred_at + observed window, never from wall-clock,
	// so reproject deterministically produces the same outcome row.
	ObservedAt time.Time
}

// Usecase emits one knowledge_loop.act_outcome.v1 event per call. It does
// not return the assigned seq because the immediate-emit path is
// fire-and-forget from the caller's perspective; a failed append is a
// metric (handled by the caller's log + counter), not a user-visible
// error.
type Usecase struct {
	appendPort knowledge_event_port.AppendKnowledgeEventPort
	nowFunc    func() time.Time
}

// New wires the usecase. nowFunc is injectable for tests but the runtime
// behaviour does NOT consume it for business facts — it is reserved for
// future fields (e.g. a server-side observation timestamp diagnostic).
func New(appendPort knowledge_event_port.AppendKnowledgeEventPort, nowFunc func() time.Time) *Usecase {
	if nowFunc == nil {
		nowFunc = time.Now
	}
	return &Usecase{appendPort: appendPort, nowFunc: nowFunc}
}

// Execute validates inputs and appends the outcome event. Errors fall
// through to the caller so the handler can log + bump a counter without
// surfacing the failure to the end-user (the outcome path is best-effort).
func (u *Usecase) Execute(ctx context.Context, in Input) error {
	if _, ok := allowedOutcomes[in.Outcome]; !ok {
		return fmt.Errorf("emit_act_outcome: outcome %q is not allowed for the view-tracker path (no_engagement is system-only)", in.Outcome)
	}
	if in.EntryKey == "" {
		return errors.New("emit_act_outcome: entry_key is required")
	}
	if in.ActedEventID == uuid.Nil {
		return errors.New("emit_act_outcome: acted_event_id is required")
	}
	if in.ObservedAt.IsZero() {
		return errors.New("emit_act_outcome: observed_at is required")
	}

	body := map[string]any{
		"acted_event_id": in.ActedEventID.String(),
		"entry_key":      in.EntryKey,
		"lens_mode_id":   in.LensModeID,
		"outcome":        in.Outcome,
		"observed_at":    in.ObservedAt.UTC().Format(time.RFC3339Nano),
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("emit_act_outcome: marshal payload: %w", err)
	}

	dedupeKey := fmt.Sprintf("%s:%s:%s",
		domain.EventKnowledgeLoopActOutcome,
		in.ActedEventID.String(),
		in.Outcome,
	)

	uid := in.UserID
	ev := domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    in.ObservedAt,
		TenantID:      in.TenantID,
		UserID:        &uid,
		ActorType:     domain.ActorSystem,
		ActorID:       "alt-backend-view-tracker",
		EventType:     domain.EventKnowledgeLoopActOutcome,
		AggregateType: "knowledge_loop_entry",
		AggregateID:   in.EntryKey,
		DedupeKey:     dedupeKey,
		Payload:       payload,
	}

	if _, err := u.appendPort.AppendKnowledgeEvent(ctx, ev); err != nil {
		return fmt.Errorf("emit_act_outcome: append event: %w", err)
	}
	return nil
}
