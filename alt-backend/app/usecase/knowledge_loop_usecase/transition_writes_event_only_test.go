package knowledge_loop_usecase

import (
	"strings"
	"testing"

	"alt/domain"

	"github.com/stretchr/testify/require"
)

// Single-emission rule guard (canonical contract §3 invariant 7):
//
//	/loop UI MUST emit knowledge_loop.* events.
//	/feeds UI MUST emit HomeItem*  events.
//	No single user intent emits both.
//
// ClassifyTransitionEvent is the only place the /loop transition usecase
// derives an event type from (fromStage, toStage, trigger). If a future
// refactor mistakenly wires it to return a HomeItem* type, /loop interactions
// would silently feed back into the /feeds projection lane and double-count
// against the same user intent. This test enumerates every valid transition
// + the deferred path and asserts each one returns a KnowledgeLoop* type.
//
// The list of stage / trigger string literals mirrors the proto enum names the
// production caller passes in; if the proto changes, update this test.

func TestClassifyTransitionEvent_OnlyEmitsKnowledgeLoopEvents(t *testing.T) {
	t.Parallel()

	stages := []string{
		"LOOP_STAGE_OBSERVE",
		"LOOP_STAGE_ORIENT",
		"LOOP_STAGE_DECIDE",
		"LOOP_STAGE_ACT",
	}
	triggers := []string{
		"TRANSITION_TRIGGER_DWELL",
		"TRANSITION_TRIGGER_USER_TAP",
		"TRANSITION_TRIGGER_KEYBOARD",
		"TRANSITION_TRIGGER_PROGRAMMATIC",
		"TRANSITION_TRIGGER_DEFER",
	}

	emittedTypes := map[string]struct{}{}

	for _, from := range stages {
		for _, to := range stages {
			for _, trig := range triggers {
				ev, err := ClassifyTransitionEvent(from, to, trig)
				if err != nil {
					// Forbidden transition — that's fine; the function correctly
					// rejects it. The invariant only applies when an event would
					// actually be emitted.
					continue
				}
				require.NotEmpty(t, ev, "ClassifyTransitionEvent returned empty event type without error for %s->%s/%s", from, to, trig)
				emittedTypes[ev] = struct{}{}

				require.Truef(t, strings.HasPrefix(ev, "knowledge_loop."),
					"ClassifyTransitionEvent(%s->%s/%s) returned %q which is NOT a knowledge_loop.* event — single-emission rule violated",
					from, to, trig, ev)
				require.Falsef(t, strings.HasPrefix(ev, "HomeItem"),
					"ClassifyTransitionEvent(%s->%s/%s) returned a HomeItem* event %q — /loop must never emit /feeds events",
					from, to, trig, ev)
			}
		}
	}

	// The matrix must actually exercise the canonical Loop event types so
	// this guard isn't vacuously satisfied by a pruned switch statement.
	expected := []string{
		domain.EventKnowledgeLoopObserved,
		domain.EventKnowledgeLoopOriented,
		domain.EventKnowledgeLoopDecisionPresented,
		domain.EventKnowledgeLoopActed,
		domain.EventKnowledgeLoopReturned,
		domain.EventKnowledgeLoopDeferred,
	}
	for _, want := range expected {
		_, ok := emittedTypes[want]
		require.Truef(t, ok, "no transition exercises %q — the test's stage/trigger matrix may be missing a case", want)
	}
}

// TestClassifyTransitionEvent_NeverEmitsHomeItemForArbitraryTrigger guards
// against future regressions where someone wires a /feeds-style trigger label
// into the /loop classifier. ClassifyTransitionEvent disambiguates within
// allowed stage transitions using the trigger; the disambiguation must never
// pick a HomeItem* type even for an unrecognised trigger. The single-emission
// rule (canonical contract §3 invariant 7) does not depend on trigger
// validation — it depends on the classifier never spelling HomeItem*.
func TestClassifyTransitionEvent_NeverEmitsHomeItemForArbitraryTrigger(t *testing.T) {
	t.Parallel()

	smuggled := []string{
		"HOME_ITEM_OPENED",
		"HOME_ITEM_DISMISSED",
		"HOME_ITEM_ASKED",
		"open",
		"dismiss",
		"asked",
		"",
	}
	stages := []string{
		"LOOP_STAGE_OBSERVE",
		"LOOP_STAGE_ORIENT",
		"LOOP_STAGE_DECIDE",
		"LOOP_STAGE_ACT",
	}
	for _, from := range stages {
		for _, to := range stages {
			for _, trig := range smuggled {
				ev, err := ClassifyTransitionEvent(from, to, trig)
				if err != nil {
					continue
				}
				require.Falsef(t, strings.HasPrefix(ev, "HomeItem"),
					"ClassifyTransitionEvent(%s->%s/%q) returned a HomeItem* event %q — /loop must never emit /feeds events even for unknown triggers",
					from, to, trig, ev)
			}
		}
	}
}
