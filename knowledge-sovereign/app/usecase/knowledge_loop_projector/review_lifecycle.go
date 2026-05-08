package knowledge_loop_projector

import (
	"encoding/json"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// ReviewLifecycle is the coordinated patch outcome for a
// knowledge_loop.reviewed.v1 event. The three triggers that share this event
// type — recheck, archive, mark_reviewed — must produce different lifecycle
// outcomes even when two of them share the same dismiss_state. Bundling them
// here keeps the trigger semantics explicit and prevents archive (hide) and
// mark_reviewed (keep visible in Review) from collapsing through a flat
// dismiss → visibility lookup.
//
//	recheck       → ACTIVE  + VISIBLE + OPEN       (entry re-armed, eligible
//	                                                for v2 bucket recompute)
//	archive       → COMPLETED + HIDDEN  + COMPLETED (entry leaves the visible
//	                                                read path entirely)
//	mark_reviewed → COMPLETED + VISIBLE + COMPLETED (entry stays in Review with
//	                                                review_reason=reviewed so
//	                                                the user sees it was
//	                                                acknowledged but not lost)
type ReviewLifecycle struct {
	DismissState    sovereignv1.DismissState
	VisibilityState sovereignv1.LoopVisibilityState
	CompletionState sovereignv1.LoopCompletionState
}

// reviewLifecycleForReviewedEvent maps the trigger sub-field on a
// KnowledgeLoopReviewed event payload to the coordinated lifecycle update.
// Pure: same payload always yields the same lifecycle, so reproject
// reproduces row state byte-identically.
func reviewLifecycleForReviewedEvent(payload json.RawMessage) ReviewLifecycle {
	switch triggerFromReviewedPayload(payload) {
	case "TRANSITION_TRIGGER_RECHECK":
		return ReviewLifecycle{
			DismissState:    sovereignv1.DismissState_DISMISS_STATE_ACTIVE,
			VisibilityState: sovereignv1.LoopVisibilityState_LOOP_VISIBILITY_STATE_VISIBLE,
			CompletionState: sovereignv1.LoopCompletionState_LOOP_COMPLETION_STATE_OPEN,
		}
	case "TRANSITION_TRIGGER_ARCHIVE":
		return ReviewLifecycle{
			DismissState:    sovereignv1.DismissState_DISMISS_STATE_COMPLETED,
			VisibilityState: sovereignv1.LoopVisibilityState_LOOP_VISIBILITY_STATE_HIDDEN,
			CompletionState: sovereignv1.LoopCompletionState_LOOP_COMPLETION_STATE_COMPLETED,
		}
	case "TRANSITION_TRIGGER_MARK_REVIEWED":
		return ReviewLifecycle{
			DismissState:    sovereignv1.DismissState_DISMISS_STATE_COMPLETED,
			VisibilityState: sovereignv1.LoopVisibilityState_LOOP_VISIBILITY_STATE_VISIBLE,
			CompletionState: sovereignv1.LoopCompletionState_LOOP_COMPLETION_STATE_COMPLETED,
		}
	default:
		// Fail-closed but non-destructive: COMPLETED so the entry doesn't
		// linger as ACTIVE on a malformed payload, but VISIBLE so the user
		// can still see and act on it (archive is the destructive option and
		// must be opted into explicitly).
		return ReviewLifecycle{
			DismissState:    sovereignv1.DismissState_DISMISS_STATE_COMPLETED,
			VisibilityState: sovereignv1.LoopVisibilityState_LOOP_VISIBILITY_STATE_VISIBLE,
			CompletionState: sovereignv1.LoopCompletionState_LOOP_COMPLETION_STATE_COMPLETED,
		}
	}
}

func triggerFromReviewedPayload(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		return ""
	}
	t, _ := m["trigger"].(string)
	return t
}

