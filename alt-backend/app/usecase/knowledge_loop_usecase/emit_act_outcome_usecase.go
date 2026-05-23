package knowledge_loop_usecase

import (
	"alt/domain"
	"alt/port/knowledge_event_port"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// EmitActOutcomeUsecase is the write-path for the FE-initiated closure of an
// OODA loop iteration. Before ADR-000912 the only producer of
// knowledge_loop.act_outcome.v1 was the 7-day no_engagement cron, which
// meant Surface Planner v2 only learned from the *absence* of engagement,
// never from positive signals. This usecase lets the frontend collapse
// dwell-time, ask-turn-count, and explicit "I got this" CTAs into the
// same event vocabulary so the projector picks up positive signals in
// real time.
//
// Idempotency: the dedupe_key is derived from
// (event_type, user_id, entry_key, client_outcome_id) so a retried emit
// collides at the knowledge_event_dedupes UNIQUE constraint and the
// AppendKnowledgeEvent driver returns event_seq == 0 to signal "already
// recorded". The cron emits with a distinct dedupe_key namespace
// (...:<acted_event_id>:no_engagement) so the two producers cannot
// collide accidentally.
type EmitActOutcomeUsecase struct {
	appendPort knowledge_event_port.AppendKnowledgeEventPort
	nowFunc    func() time.Time
}

// NewEmitActOutcomeUsecase wires the usecase. nowFunc is injectable so tests
// can pin a deterministic clock; production passes time.Now.
func NewEmitActOutcomeUsecase(
	appendPort knowledge_event_port.AppendKnowledgeEventPort,
	nowFunc func() time.Time,
) *EmitActOutcomeUsecase {
	if nowFunc == nil {
		nowFunc = time.Now
	}
	return &EmitActOutcomeUsecase{appendPort: appendPort, nowFunc: nowFunc}
}

// EmitActOutcomeInput is the validated usecase input. All fields originate
// from the EmitActOutcome RPC request after JWT-claim extraction.
type EmitActOutcomeInput struct {
	TenantID        uuid.UUID
	UserID          uuid.UUID
	LensModeID      string
	EntryKey        string
	Outcome         string // raw proto enum name, e.g. "ACT_OUTCOME_KIND_ENGAGED"
	ClientOutcomeID string // UUIDv7, client-generated
	OccurredAt      time.Time
	DwellSeconds    *uint32
	AskTurns        *uint32
}

// EmitActOutcomeResult mirrors the proto response.
type EmitActOutcomeResult struct {
	Accepted     bool
	Deduplicated bool
	EventSeq     int64
}

// Execute validates, builds the act_outcome.v1 event, and appends it. A
// duplicate emit returns Accepted=false, Deduplicated=true, EventSeq=0 so
// the caller can surface an idempotent-OK response without re-emitting.
func (u *EmitActOutcomeUsecase) Execute(ctx context.Context, in EmitActOutcomeInput) (*EmitActOutcomeResult, error) {
	lensModeID := in.LensModeID
	if lensModeID == "" {
		lensModeID = "default"
	}
	if err := ValidateKeyFormat("lens_mode_id", lensModeID); err != nil {
		return nil, err
	}
	if err := ValidateKeyFormat("entry_key", in.EntryKey); err != nil {
		return nil, err
	}
	if err := ValidateClientTransitionID(in.ClientOutcomeID, u.nowFunc()); err != nil {
		return nil, fmt.Errorf("client_outcome_id: %w", err)
	}
	outcome := strings.TrimSpace(in.Outcome)
	if !isValidActOutcomeEnum(outcome) {
		return nil, fmt.Errorf("%w: unknown outcome %q", ErrInvalidArgument, outcome)
	}
	if in.OccurredAt.IsZero() {
		return nil, fmt.Errorf("%w: occurred_at required", ErrInvalidArgument)
	}

	payload := map[string]any{
		"entry_key":    in.EntryKey,
		"lens_mode_id": lensModeID,
		// Lower-case the outcome label so it matches what the cron writes —
		// the projector's normalisation in observeActOutcomeEmitted expects
		// lower-case strings.
		"outcome":     enumToOutcomeLabel(outcome),
		"observed_at": in.OccurredAt.UTC().Format(time.RFC3339Nano),
		"emitter":     "frontend",
	}
	if in.DwellSeconds != nil {
		payload["dwell_seconds"] = *in.DwellSeconds
	}
	if in.AskTurns != nil {
		payload["ask_turns"] = *in.AskTurns
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal act_outcome payload: %w", err)
	}

	// dedupe_key namespace separated from the cron's
	// `...:<acted_event_id>:no_engagement` form. Including
	// client_outcome_id here is the idempotency contract — same key
	// on retry collapses at the knowledge_event_dedupes UNIQUE
	// constraint.
	dedupeKey := fmt.Sprintf(
		"%s:fe:%s:%s:%s",
		domain.EventKnowledgeLoopActOutcome,
		in.UserID.String(),
		in.EntryKey,
		in.ClientOutcomeID,
	)

	uid := in.UserID
	event := domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    in.OccurredAt.UTC(),
		TenantID:      in.TenantID,
		UserID:        &uid,
		ActorType:     "user",
		ActorID:       in.UserID.String(),
		EventType:     domain.EventKnowledgeLoopActOutcome,
		AggregateType: "knowledge_loop_entry",
		AggregateID:   in.EntryKey,
		DedupeKey:     dedupeKey,
		Payload:       body,
	}

	seq, err := u.appendPort.AppendKnowledgeEvent(ctx, event)
	if err != nil {
		return nil, fmt.Errorf("append act_outcome event: %w", err)
	}
	if seq == 0 {
		// Driver signals "dedupe hit, no row inserted". Return
		// idempotent-OK so the FE caller treats a retry as success.
		return &EmitActOutcomeResult{
			Accepted:     false,
			Deduplicated: true,
			EventSeq:     0,
		}, nil
	}
	return &EmitActOutcomeResult{
		Accepted:     true,
		Deduplicated: false,
		EventSeq:     seq,
	}, nil
}

// Allowed outcome enum names. ACT_OUTCOME_KIND_NO_ENGAGEMENT is the cron's
// exclusive label and is rejected on the FE-emit path so the two producers
// stay separable in the event log. INTERNALIZED is a dismiss_state
// transition, not an outcome — it routes through TransitionKnowledgeLoop
// rather than this usecase.
var fePermittedOutcomes = map[string]string{
	"ACT_OUTCOME_KIND_ENGAGED":         "engaged",
	"ACT_OUTCOME_KIND_DEEP_ENGAGEMENT": "deep_engagement",
	"ACT_OUTCOME_KIND_STALE_SAVE":      "stale_save",
	"ACT_OUTCOME_KIND_ACCEPTED_CHANGE": "accepted_change",
}

func isValidActOutcomeEnum(s string) bool {
	_, ok := fePermittedOutcomes[s]
	return ok
}

func enumToOutcomeLabel(s string) string {
	if label, ok := fePermittedOutcomes[s]; ok {
		return label
	}
	return ""
}
