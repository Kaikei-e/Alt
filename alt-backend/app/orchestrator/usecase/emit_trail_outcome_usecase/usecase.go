// Package emit_trail_outcome_usecase records the observed consequence of a
// taken Knowledge Trail branch by appending a trail.act_outcome.v1 event
// carrying the raw visible dwell. Classification (engaged / no_engagement) is a
// projector-side derivation — never baked into the emitted fact (D18).
package emit_trail_outcome_usecase

import (
	"context"
	"errors"

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

// Execute validates and appends a dwell outcome for a taken branch.
func (u *EmitTrailOutcomeUsecase) Execute(ctx context.Context, userID, tenantID uuid.UUID, branchKey, itemKey string, dwellMs int64) error {
	return errors.New("not implemented")
}
