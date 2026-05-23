package knowledge_loop_projector

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"knowledge-sovereign/driver/sovereign_db"
	"knowledge-sovereign/usecase/knowledge_loop_session_state"
)

// macroStateLookback is the 7-day window the macro layer summarises. The
// constant lives here (not in the session_state package) because the
// projector owns scheduling decisions; the pure builder is window-agnostic
// and accepts the lookback as an argument.
const macroStateLookback = 7 * 24 * time.Hour

// macroStateMaxEventsPerWindow caps how many in-window events the projector
// will reduce per recompute. The reducer is O(N) so the bound is a soft
// safety rail rather than a correctness invariant — even a busy user
// rarely emits more than a few hundred Loop events per week.
const macroStateMaxEventsPerWindow = 4096

// macroStateEventTypes is the set of event types the macro layer cares
// about. Keeping it explicit means the SQL filter (`event_type = ANY($)`)
// keeps the projector from scanning the entire event log; if a new event
// type starts feeding macro_state, add it here and the runbook receives
// the bump signal.
var macroStateEventTypes = []string{
	EventKnowledgeLoopActed,
	EventKnowledgeLoopReviewed,
	EventKnowledgeLoopActOutcome,
}

// recomputeMacroState rebuilds the macro projection row for the user
// associated with `ev` and writes it via the driver. The window right
// edge is ev.OccurredAt (event-time purity), the left edge is
// windowEnd - macroStateLookback.
//
// Reproject-safety: the reducer reads no projection row, no latest-state
// snapshot, no wall-clock; it only reduces in-window event payload. The
// driver upsert enforces the seq_hiwater guard so out-of-order replays
// collapse to a no-op.
//
// Errors are non-fatal at the projector level — the caller decides
// whether to surface or swallow. We log the error here so operators can
// see the macro layer falling behind even when the rest of the projection
// keeps advancing.
func (p *Projector) recomputeMacroState(ctx context.Context, ev *sovereign_db.KnowledgeEvent, lensModeID string) error {
	if ev == nil || ev.UserID == nil {
		return nil
	}
	windowEnd := ev.OccurredAt.UTC()
	windowStart := windowEnd.Add(-macroStateLookback)

	events, err := p.repo.ListKnowledgeEventsForUserInWindow(
		ctx,
		*ev.UserID,
		macroStateEventTypes,
		windowStart,
		// `until` is exclusive in the driver query (occurred_at < until),
		// so widen by 1ns to include the triggering event itself.
		windowEnd.Add(time.Nanosecond),
		macroStateMaxEventsPerWindow,
	)
	if err != nil {
		return fmt.Errorf("macro_state: list events in window: %w", err)
	}

	weights := knowledge_loop_session_state.LookupLensModeWeights(
		knowledge_loop_session_state.LensModeID(lensModeID),
	)
	state := knowledge_loop_session_state.BuildMacroState(
		events,
		windowEnd,
		macroStateLookback,
		weights,
		knowledge_loop_session_state.LensWeightsVersion,
	)

	row := sovereign_db.KnowledgeLoopMacroStateRow{
		UserID:                  *ev.UserID,
		TenantID:                ev.TenantID,
		LensModeID:              lensModeID,
		ActiveContinueThreads:   state.ActiveContinueThreads,
		PendingReviewCount:      state.PendingReviewCount,
		RecentInternalizedCount: state.RecentInternalizedCount,
		CognitiveLoadHint:       cognitiveLoadHintEnum(state.CognitiveLoadHint),
		WindowStartAt:           state.WindowStartAt,
		WindowEndAt:             state.WindowEndAt,
		SeqHiwater:              state.SeqHiwater,
		LensWeightsVersion:      state.LensWeightsVersion,
	}

	if _, err := p.repo.UpsertKnowledgeLoopMacroState(ctx, row); err != nil {
		return fmt.Errorf("macro_state: upsert: %w", err)
	}
	p.logger.DebugContext(ctx, "knowledge_loop_projector: macro_state recomputed",
		slog.String("user_id", ev.UserID.String()),
		slog.String("lens_mode_id", lensModeID),
		slog.Int64("seq_hiwater", state.SeqHiwater),
		slog.Int("active_continue", int(state.ActiveContinueThreads)),
		slog.Int("pending_review", int(state.PendingReviewCount)),
		slog.Int("recent_internalized", int(state.RecentInternalizedCount)),
	)
	return nil
}

// cognitiveLoadHintEnum normalises the builder's typed-alias hint into the
// Postgres enum literal the driver expects. Keeping the mapping here (not
// in the driver) avoids forcing the driver to import the builder package.
func cognitiveLoadHintEnum(h knowledge_loop_session_state.CognitiveLoadHint) string {
	switch h {
	case knowledge_loop_session_state.CognitiveLoadHintLight:
		return "light"
	case knowledge_loop_session_state.CognitiveLoadHintMedium:
		return "medium"
	case knowledge_loop_session_state.CognitiveLoadHintHeavy:
		return "heavy"
	default:
		return "unspecified"
	}
}
