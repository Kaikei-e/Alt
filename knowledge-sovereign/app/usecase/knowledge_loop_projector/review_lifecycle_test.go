package knowledge_loop_projector

import (
	"encoding/json"
	"testing"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

func TestReviewLifecycleForReviewedEvent_TableDriven(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		payload map[string]any
		want    ReviewLifecycle
	}{
		{
			name:    "recheck re-arms ACTIVE/VISIBLE/OPEN",
			payload: map[string]any{"trigger": "TRANSITION_TRIGGER_RECHECK"},
			want: ReviewLifecycle{
				DismissState:    sovereignv1.DismissState_DISMISS_STATE_ACTIVE,
				VisibilityState: sovereignv1.LoopVisibilityState_LOOP_VISIBILITY_STATE_VISIBLE,
				CompletionState: sovereignv1.LoopCompletionState_LOOP_COMPLETION_STATE_OPEN,
			},
		},
		{
			name:    "archive hides COMPLETED/HIDDEN/COMPLETED",
			payload: map[string]any{"trigger": "TRANSITION_TRIGGER_ARCHIVE"},
			want: ReviewLifecycle{
				DismissState:    sovereignv1.DismissState_DISMISS_STATE_COMPLETED,
				VisibilityState: sovereignv1.LoopVisibilityState_LOOP_VISIBILITY_STATE_HIDDEN,
				CompletionState: sovereignv1.LoopCompletionState_LOOP_COMPLETION_STATE_COMPLETED,
			},
		},
		{
			name:    "mark_reviewed keeps visible in Review COMPLETED/VISIBLE/COMPLETED",
			payload: map[string]any{"trigger": "TRANSITION_TRIGGER_MARK_REVIEWED"},
			want: ReviewLifecycle{
				DismissState:    sovereignv1.DismissState_DISMISS_STATE_COMPLETED,
				VisibilityState: sovereignv1.LoopVisibilityState_LOOP_VISIBILITY_STATE_VISIBLE,
				CompletionState: sovereignv1.LoopCompletionState_LOOP_COMPLETION_STATE_COMPLETED,
			},
		},
		{
			name:    "unknown trigger fails closed COMPLETED/VISIBLE/COMPLETED",
			payload: map[string]any{"trigger": "TRANSITION_TRIGGER_MAKE_COFFEE"},
			want: ReviewLifecycle{
				DismissState:    sovereignv1.DismissState_DISMISS_STATE_COMPLETED,
				VisibilityState: sovereignv1.LoopVisibilityState_LOOP_VISIBILITY_STATE_VISIBLE,
				CompletionState: sovereignv1.LoopCompletionState_LOOP_COMPLETION_STATE_COMPLETED,
			},
		},
		{
			name:    "legacy action-only payload fails closed",
			payload: map[string]any{"action": "recheck"},
			want: ReviewLifecycle{
				DismissState:    sovereignv1.DismissState_DISMISS_STATE_COMPLETED,
				VisibilityState: sovereignv1.LoopVisibilityState_LOOP_VISIBILITY_STATE_VISIBLE,
				CompletionState: sovereignv1.LoopCompletionState_LOOP_COMPLETION_STATE_COMPLETED,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			payload, _ := json.Marshal(tc.payload)
			got := reviewLifecycleForReviewedEvent(payload)
			if got != tc.want {
				t.Errorf("reviewLifecycleForReviewedEvent(%s) = %+v; want %+v", string(payload), got, tc.want)
			}
		})
	}
}

func TestReviewLifecycleForReviewedEvent_EmptyPayload(t *testing.T) {
	t.Parallel()
	got := reviewLifecycleForReviewedEvent(nil)
	want := ReviewLifecycle{
		DismissState:    sovereignv1.DismissState_DISMISS_STATE_COMPLETED,
		VisibilityState: sovereignv1.LoopVisibilityState_LOOP_VISIBILITY_STATE_VISIBLE,
		CompletionState: sovereignv1.LoopCompletionState_LOOP_COMPLETION_STATE_COMPLETED,
	}
	if got != want {
		t.Errorf("nil payload = %+v; want %+v", got, want)
	}
}

func TestReviewLifecycleForReviewedEvent_MalformedJSON(t *testing.T) {
	t.Parallel()
	got := reviewLifecycleForReviewedEvent([]byte("{not json"))
	want := ReviewLifecycle{
		DismissState:    sovereignv1.DismissState_DISMISS_STATE_COMPLETED,
		VisibilityState: sovereignv1.LoopVisibilityState_LOOP_VISIBILITY_STATE_VISIBLE,
		CompletionState: sovereignv1.LoopCompletionState_LOOP_COMPLETION_STATE_COMPLETED,
	}
	if got != want {
		t.Errorf("malformed JSON = %+v; want %+v", got, want)
	}
}

// TestReviewLifecycleForReviewedEvent_Determinism asserts the function is pure
// across repeated invocations with the same payload — the reproject-safety
// invariant for review-lane events.
func TestReviewLifecycleForReviewedEvent_Determinism(t *testing.T) {
	t.Parallel()
	payload, _ := json.Marshal(map[string]any{"trigger": "TRANSITION_TRIGGER_MARK_REVIEWED"})
	first := reviewLifecycleForReviewedEvent(payload)
	for i := range 5 {
		got := reviewLifecycleForReviewedEvent(payload)
		if got != first {
			t.Fatalf("invocation %d diverged: %+v vs %+v", i, got, first)
		}
	}
}
