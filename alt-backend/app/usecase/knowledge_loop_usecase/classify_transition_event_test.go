package knowledge_loop_usecase

import (
	"alt/domain"
	"testing"

	"github.com/stretchr/testify/require"
)

// Table-driven tests for ClassifyTransitionEvent. Input is the proto-enum-name
// tuple (from_stage, to_stage, trigger); output is the canonical event_type string.
// The matrix is exhaustive over the allowed transitions in ADR-000831 §7 / §8.

func TestClassifyTransitionEvent(t *testing.T) {
	cases := []struct {
		name      string
		from      string
		to        string
		trigger   string
		wantType  string
		wantErr   bool
		wantActed bool // if true: acted event with continue_flag semantics
	}{
		{
			// Auto-OODA suppression: dwell is rejected at the classifier (and
			// caught one layer earlier by ValidateDwellTriggerTarget). Passive
			// viewing must not advance OODA stage; user_tap is the only legal
			// path to Orient.
			name:    "observe->orient via dwell => rejected (Auto-OODA suppression)",
			from:    "LOOP_STAGE_OBSERVE",
			to:      "LOOP_STAGE_ORIENT",
			trigger: "TRANSITION_TRIGGER_DWELL",
			wantErr: true,
		},
		{
			name:    "observe->observe via dwell => rejected (no passive ack)",
			from:    "LOOP_STAGE_OBSERVE",
			to:      "LOOP_STAGE_OBSERVE",
			trigger: "TRANSITION_TRIGGER_DWELL",
			wantErr: true,
		},
		{
			name:     "observe->orient via user_tap => Oriented",
			from:     "LOOP_STAGE_OBSERVE",
			to:       "LOOP_STAGE_ORIENT",
			trigger:  "TRANSITION_TRIGGER_USER_TAP",
			wantType: domain.EventKnowledgeLoopOriented,
		},
		{
			name:     "observe->orient via keyboard => Oriented",
			from:     "LOOP_STAGE_OBSERVE",
			to:       "LOOP_STAGE_ORIENT",
			trigger:  "TRANSITION_TRIGGER_KEYBOARD",
			wantType: domain.EventKnowledgeLoopOriented,
		},
		{
			name:     "observe->decide (bypass orient) => Oriented (coarse)",
			from:     "LOOP_STAGE_OBSERVE",
			to:       "LOOP_STAGE_DECIDE",
			trigger:  "TRANSITION_TRIGGER_USER_TAP",
			wantType: domain.EventKnowledgeLoopOriented,
		},
		{
			name:     "orient->decide => DecisionPresented",
			from:     "LOOP_STAGE_ORIENT",
			to:       "LOOP_STAGE_DECIDE",
			trigger:  "TRANSITION_TRIGGER_USER_TAP",
			wantType: domain.EventKnowledgeLoopDecisionPresented,
		},
		{
			name:     "decide->act => Acted",
			from:     "LOOP_STAGE_DECIDE",
			to:       "LOOP_STAGE_ACT",
			trigger:  "TRANSITION_TRIGGER_USER_TAP",
			wantType: domain.EventKnowledgeLoopActed,
		},
		{
			name:     "act->observe => Returned",
			from:     "LOOP_STAGE_ACT",
			to:       "LOOP_STAGE_OBSERVE",
			trigger:  "TRANSITION_TRIGGER_USER_TAP",
			wantType: domain.EventKnowledgeLoopReturned,
		},
		// Invalid transitions must error per ADR-000831 §7 forbidden set
		{
			name:    "observe->act (forbidden) => error",
			from:    "LOOP_STAGE_OBSERVE",
			to:      "LOOP_STAGE_ACT",
			trigger: "TRANSITION_TRIGGER_USER_TAP",
			wantErr: true,
		},
		{
			name:    "decide->observe without explicit return => error",
			from:    "LOOP_STAGE_DECIDE",
			to:      "LOOP_STAGE_OBSERVE",
			trigger: "TRANSITION_TRIGGER_USER_TAP",
			wantErr: true,
		},
		{
			name:    "act->act => error",
			from:    "LOOP_STAGE_ACT",
			to:      "LOOP_STAGE_ACT",
			trigger: "TRANSITION_TRIGGER_USER_TAP",
			wantErr: true,
		},
		{
			name:    "unspecified stage => error",
			from:    "LOOP_STAGE_UNSPECIFIED",
			to:      "LOOP_STAGE_OBSERVE",
			trigger: "TRANSITION_TRIGGER_USER_TAP",
			wantErr: true,
		},
		// DEFER trigger: same-stage Deferred regardless of OODA stage
		// (canonical contract §8.2 passive dismiss / snooze).
		{
			name:     "observe->observe via DEFER => Deferred",
			from:     "LOOP_STAGE_OBSERVE",
			to:       "LOOP_STAGE_OBSERVE",
			trigger:  "TRANSITION_TRIGGER_DEFER",
			wantType: domain.EventKnowledgeLoopDeferred,
		},
		{
			name:     "orient->orient via DEFER => Deferred",
			from:     "LOOP_STAGE_ORIENT",
			to:       "LOOP_STAGE_ORIENT",
			trigger:  "TRANSITION_TRIGGER_DEFER",
			wantType: domain.EventKnowledgeLoopDeferred,
		},
		{
			name:     "decide->decide via DEFER => Deferred",
			from:     "LOOP_STAGE_DECIDE",
			to:       "LOOP_STAGE_DECIDE",
			trigger:  "TRANSITION_TRIGGER_DEFER",
			wantType: domain.EventKnowledgeLoopDeferred,
		},
		{
			name:     "act->act via DEFER => Deferred (overrides act->act forbid)",
			from:     "LOOP_STAGE_ACT",
			to:       "LOOP_STAGE_ACT",
			trigger:  "TRANSITION_TRIGGER_DEFER",
			wantType: domain.EventKnowledgeLoopDeferred,
		},
		{
			name:    "DEFER with non-equal stages is rejected",
			from:    "LOOP_STAGE_OBSERVE",
			to:      "LOOP_STAGE_ORIENT",
			trigger: "TRANSITION_TRIGGER_DEFER",
			wantErr: true,
		},
		{
			name:    "DEFER on UNSPECIFIED stage is rejected",
			from:    "LOOP_STAGE_UNSPECIFIED",
			to:      "LOOP_STAGE_UNSPECIFIED",
			trigger: "TRANSITION_TRIGGER_DEFER",
			wantErr: true,
		},
		{
			name:     "observe->observe via RECHECK => Reviewed",
			from:     "LOOP_STAGE_OBSERVE",
			to:       "LOOP_STAGE_OBSERVE",
			trigger:  "TRANSITION_TRIGGER_RECHECK",
			wantType: domain.EventKnowledgeLoopReviewed,
		},
		{
			name:     "orient->orient via ARCHIVE => Reviewed",
			from:     "LOOP_STAGE_ORIENT",
			to:       "LOOP_STAGE_ORIENT",
			trigger:  "TRANSITION_TRIGGER_ARCHIVE",
			wantType: domain.EventKnowledgeLoopReviewed,
		},
		{
			name:     "decide->decide via MARK_REVIEWED => Reviewed",
			from:     "LOOP_STAGE_DECIDE",
			to:       "LOOP_STAGE_DECIDE",
			trigger:  "TRANSITION_TRIGGER_MARK_REVIEWED",
			wantType: domain.EventKnowledgeLoopReviewed,
		},
		{
			name:    "RECHECK with non-equal stages is rejected",
			from:    "LOOP_STAGE_OBSERVE",
			to:      "LOOP_STAGE_ORIENT",
			trigger: "TRANSITION_TRIGGER_RECHECK",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ClassifyTransitionEvent(tc.from, tc.to, tc.trigger)
			if tc.wantErr {
				require.Error(t, err)
				require.ErrorIs(t, err, ErrInvalidArgument)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantType, got)
		})
	}
}
