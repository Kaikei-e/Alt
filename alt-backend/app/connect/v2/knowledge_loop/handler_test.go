package knowledge_loop

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"testing"
	"time"

	"connectrpc.com/connect"

	"alt/domain"
	loopv1 "alt/gen/proto/alt/knowledge/loop/v1"
	"alt/port/knowledge_loop_port"
	"alt/usecase/knowledge_loop_usecase"

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

// ---------------------------------------------------------------------------
// TransitionKnowledgeLoop error-classification tests
//
// Ensures the handler maps usecase error sentinels to distinct Connect-RPC codes
// (per connectrpc.com/docs/protocol#error-codes) so the SvelteKit BFF can map
// each to a meaningful HTTP status rather than collapsing everything into 502.
// ---------------------------------------------------------------------------

type fakeDedupePort struct {
	err error
}

func (f *fakeDedupePort) ReserveTransitionIdempotency(
	_ context.Context,
	_ uuid.UUID,
	_ string,
) (bool, *knowledge_loop_port.CachedTransitionResponse, error) {
	if f.err != nil {
		return false, nil, f.err
	}
	return true, nil, nil
}

func newTransitionHandlerWithDedupeErr(t *testing.T, dedupeErr error) *Handler {
	t.Helper()
	// nowFunc uses real time so UUIDv7 embedded timestamps pass ValidateClientTransitionID.
	// appendPort is nil: these tests exercise the reserve-failure path, so event append
	// never runs. Tests that verify append behavior live in the usecase package.
	// eventsForUserPort is nil: these tests exercise the transition RPC, not the stream RPC.
	uc := knowledge_loop_usecase.NewTransitionKnowledgeLoopUsecase(&fakeDedupePort{err: dedupeErr}, nil, nil, time.Now)
	return NewHandler(nil, uc, nil, slog.Default())
}

func authedContextForHandlerTests(t *testing.T) context.Context {
	t.Helper()
	user := &domain.UserContext{
		UserID:    uuid.New(),
		TenantID:  uuid.New(),
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		SessionID: "sess-test",
		LoginAt:   time.Now().Add(-time.Minute),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	return domain.SetUserContext(context.Background(), user)
}

// validTransitionRequest builds a request that passes validators; the dedupe port
// decides whether Execute succeeds or fails.
func validTransitionRequest(t *testing.T) *connect.Request[loopv1.TransitionKnowledgeLoopRequest] {
	t.Helper()
	id, err := uuid.NewV7()
	require.NoError(t, err)
	return connect.NewRequest(&loopv1.TransitionKnowledgeLoopRequest{
		LensModeId:                 "default",
		ClientTransitionId:         id.String(),
		EntryKey:                   "article:42",
		FromStage:                  loopv1.LoopStage_LOOP_STAGE_OBSERVE,
		ToStage:                    loopv1.LoopStage_LOOP_STAGE_ORIENT,
		Trigger:                    loopv1.TransitionTrigger_TRANSITION_TRIGGER_USER_TAP,
		ObservedProjectionRevision: 1,
	})
}

func TestTransitionKnowledgeLoop_ReturnsUnavailable_OnUpstreamUnavailable(t *testing.T) {
	h := newTransitionHandlerWithDedupeErr(
		t,
		// simulated driver failure: sovereign dial fails
		&net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")},
	)
	_, err := h.TransitionKnowledgeLoop(authedContextForHandlerTests(t), validTransitionRequest(t))
	require.Error(t, err)
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	require.Equal(t, connect.CodeUnavailable, ce.Code(), "net.OpError must map to CodeUnavailable")
}

func TestTransitionKnowledgeLoop_ReturnsDeadlineExceeded_OnContextDeadline(t *testing.T) {
	h := newTransitionHandlerWithDedupeErr(t, context.DeadlineExceeded)
	_, err := h.TransitionKnowledgeLoop(authedContextForHandlerTests(t), validTransitionRequest(t))
	require.Error(t, err)
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	require.Equal(t, connect.CodeDeadlineExceeded, ce.Code())
}

func TestTransitionKnowledgeLoop_ReturnsInternal_OnOpaqueError(t *testing.T) {
	h := newTransitionHandlerWithDedupeErr(t, errors.New("opaque boom"))
	_, err := h.TransitionKnowledgeLoop(authedContextForHandlerTests(t), validTransitionRequest(t))
	require.Error(t, err)
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	require.Equal(t, connect.CodeInternal, ce.Code())
}

func TestTransitionKnowledgeLoop_ReturnsInvalidArgument_OnValidator(t *testing.T) {
	// No dedupe error; fail at validation instead.
	h := newTransitionHandlerWithDedupeErr(t, nil)
	req := validTransitionRequest(t)
	req.Msg.EntryKey = "has space" // keyFormat regexp rejects
	_, err := h.TransitionKnowledgeLoop(authedContextForHandlerTests(t), req)
	require.Error(t, err)
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	require.Equal(t, connect.CodeInvalidArgument, ce.Code())
}

func TestTransitionKnowledgeLoop_ReturnsUnauthenticated_OnMissingUser(t *testing.T) {
	h := newTransitionHandlerWithDedupeErr(t, nil)
	_, err := h.TransitionKnowledgeLoop(context.Background(), validTransitionRequest(t))
	require.Error(t, err)
	var ce *connect.Error
	require.ErrorAs(t, err, &ce)
	require.Equal(t, connect.CodeUnauthenticated, ce.Code())
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
