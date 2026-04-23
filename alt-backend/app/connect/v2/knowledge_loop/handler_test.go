package knowledge_loop

import (
	"encoding/json"
	"testing"
	"time"

	"alt/domain"
	loopv1 "alt/gen/proto/alt/knowledge/loop/v1"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestToProtoEntry_MapsChangeSummary verifies that the JSONB change_summary blob
// stored on domain.KnowledgeLoopEntry is unmarshaled into the proto ChangeSummary
// message so /loop UI can render the "what changed" band (PR-L1).
func TestToProtoEntry_MapsChangeSummary(t *testing.T) {
	prev := "article:old"
	payload, err := json.Marshal(map[string]any{
		"summary":            "Title tightened",
		"changed_fields":     []string{"title", "summary"},
		"previous_entry_key": prev,
	})
	require.NoError(t, err)

	in := baseEntry()
	in.ChangeSummary = payload

	pb := toProtoEntry(in)
	require.NotNil(t, pb.ChangeSummary)
	require.Equal(t, "Title tightened", pb.ChangeSummary.Summary)
	require.Equal(t, []string{"title", "summary"}, pb.ChangeSummary.ChangedFields)
	require.NotNil(t, pb.ChangeSummary.PreviousEntryKey)
	require.Equal(t, "article:old", *pb.ChangeSummary.PreviousEntryKey)
}

// TestToProtoEntry_MapsContinueContext checks the continue_context blob is
// carried over including last_interacted_at timestamp.
func TestToProtoEntry_MapsContinueContext(t *testing.T) {
	interactedAt := time.Date(2026, 4, 20, 9, 15, 0, 0, time.UTC)
	payload, err := json.Marshal(map[string]any{
		"summary":              "Read 3m ago",
		"recent_action_labels": []string{"scroll_30pct", "open_source"},
		"last_interacted_at":   interactedAt.Format(time.RFC3339),
	})
	require.NoError(t, err)

	in := baseEntry()
	in.ContinueContext = payload

	pb := toProtoEntry(in)
	require.NotNil(t, pb.ContinueContext)
	require.Equal(t, "Read 3m ago", pb.ContinueContext.Summary)
	require.Equal(t, []string{"scroll_30pct", "open_source"}, pb.ContinueContext.RecentActionLabels)
	require.NotNil(t, pb.ContinueContext.LastInteractedAt)
	require.True(t, pb.ContinueContext.LastInteractedAt.AsTime().Equal(interactedAt))
}

// TestToProtoEntry_MapsDecisionOptions verifies the projector-seeded
// decision_options blob becomes repeated DecisionOption protos with intent enums.
func TestToProtoEntry_MapsDecisionOptions(t *testing.T) {
	label := "Open source"
	payload, err := json.Marshal([]map[string]any{
		{"action_id": "open", "intent": "open", "label": label},
		{"action_id": "ask", "intent": "ask"},
		{"action_id": "save", "intent": "save"},
		{"action_id": "dismiss", "intent": "snooze"},
	})
	require.NoError(t, err)

	in := baseEntry()
	in.DecisionOptions = payload

	pb := toProtoEntry(in)
	require.Len(t, pb.DecisionOptions, 4)
	require.Equal(t, "open", pb.DecisionOptions[0].ActionId)
	require.Equal(t, loopv1.DecisionIntent_DECISION_INTENT_OPEN, pb.DecisionOptions[0].Intent)
	require.NotNil(t, pb.DecisionOptions[0].Label)
	require.Equal(t, "Open source", *pb.DecisionOptions[0].Label)

	require.Equal(t, loopv1.DecisionIntent_DECISION_INTENT_ASK, pb.DecisionOptions[1].Intent)
	require.Nil(t, pb.DecisionOptions[1].Label, "unspecified label stays nil, not empty string")
	require.Equal(t, loopv1.DecisionIntent_DECISION_INTENT_SAVE, pb.DecisionOptions[2].Intent)
	require.Equal(t, loopv1.DecisionIntent_DECISION_INTENT_SNOOZE, pb.DecisionOptions[3].Intent)
}

// TestToProtoEntry_MapsActTargets checks act_targets[] mapping with enum + route oneof.
func TestToProtoEntry_MapsActTargets(t *testing.T) {
	route := "/feeds/article:42"
	payload, err := json.Marshal([]map[string]any{
		{"target_type": "article", "target_ref": "article:42", "route": route},
		{"target_type": "ask", "target_ref": "entry:42"},
	})
	require.NoError(t, err)

	in := baseEntry()
	in.ActTargets = payload

	pb := toProtoEntry(in)
	require.Len(t, pb.ActTargets, 2)
	require.Equal(t, loopv1.ActTargetType_ACT_TARGET_TYPE_ARTICLE, pb.ActTargets[0].TargetType)
	require.Equal(t, "article:42", pb.ActTargets[0].TargetRef)
	require.NotNil(t, pb.ActTargets[0].Route)
	require.Equal(t, "/feeds/article:42", *pb.ActTargets[0].Route)

	require.Equal(t, loopv1.ActTargetType_ACT_TARGET_TYPE_ASK, pb.ActTargets[1].TargetType)
	require.Nil(t, pb.ActTargets[1].Route)
}

// TestToProtoEntry_EmptyBlobsStayNil verifies the 4 fields default to nil/empty
// when their JSONB blobs are empty or missing, matching the proto oneof / repeated
// defaults.
func TestToProtoEntry_EmptyBlobsStayNil(t *testing.T) {
	in := baseEntry()
	// all 4 fields explicitly empty

	pb := toProtoEntry(in)
	require.Nil(t, pb.ChangeSummary)
	require.Nil(t, pb.ContinueContext)
	require.Empty(t, pb.DecisionOptions)
	require.Empty(t, pb.ActTargets)
}

// TestToProtoEntry_MalformedBlobIsTolerated documents the defensive fallback:
// a malformed JSONB blob MUST NOT panic the handler or leak an error to the
// client. It just falls back to nil / empty for that field.
func TestToProtoEntry_MalformedBlobIsTolerated(t *testing.T) {
	in := baseEntry()
	in.ChangeSummary = []byte(`{"summary": not valid json`)
	in.DecisionOptions = []byte(`not-an-array`)

	pb := toProtoEntry(in)
	require.Nil(t, pb.ChangeSummary)
	require.Empty(t, pb.DecisionOptions)
}

// baseEntry returns a minimal valid domain entry used by toProtoEntry tests.
func baseEntry() *domain.KnowledgeLoopEntry {
	return &domain.KnowledgeLoopEntry{
		UserID:               uuid.New(),
		TenantID:             uuid.New(),
		LensModeID:           "default",
		EntryKey:             "article:42",
		SourceItemKey:        "article:42",
		ProposedStage:        domain.LoopStageObserve,
		SurfaceBucket:        domain.SurfaceNow,
		ProjectionRevision:   1,
		ProjectionSeqHiwater: 100,
		SourceEventSeq:       100,
		FreshnessAt:          time.Date(2026, 4, 23, 10, 0, 0, 0, time.UTC),
		WhyKind:              domain.WhyKindSource,
		WhyText:              "New summary",
		DismissState:         domain.DismissActive,
		RenderDepthHint:      domain.RenderDepthFlat,
		LoopPriority:         domain.LoopPriorityReference,
	}
}
