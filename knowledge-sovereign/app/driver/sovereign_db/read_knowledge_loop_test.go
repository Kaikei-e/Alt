package sovereign_db

import (
	"testing"

	"github.com/stretchr/testify/require"

	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

func TestWhyKindFromDB_SurfacePlannerNarrativeKinds(t *testing.T) {
	cases := []struct {
		db   string
		want sovereignv1.WhyKind
	}{
		{"topic_affinity_why", sovereignv1.WhyKind_WHY_KIND_TOPIC_AFFINITY},
		{"tag_trending_why", sovereignv1.WhyKind_WHY_KIND_TAG_TRENDING},
		{"unfinished_continue_why", sovereignv1.WhyKind_WHY_KIND_UNFINISHED_CONTINUE},
	}

	for _, tc := range cases {
		t.Run(tc.db, func(t *testing.T) {
			require.Equal(t, tc.want, whyKindFromDB(tc.db))
		})
	}
}

func TestLifecycleStateFromDB(t *testing.T) {
	require.Equal(t,
		sovereignv1.LoopVisibilityState_LOOP_VISIBILITY_STATE_VISIBLE,
		visibilityStateFromDB("visible"))
	require.Equal(t,
		sovereignv1.LoopVisibilityState_LOOP_VISIBILITY_STATE_SNOOZED,
		visibilityStateFromDB("snoozed"))
	require.Equal(t,
		sovereignv1.LoopCompletionState_LOOP_COMPLETION_STATE_COMPLETED,
		completionStateFromDB("completed"))
	require.Equal(t,
		sovereignv1.LoopCompletionState_LOOP_COMPLETION_STATE_DISMISSED,
		completionStateFromDB("dismissed"))
}
