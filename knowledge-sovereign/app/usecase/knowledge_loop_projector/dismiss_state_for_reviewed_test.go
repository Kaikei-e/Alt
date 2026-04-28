package knowledge_loop_projector

import (
	"encoding/json"
	"testing"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// dismissStateForReviewedEvent maps the trigger sub-field on a
// KnowledgeLoopReviewed event payload to the dismiss_state the projector
// applies to the entry. Pure: replay must produce the same dismiss_state for
// the same payload regardless of when it runs.

func TestDismissStateForReviewedEvent_RecheckReArmsEntry(t *testing.T) {
	t.Parallel()
	payload, _ := json.Marshal(map[string]any{"trigger": "TRANSITION_TRIGGER_RECHECK"})
	got := dismissStateForReviewedEvent(payload)
	if got != sovereignv1.DismissState_DISMISS_STATE_ACTIVE {
		t.Errorf("recheck → %v; want ACTIVE", got)
	}
}

func TestDismissStateForReviewedEvent_ArchiveCompletes(t *testing.T) {
	t.Parallel()
	payload, _ := json.Marshal(map[string]any{"trigger": "TRANSITION_TRIGGER_ARCHIVE"})
	got := dismissStateForReviewedEvent(payload)
	if got != sovereignv1.DismissState_DISMISS_STATE_COMPLETED {
		t.Errorf("archive → %v; want COMPLETED", got)
	}
}

func TestDismissStateForReviewedEvent_MarkReviewedCompletes(t *testing.T) {
	t.Parallel()
	payload, _ := json.Marshal(map[string]any{"trigger": "TRANSITION_TRIGGER_MARK_REVIEWED"})
	got := dismissStateForReviewedEvent(payload)
	if got != sovereignv1.DismissState_DISMISS_STATE_COMPLETED {
		t.Errorf("mark_reviewed → %v; want COMPLETED", got)
	}
}

func TestDismissStateForReviewedEvent_UnknownTriggerFailsClosed(t *testing.T) {
	t.Parallel()
	payload, _ := json.Marshal(map[string]any{"trigger": "TRANSITION_TRIGGER_MAKE_COFFEE"})
	got := dismissStateForReviewedEvent(payload)
	if got != sovereignv1.DismissState_DISMISS_STATE_COMPLETED {
		t.Errorf("unknown trigger → %v; want COMPLETED (fail-closed default)", got)
	}
}

func TestDismissStateForReviewedEvent_ActionWithoutTriggerFailsClosed(t *testing.T) {
	t.Parallel()
	payload, _ := json.Marshal(map[string]any{"action": "recheck"})
	got := dismissStateForReviewedEvent(payload)
	if got != sovereignv1.DismissState_DISMISS_STATE_COMPLETED {
		t.Errorf("legacy action-only payload → %v; want COMPLETED (trigger is the contract)", got)
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
