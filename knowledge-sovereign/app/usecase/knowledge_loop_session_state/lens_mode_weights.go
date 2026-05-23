// Package knowledge_loop_session_state derives the macro (day-to-week) layer
// of Knowledge Loop session state from the append-only knowledge_events log.
//
// Reproject-safety invariants:
//   - All outputs are pure functions of (events, windowEnd, lookback, weights).
//     No wall-clock, no latest-state queries, no random ordering dependency.
//   - windowEnd is supplied by the projector at consume time and equals the
//     occurred_at of the event that triggered the recompute (event-time
//     purity, see canonical contract §3).
//   - Lens weights are immutable compile-time constants keyed by
//     LensModeID; changing a weight requires bumping LensWeightsVersion and
//     scheduling a full reproject (see runbook).
package knowledge_loop_session_state

// LensWeightsVersion is the cohort version for the lens_mode_weights lookup
// table. Bump when a threshold changes; a bump triggers a full reproject
// via the knowledge-loop-reproject runbook because cognitive_load_hint
// values for historic rows must be recomputed under the new mapping.
//
// v1 (2026-05-24): initial cohort — default / focus / wide lenses use the
// same canonical thresholds (medium >= 3, heavy >= 7). ADR-000909 §Δ2
// supplement landing macro_state_builder.
const LensWeightsVersion int32 = 1

// LensModeID is the stable identifier the projector receives in event
// payload and session_state. Keep this as a typed string so callers cannot
// accidentally pass a SurfaceBucket id or similar lookalike.
type LensModeID string

const (
	// DefaultLensModeID is the lens every authenticated user starts with.
	DefaultLensModeID LensModeID = "default"
)

// LensModeWeights are the per-lens thresholds that translate raw 7d
// counts into the qualitative cognitive_load_hint label. Both fields are
// in units of (active_continue_threads + pending_review_count).
type LensModeWeights struct {
	// MediumThreshold is the inclusive lower bound at which the hint flips
	// from "light" to "medium". Below this and the macro state still reads
	// as "light" — the user has unfinished context but not so much that it
	// changes their pace.
	MediumThreshold uint32

	// HeavyThreshold is the inclusive lower bound at which the hint flips
	// from "medium" to "heavy". This is the signal the UI uses to widen
	// review-bucket affordances (e.g. promote "I got this" CTAs) so the
	// user can clear the backlog deliberately.
	HeavyThreshold uint32
}

// LookupLensModeWeights returns the immutable weights for a lens. Unknown
// lenses fall back to the default cohort — the projector must never panic
// because of a stale lens id in event payload.
func LookupLensModeWeights(id LensModeID) LensModeWeights {
	if w, ok := lensModeWeightsTable[id]; ok {
		return w
	}
	return lensModeWeightsTable[DefaultLensModeID]
}

// lensModeWeightsTable holds the canonical compile-time mapping. Treat it
// as immutable: a runtime mutation here would bypass the reproject ladder.
var lensModeWeightsTable = map[LensModeID]LensModeWeights{
	DefaultLensModeID: {
		MediumThreshold: 3,
		HeavyThreshold:  7,
	},
}
