package emit_act_outcome_usecase

import (
	"alt/domain"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// fakeAppendPort captures the event submitted by the usecase. The contract
// it implements (AppendKnowledgeEventPort) returns (event_seq, err) — we
// return a fixed non-zero seq on success.
type fakeAppendPort struct {
	called  []domain.KnowledgeEvent
	err     error
	nextSeq int64
}

func (f *fakeAppendPort) AppendKnowledgeEvent(_ context.Context, ev domain.KnowledgeEvent) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.called = append(f.called, ev)
	if f.nextSeq == 0 {
		return 1, nil
	}
	seq := f.nextSeq
	f.nextSeq++
	return seq, nil
}

// DeriveOutcomeKind pins the threshold table (ADR-000908 §Δ1):
//
//	dwell ≥ 30s         → engaged
//	ask turn ≥ 3        → deep_engagement (article close-read or
//	                       sustained Augur conversation)
//	otherwise           → unspecified (caller should not emit)
//
// The function is pure so the projector and the resolver can rely on
// deterministic outcome classification regardless of event ordering.
func TestDeriveOutcomeKind_Table(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		dwellMs  int64
		askTurns int32
		want     string
		wantEmit bool
	}{
		{name: "deep engagement from ask turns alone", dwellMs: 0, askTurns: 3, want: "deep_engagement", wantEmit: true},
		{name: "deep engagement from many ask turns", dwellMs: 0, askTurns: 7, want: "deep_engagement", wantEmit: true},
		{name: "engaged from dwell only", dwellMs: 30_000, askTurns: 0, want: "engaged", wantEmit: true},
		{name: "engaged at the dwell threshold", dwellMs: 30_000, askTurns: 2, want: "engaged", wantEmit: true},
		{name: "deep wins when both clear", dwellMs: 45_000, askTurns: 4, want: "deep_engagement", wantEmit: true},
		{name: "below dwell, below ask: unspecified, no emit", dwellMs: 25_000, askTurns: 2, want: "unspecified", wantEmit: false},
		{name: "zero signals: unspecified", dwellMs: 0, askTurns: 0, want: "unspecified", wantEmit: false},
		{name: "negative inputs ignored", dwellMs: -1, askTurns: -5, want: "unspecified", wantEmit: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, emit := DeriveOutcomeKind(tc.dwellMs, tc.askTurns)
			require.Equal(t, tc.want, got, "outcome kind mismatch")
			require.Equal(t, tc.wantEmit, emit, "emit gate mismatch")
		})
	}
}

func TestExecute_AppendsActOutcomeEvent(t *testing.T) {
	t.Parallel()

	port := &fakeAppendPort{}
	uc := New(port, func() time.Time { return time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC) })

	actedAt := time.Date(2026, 5, 23, 11, 59, 30, 0, time.UTC)
	actedEventID := uuid.New()
	userID := uuid.New()
	tenantID := uuid.New()

	err := uc.Execute(context.Background(), Input{
		TenantID:     tenantID,
		UserID:       userID,
		LensModeID:   "default",
		ActedEventID: actedEventID,
		EntryKey:     "article:42",
		Outcome:      "engaged",
		ObservedAt:   actedAt.Add(30 * time.Second),
	})
	require.NoError(t, err)
	require.Len(t, port.called, 1)

	ev := port.called[0]
	require.Equal(t, domain.EventKnowledgeLoopActOutcome, ev.EventType)
	require.Equal(t, domain.ActorSystem, ev.ActorType,
		"act_outcome.v1 is a system event regardless of which user triggered it")
	require.Equal(t, "knowledge_loop_entry", ev.AggregateType)
	require.Equal(t, "article:42", ev.AggregateID)
	require.Equal(t, tenantID, ev.TenantID)
	require.NotNil(t, ev.UserID)
	require.Equal(t, userID, *ev.UserID)

	// dedupe_key is keyed on (event_type, acted_event_id, outcome) so
	// re-invocation with the same input is a no-op at the sovereign side.
	require.Equal(t,
		"knowledge_loop.act_outcome.v1:"+actedEventID.String()+":engaged",
		ev.DedupeKey,
	)

	var payload struct {
		ActedEventID string `json:"acted_event_id"`
		EntryKey     string `json:"entry_key"`
		LensModeID   string `json:"lens_mode_id"`
		Outcome      string `json:"outcome"`
		ObservedAt   string `json:"observed_at"`
	}
	require.NoError(t, json.Unmarshal(ev.Payload, &payload))
	require.Equal(t, actedEventID.String(), payload.ActedEventID)
	require.Equal(t, "article:42", payload.EntryKey)
	require.Equal(t, "default", payload.LensModeID)
	require.Equal(t, "engaged", payload.Outcome)

	// occurred_at must echo the caller-supplied observed_at (event-time
	// purity). The usecase does NOT default to its nowFunc for the event
	// header — that would let wall-clock leak into a business fact.
	require.True(t, ev.OccurredAt.Equal(actedAt.Add(30*time.Second)),
		"occurred_at must mirror caller-supplied observed_at, not wall-clock")
}

func TestExecute_RejectsUnknownOutcome(t *testing.T) {
	t.Parallel()
	port := &fakeAppendPort{}
	uc := New(port, time.Now)

	err := uc.Execute(context.Background(), Input{
		TenantID:     uuid.New(),
		UserID:       uuid.New(),
		LensModeID:   "default",
		ActedEventID: uuid.New(),
		EntryKey:     "article:42",
		Outcome:      "totally_made_up",
		ObservedAt:   time.Now(),
	})
	require.Error(t, err, "unknown outcome strings must be rejected at the usecase boundary")
	require.Empty(t, port.called, "no event should be appended when outcome is invalid")
}

func TestExecute_RejectsNoEngagement(t *testing.T) {
	t.Parallel()
	port := &fakeAppendPort{}
	uc := New(port, time.Now)

	// `no_engagement` is the system-only fallback emitted by
	// act_outcome_cron — it must NEVER come from the alt-backend view
	// tracker path, even if a malicious / buggy frontend sends it.
	err := uc.Execute(context.Background(), Input{
		TenantID:     uuid.New(),
		UserID:       uuid.New(),
		LensModeID:   "default",
		ActedEventID: uuid.New(),
		EntryKey:     "article:42",
		Outcome:      "no_engagement",
		ObservedAt:   time.Now(),
	})
	require.Error(t, err)
	require.Empty(t, port.called)
}

func TestExecute_RejectsEmptyEntryKey(t *testing.T) {
	t.Parallel()
	port := &fakeAppendPort{}
	uc := New(port, time.Now)
	err := uc.Execute(context.Background(), Input{
		TenantID:     uuid.New(),
		UserID:       uuid.New(),
		LensModeID:   "default",
		ActedEventID: uuid.New(),
		EntryKey:     "",
		Outcome:      "engaged",
		ObservedAt:   time.Now(),
	})
	require.Error(t, err)
}

func TestExecute_PropagatesAppendError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("upstream sovereign blew up")
	port := &fakeAppendPort{err: wantErr}
	uc := New(port, time.Now)
	err := uc.Execute(context.Background(), Input{
		TenantID:     uuid.New(),
		UserID:       uuid.New(),
		LensModeID:   "default",
		ActedEventID: uuid.New(),
		EntryKey:     "article:42",
		Outcome:      "engaged",
		ObservedAt:   time.Now(),
	})
	require.ErrorIs(t, err, wantErr,
		"append errors must be wrapped, not swallowed — caller decides whether to retry")
}

// Reproject-safety: the same Input produces the same KnowledgeEvent
// payload, dedupe_key, and occurred_at. Event_id changes (uuid.New) but
// the dedupe key carries idempotency.
func TestExecute_DeterministicDedupeAndOccurredAt(t *testing.T) {
	t.Parallel()
	port1 := &fakeAppendPort{}
	port2 := &fakeAppendPort{}
	uc1 := New(port1, func() time.Time { return time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC) })
	uc2 := New(port2, func() time.Time { return time.Date(2099, 12, 31, 23, 59, 59, 0, time.UTC) })

	in := Input{
		TenantID:     uuid.MustParse("11111111-1111-4111-8111-111111111111"),
		UserID:       uuid.MustParse("22222222-2222-4222-8222-222222222222"),
		LensModeID:   "default",
		ActedEventID: uuid.MustParse("33333333-3333-4333-8333-333333333333"),
		EntryKey:     "article:42",
		Outcome:      "deep_engagement",
		ObservedAt:   time.Date(2026, 5, 23, 12, 0, 30, 0, time.UTC),
	}
	require.NoError(t, uc1.Execute(context.Background(), in))
	require.NoError(t, uc2.Execute(context.Background(), in))

	require.Equal(t, port1.called[0].DedupeKey, port2.called[0].DedupeKey)
	require.True(t, port1.called[0].OccurredAt.Equal(port2.called[0].OccurredAt),
		"occurred_at must depend only on Input.ObservedAt, not nowFunc")
	require.JSONEq(t, string(port1.called[0].Payload), string(port2.called[0].Payload))
}
