// Package emit_trail_outcome_usecase records the observed consequence of a
// taken Knowledge Trail branch by appending a trail.act_outcome.v1 event
// carrying the raw visible dwell. Classification (engaged / no_engagement) is a
// projector-side derivation — never baked into the emitted fact (D18).
package emit_trail_outcome_usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"alt/domain"
	"alt/shared/port/knowledge_event_port"

	"github.com/google/uuid"
)

// EventTrailActOutcome is the dwell-outcome event type. It replaces the
// historical knowledge_loop.act_outcome.v1 vocabulary, which is history-only
// and must never be emitted anew (D16).
const EventTrailActOutcome = "trail.act_outcome.v1"

// MaxDwellMs caps the recorded dwell at 24h.
const MaxDwellMs = int64(24 * 60 * 60 * 1000)

// ErrInvalidRequest wraps client-side validation failures so the handler can
// map them to InvalidArgument (vs an append failure, which is Internal).
var ErrInvalidRequest = errors.New("invalid emit-trail-outcome request")

// EmitTrailOutcomeUsecase appends act_outcome events.
type EmitTrailOutcomeUsecase struct {
	appendPort knowledge_event_port.AppendKnowledgeEventPort
}

func NewEmitTrailOutcomeUsecase(appendPort knowledge_event_port.AppendKnowledgeEventPort) *EmitTrailOutcomeUsecase {
	return &EmitTrailOutcomeUsecase{appendPort: appendPort}
}

// Execute validates and appends a dwell outcome for a taken branch. Idempotent
// per branch: the dedupe key is the proposal ref, so the first outcome wins and
// retries append nothing new (D19 — no client-minted id needed).
func (u *EmitTrailOutcomeUsecase) Execute(ctx context.Context, userID, tenantID uuid.UUID, branchKey, itemKey string, dwellMs int64) error {
	branchKey = strings.TrimSpace(branchKey)
	if branchKey == "" {
		return fmt.Errorf("%w: branch_key required", ErrInvalidRequest)
	}
	itemKey = strings.TrimSpace(itemKey)
	if itemKey == "" {
		return fmt.Errorf("%w: item_key required", ErrInvalidRequest)
	}
	if dwellMs < 0 {
		return fmt.Errorf("%w: dwell_ms must be non-negative", ErrInvalidRequest)
	}
	// A forgotten overnight tab must not mint absurd business facts.
	if dwellMs > MaxDwellMs {
		dwellMs = MaxDwellMs
	}

	payload, _ := json.Marshal(map[string]any{
		"branch_key": branchKey,
		"item_key":   itemKey,
		"dwell_ms":   dwellMs,
	})
	uid := userID
	evt := domain.KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    time.Now(),
		TenantID:      tenantID,
		UserID:        &uid,
		ActorType:     domain.ActorUser,
		ActorID:       userID.String(),
		EventType:     EventTrailActOutcome,
		AggregateType: "trail_branch",
		AggregateID:   branchKey,
		DedupeKey:     EventTrailActOutcome + ":" + branchKey,
		Payload:       payload,
	}
	if _, err := u.appendPort.AppendKnowledgeEvent(ctx, evt); err != nil {
		return fmt.Errorf("emit trail outcome: %w", err)
	}
	return nil
}
