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

// Dwell from observe→orient is the canonical KnowledgeLoopObserved event
// fired by the FE IntersectionObserver. Prior to 2026-04-24, the validator
// rejected this tuple (dwell → ORIENT) even though ClassifyTransitionEvent
// explicitly classifies it as EventKnowledgeLoopObserved — producing the
// production 502 bursts on `/loop/transition`. This test pins the end-to-end
// Execute path so the regression can't return.
func TestTransition_DwellObserveToOrient_EmitsObservedEvent(t *testing.T) {
	dedupe := &fakeDedupePort{reserved: true}
	appendPort := &fakeAppendPort{}
	uc := NewTransitionKnowledgeLoopUsecase(dedupe, appendPort, nil, time.Now)

	in := newTransitionInput(t, "LOOP_STAGE_OBSERVE", "LOOP_STAGE_ORIENT", "TRANSITION_TRIGGER_DWELL")
	res, err := uc.Execute(context.Background(), in)

	require.NoError(t, err)
	require.True(t, res.Accepted)
	require.Len(t, appendPort.events, 1)
	require.Equal(t, domain.EventKnowledgeLoopObserved, appendPort.events[0].EventType,
		"dwell on observe→orient must classify as KnowledgeLoopObserved (contract §8.2)")
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

func TestTransition_DwellNonObserveStillRejected(t *testing.T) {
	// DWELL trigger is only valid when to_stage == OBSERVE; ValidateDwellTriggerTarget
	// continues to apply even when the new classify function runs afterward.
	dedupe := &fakeDedupePort{reserved: true}
	appendPort := &fakeAppendPort{}
	uc := NewTransitionKnowledgeLoopUsecase(dedupe, appendPort, nil, time.Now)

	in := newTransitionInput(t, "LOOP_STAGE_OBSERVE", "LOOP_STAGE_ORIENT", "TRANSITION_TRIGGER_DWELL")
	in.ToStage = "LOOP_STAGE_DECIDE" // dwell target != OBSERVE
	_, err := uc.Execute(context.Background(), in)

	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidArgument)
	require.Equal(t, 0, appendPort.callCount)
}
