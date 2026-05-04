package knowledge_loop_usecase

import (
	"alt/domain"
	"alt/port/knowledge_loop_port"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// --- fakes -----------------------------------------------------------------

type fakeDedupePort struct {
	reserved bool
	cached   *knowledge_loop_port.CachedTransitionResponse
	err      error
	calls    int
}

func (f *fakeDedupePort) ReserveTransitionIdempotency(
	_ context.Context,
	_ uuid.UUID,
	_ string,
) (bool, *knowledge_loop_port.CachedTransitionResponse, error) {
	f.calls++
	if f.err != nil {
		return false, nil, f.err
	}
	return f.reserved, f.cached, nil
}

type fakeAppendPort struct {
	err       error
	events    []domain.KnowledgeEvent
	callCount int
}

func (f *fakeAppendPort) AppendKnowledgeEvent(_ context.Context, ev domain.KnowledgeEvent) (int64, error) {
	f.callCount++
	if f.err != nil {
		return 0, f.err
	}
	f.events = append(f.events, ev)
	return int64(len(f.events)), nil
}

// mustMintUUIDv7 builds a UUIDv7 whose embedded Unix-milli timestamp equals
// `at`, so validator time-window checks pass under a controlled nowFunc.
func mustMintUUIDv7(t *testing.T, at time.Time) string {
	t.Helper()
	var raw [16]byte
	ms := at.UnixMilli()
	raw[0] = byte(ms >> 40)
	raw[1] = byte(ms >> 32)
	raw[2] = byte(ms >> 24)
	raw[3] = byte(ms >> 16)
	raw[4] = byte(ms >> 8)
	raw[5] = byte(ms)
	raw[6] = 0x70 // version 7 in the high nibble of byte 6
	raw[8] = 0x80 // variant bits
	id, err := uuid.FromBytes(raw[:])
	require.NoError(t, err)
	return id.String()
}

// --- helpers ---------------------------------------------------------------

func newTransitionInput(t *testing.T, from, to, trigger string) TransitionInput {
	t.Helper()
	id, err := uuid.NewV7()
	require.NoError(t, err)
	return TransitionInput{
		TenantID:             uuid.New(),
		UserID:               uuid.New(),
		LensModeID:           "default",
		ClientTransitionID:   id.String(),
		EntryKey:             "article:42",
		FromStage:            from,
		ToStage:              to,
		Trigger:              trigger,
		ObservedProjRevision: 1,
	}
}

// --- tests -----------------------------------------------------------------

func TestTransition_AppendsEventOnFreshReservation(t *testing.T) {
	dedupe := &fakeDedupePort{reserved: true}
	appendPort := &fakeAppendPort{}
	uc := NewTransitionKnowledgeLoopUsecase(dedupe, appendPort, nil, time.Now)

	in := newTransitionInput(t, "LOOP_STAGE_OBSERVE", "LOOP_STAGE_ORIENT", "TRANSITION_TRIGGER_USER_TAP")
	res, err := uc.Execute(context.Background(), in)

	require.NoError(t, err)
	require.True(t, res.Accepted)
	require.Equal(t, 1, appendPort.callCount, "append must be called exactly once on fresh reserve")

	require.Len(t, appendPort.events, 1)
	ev := appendPort.events[0]
	require.Equal(t, domain.EventKnowledgeLoopOriented, ev.EventType)
	require.Equal(t, domain.AggregateLoopSession, ev.AggregateType)
	require.Equal(t, in.EntryKey, ev.AggregateID)
	require.Equal(t, in.TenantID, ev.TenantID)
	require.NotNil(t, ev.UserID)
	require.Equal(t, in.UserID, *ev.UserID)
	require.Equal(t, domain.ActorUser, ev.ActorType)
	require.Equal(t, in.ClientTransitionID, ev.DedupeKey,
		"dedupe_key must equal client_transition_id to unify fast-path and slow-path idempotency")

	// Payload must be JSON and reproject-safe: carries every field needed to
	// compute projection deltas without reading latest projection state.
	var payload map[string]any
	require.NoError(t, json.Unmarshal(ev.Payload, &payload))
	require.Equal(t, in.EntryKey, payload["entry_key"])
	require.Equal(t, in.LensModeID, payload["lens_mode_id"])
	require.Equal(t, "LOOP_STAGE_OBSERVE", payload["from_stage"])
	require.Equal(t, "LOOP_STAGE_ORIENT", payload["to_stage"])
	require.Equal(t, "TRANSITION_TRIGGER_USER_TAP", payload["trigger"])
	require.Equal(t, float64(in.ObservedProjRevision), payload["observed_projection_revision"])
}

// Auto-OODA suppression (Knowledge Loop 体験回復プラン Pillar 1):
// dwell triggers are rejected at the validator. The frontend no longer
// sends dwell at all — passive viewing must not advance OODA stage.
// This test replaces the pre-fix "dwell → KnowledgeLoopObserved" path and
// pins the new contract end-to-end so a future regression cannot
// silently re-introduce passive stage advancement.
func TestTransition_DwellRejected_NoEventAppended(t *testing.T) {
	dedupe := &fakeDedupePort{reserved: true}
	appendPort := &fakeAppendPort{}
	uc := NewTransitionKnowledgeLoopUsecase(dedupe, appendPort, nil, time.Now)

	in := newTransitionInput(t, "LOOP_STAGE_OBSERVE", "LOOP_STAGE_ORIENT", "TRANSITION_TRIGGER_DWELL")
	_, err := uc.Execute(context.Background(), in)

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInvalidArgument),
		"dwell trigger must surface as invalid_argument so the BFF returns 400")
	require.Empty(t, appendPort.events,
		"rejected dwell must not append any event (no projection pollution)")
}

// User-tap on observe→orient is the explicit replacement for the old dwell
// path. Pins that the cross-stage transition still works under the new
// contract.
func TestTransition_UserTapObserveToOrient_EmitsOriented(t *testing.T) {
	dedupe := &fakeDedupePort{reserved: true}
	appendPort := &fakeAppendPort{}
	uc := NewTransitionKnowledgeLoopUsecase(dedupe, appendPort, nil, time.Now)

	in := newTransitionInput(t, "LOOP_STAGE_OBSERVE", "LOOP_STAGE_ORIENT", "TRANSITION_TRIGGER_USER_TAP")
	res, err := uc.Execute(context.Background(), in)

	require.NoError(t, err)
	require.True(t, res.Accepted)
	require.Len(t, appendPort.events, 1)
	require.Equal(t, domain.EventKnowledgeLoopOriented, appendPort.events[0].EventType,
		"user_tap on observe→orient must classify as KnowledgeLoopOriented")
}

func TestTransition_ReviewActionPayloadUsesTriggerNotAction(t *testing.T) {
	dedupe := &fakeDedupePort{reserved: true}
	appendPort := &fakeAppendPort{}
	uc := NewTransitionKnowledgeLoopUsecase(dedupe, appendPort, nil, time.Now)

	in := newTransitionInput(t, "LOOP_STAGE_OBSERVE", "LOOP_STAGE_OBSERVE", "TRANSITION_TRIGGER_RECHECK")
	res, err := uc.Execute(context.Background(), in)

	require.NoError(t, err)
	require.True(t, res.Accepted)
	require.Len(t, appendPort.events, 1)
	ev := appendPort.events[0]
	require.Equal(t, domain.EventKnowledgeLoopReviewed, ev.EventType)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(ev.Payload, &payload))
	require.Equal(t, "TRANSITION_TRIGGER_RECHECK", payload["trigger"])
	require.NotContains(t, payload, "action",
		"review action distinction lives in TransitionTrigger, not a parallel action field")
}

func TestTransition_ActedPayloadCarriesIntentMetadata(t *testing.T) {
	dedupe := &fakeDedupePort{reserved: true}
	appendPort := &fakeAppendPort{}
	uc := NewTransitionKnowledgeLoopUsecase(dedupe, appendPort, nil, time.Now)

	acted := "DECISION_INTENT_OPEN"
	actionID := "open"
	targetType := "ACT_TARGET_TYPE_ARTICLE"
	targetRef := "article:42"
	in := newTransitionInput(t, "LOOP_STAGE_DECIDE", "LOOP_STAGE_ACT", "TRANSITION_TRIGGER_USER_TAP")
	in.ActedIntent = &acted
	in.ActionID = &actionID
	in.TargetType = &targetType
	in.TargetRef = &targetRef
	in.ContinueFlag = true

	res, err := uc.Execute(context.Background(), in)

	require.NoError(t, err)
	require.True(t, res.Accepted)
	require.Len(t, appendPort.events, 1)
	require.Equal(t, domain.EventKnowledgeLoopActed, appendPort.events[0].EventType)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(appendPort.events[0].Payload, &payload))
	require.Equal(t, "DECISION_INTENT_OPEN", payload["acted_intent"])
	require.Equal(t, "open", payload["action_id"])
	require.Equal(t, "ACT_TARGET_TYPE_ARTICLE", payload["target_type"])
	require.Equal(t, "article:42", payload["target_ref"])
	require.Equal(t, true, payload["continue_flag"])
}

func TestTransition_SkipsAppendOnDuplicateReservation(t *testing.T) {
	canonical := "article:42"
	dedupe := &fakeDedupePort{
		reserved: false,
		cached:   &knowledge_loop_port.CachedTransitionResponse{CanonicalEntryKey: &canonical},
	}
	appendPort := &fakeAppendPort{}
	uc := NewTransitionKnowledgeLoopUsecase(dedupe, appendPort, nil, time.Now)

	in := newTransitionInput(t, "LOOP_STAGE_OBSERVE", "LOOP_STAGE_ORIENT", "TRANSITION_TRIGGER_USER_TAP")
	res, err := uc.Execute(context.Background(), in)

	require.NoError(t, err)
	require.False(t, res.Accepted, "duplicate must not re-accept")
	require.Equal(t, 0, appendPort.callCount,
		"duplicate reserve must not re-emit the event (single emission rule)")
}

func TestTransition_ForbiddenTransitionRejected_NoSideEffects(t *testing.T) {
	dedupe := &fakeDedupePort{reserved: true}
	appendPort := &fakeAppendPort{}
	uc := NewTransitionKnowledgeLoopUsecase(dedupe, appendPort, nil, time.Now)

	// observe->act is forbidden per ADR-000831 §7.
	in := newTransitionInput(t, "LOOP_STAGE_OBSERVE", "LOOP_STAGE_ACT", "TRANSITION_TRIGGER_USER_TAP")
	_, err := uc.Execute(context.Background(), in)

	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidArgument)
	require.Equal(t, 0, dedupe.calls, "forbidden transition must be rejected before reserving idempotency")
	require.Equal(t, 0, appendPort.callCount)
}

func TestTransition_AppendFailurePropagates(t *testing.T) {
	dedupe := &fakeDedupePort{reserved: true}
	appendPort := &fakeAppendPort{err: errors.New("sovereign unavailable")}
	uc := NewTransitionKnowledgeLoopUsecase(dedupe, appendPort, nil, time.Now)

	in := newTransitionInput(t, "LOOP_STAGE_OBSERVE", "LOOP_STAGE_ORIENT", "TRANSITION_TRIGGER_USER_TAP")
	_, err := uc.Execute(context.Background(), in)

	require.Error(t, err)
	require.Equal(t, 1, appendPort.callCount)
}

func TestTransition_RateLimit_RejectsOverGlobalCeiling(t *testing.T) {
	// Pre-fill the global ceiling so the next transition trips the limiter,
	// regardless of whether it would otherwise be idempotency-reserved.
	fixedNow := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	dedupe := &fakeDedupePort{reserved: true}
	appendPort := &fakeAppendPort{}
	limiter := NewLoopRateLimiter(func() time.Time { return fixedNow })

	userID := uuid.New()
	// Exhaust the 600/min ceiling.
	for i := 0; i < 600; i++ {
		_, _ = limiter.AllowGlobal(userID, fixedNow)
	}

	uc := NewTransitionKnowledgeLoopUsecase(dedupe, appendPort, limiter, func() time.Time { return fixedNow })
	in := newTransitionInput(t, "LOOP_STAGE_OBSERVE", "LOOP_STAGE_ORIENT", "TRANSITION_TRIGGER_USER_TAP")
	in.UserID = userID
	// Mint a UUIDv7 whose embedded timestamp matches fixedNow so the validator
	// passes regardless of real wall-clock drift. A plain uuid.NewV7() bakes in
	// the machine clock which may be far from fixedNow → validator rejects.
	in.ClientTransitionID = mustMintUUIDv7(t, fixedNow)

	_, err := uc.Execute(context.Background(), in)

	require.Error(t, err)
	require.ErrorIs(t, err, ErrRateLimited,
		"over-ceiling transitions must surface ErrRateLimited so the handler can map to CodeResourceExhausted")
	require.Equal(t, 0, appendPort.callCount,
		"a rate-limited transition must not reach event append")
	require.Equal(t, 0, dedupe.calls,
		"a rate-limited transition must not consume the dedupe reservation")
}

func TestTransition_DwellAlwaysRejected(t *testing.T) {
	// Auto-OODA suppression: dwell is rejected for every (from, to) tuple.
	// This pins the "no passive stage advancement" invariant defensively;
	// the frontend no longer fires dwell at all.
	for _, target := range []string{
		"LOOP_STAGE_OBSERVE",
		"LOOP_STAGE_ORIENT",
		"LOOP_STAGE_DECIDE",
		"LOOP_STAGE_ACT",
	} {
		t.Run("dwell→"+target, func(t *testing.T) {
			dedupe := &fakeDedupePort{reserved: true}
			appendPort := &fakeAppendPort{}
			uc := NewTransitionKnowledgeLoopUsecase(dedupe, appendPort, nil, time.Now)

			in := newTransitionInput(t, "LOOP_STAGE_OBSERVE", "LOOP_STAGE_ORIENT", "TRANSITION_TRIGGER_DWELL")
			in.ToStage = target
			_, err := uc.Execute(context.Background(), in)

			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidArgument)
			require.Equal(t, 0, appendPort.callCount,
				"dwell→%s must not append any event", target)
		})
	}
}
