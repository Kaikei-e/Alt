package knowledge_loop_projector

import (
	"encoding/json"
	"testing"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// dismissStateForReviewedEvent maps the action sub-field on a
// KnowledgeLoopReviewed event payload to the dismiss_state the projector
// applies to the entry. Pure: replay must produce the same dismiss_state for
// the same payload regardless of when it runs.

func TestDismissStateForReviewedEvent_RecheckReArmsEntry(t *testing.T) {
	t.Parallel()
	payload, _ := json.Marshal(map[string]any{"action": "recheck"})
	got := dismissStateForReviewedEvent(payload)
	if got != sovereignv1.DismissState_DISMISS_STATE_ACTIVE {
		t.Errorf("recheck → %v; want ACTIVE", got)
	}
}

func TestDismissStateForReviewedEvent_ArchiveCompletes(t *testing.T) {
	t.Parallel()
	payload, _ := json.Marshal(map[string]any{"action": "archive"})
	got := dismissStateForReviewedEvent(payload)
	if got != sovereignv1.DismissState_DISMISS_STATE_COMPLETED {
		t.Errorf("archive → %v; want COMPLETED", got)
	}
}

func TestDismissStateForReviewedEvent_MarkReviewedCompletes(t *testing.T) {
	t.Parallel()
	payload, _ := json.Marshal(map[string]any{"action": "mark_reviewed"})
	got := dismissStateForReviewedEvent(payload)
	if got != sovereignv1.DismissState_DISMISS_STATE_COMPLETED {
		t.Errorf("mark_reviewed → %v; want COMPLETED", got)
	}
}

func TestDismissStateForReviewedEvent_UnknownActionFailsClosed(t *testing.T) {
	t.Parallel()
	payload, _ := json.Marshal(map[string]any{"action": "make_coffee"})
	got := dismissStateForReviewedEvent(payload)
	if got != sovereignv1.DismissState_DISMISS_STATE_COMPLETED {
		t.Errorf("unknown action → %v; want COMPLETED (fail-closed default)", got)
	}
}

func TestDismissStateForReviewedEvent_EmptyPayloadFailsClosed(t *testing.T) {
	t.Parallel()
	got := dismissStateForReviewedEvent(nil)
	if got != sovereignv1.DismissState_DISMISS_STATE_COMPLETED {
		t.Errorf("nil payload → %v; want COMPLETED", got)
	}
}

func TestDismissStateForReviewedEvent_MalformedJsonFailsClosed(t *testing.T) {
	t.Parallel()
	got := dismissStateForReviewedEvent([]byte("{not json"))
	if got != sovereignv1.DismissState_DISMISS_STATE_COMPLETED {
		t.Errorf("malformed JSON → %v; want COMPLETED", got)
	}
}
