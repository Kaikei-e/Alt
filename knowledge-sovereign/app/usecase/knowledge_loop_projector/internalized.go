package knowledge_loop_projector

import (
	"context"
	"fmt"
	"log/slog"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// projectInternalized handles knowledge_loop.internalized.v1 events
// (ADR-000914). It is the "I got this" graduation producer pair to the
// existing DISMISS_STATE_INTERNALIZED enum value introduced by
// ADR-000908 §Δ3. The branch is intentionally minimal — same shape as
// the Deferred patch branch in projectTransition:
//
//  1. Pull entry_key + lens_mode_id from the transition payload (shares
//     the layout produced by /loop/transition for every same-stage trigger).
//  2. Patch dismiss_state via the existing seq-hiwater-guarded driver
//     so freshness_at / why_text / surface_bucket / continue_context are
//     all preserved untouched. No surfaces recompute is needed because
//     internalized rows are filtered at the read layer.
//  3. Increment the existing internalized counter so the
//     [[000910]] dashboard reflects FE-produced graduations alongside the
//     act_outcome=internalized path.
//
// Reproject-safety: the branch only reads the event payload + event_seq;
// the patch driver enforces seq_hiwater monotonicity. Replay of the same
// event is a no-op at the SQL boundary.
func (p *Projector) projectInternalized(ctx context.Context, ev *sovereign_db.KnowledgeEvent) (*sovereign_db.KnowledgeLoopUpsertResult, error) {
	payload := parseLoopTransitionPayload(ev.Payload)
	lensModeID := payload.LensModeID
	if lensModeID == "" {
		lensModeID = defaultLensModeID
	}
	entryKey := payload.EntryKey
	if entryKey == "" {
		// The classifier rejects internalize transitions without an
		// entry_key (same-stage trigger validator requires the metadata
		// envelope), so an empty key here means a stray system-emitted
		// or malformed event. Skip silently rather than fail the batch.
		p.logger.WarnContext(ctx, "knowledge_loop_projector: internalize event without entry_key skipped",
			slog.Int64("event_seq", ev.EventSeq),
		)
		return nil, nil
	}

	res, err := p.repo.PatchKnowledgeLoopEntryDismissState(
		ctx,
		ev.UserID.String(),
		ev.TenantID.String(),
		lensModeID,
		entryKey,
		ev.EventSeq,
		sovereignv1.DismissState_DISMISS_STATE_INTERNALIZED,
	)
	if err != nil {
		return res, fmt.Errorf("patch dismiss_state on Internalized: %w", err)
	}
	if res != nil && res.Applied {
		// Only count rows that actually flipped — re-applied events whose
		// seq_hiwater guard rejected the patch must not double-count.
		observeInternalizedTransition()
	}

	// Macro state recompute mirrors the act_outcome=internalized path so
	// MacroByline's recent_internalized_count surfaces FE-produced
	// graduations alongside the cron / outcome path. ADR-000911 §Δ2
	// supplement.
	if err := p.recomputeMacroState(ctx, ev, lensModeID); err != nil {
		p.logger.WarnContext(ctx, "knowledge_loop_projector: recompute macro state after internalize failed",
			slog.Int64("event_seq", ev.EventSeq),
			slog.String("err", err.Error()),
		)
	}
	return res, nil
}
