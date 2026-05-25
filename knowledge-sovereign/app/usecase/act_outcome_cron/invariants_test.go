package act_outcome_cron

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

// These invariants lock the event-time purity rules from ADR-000908 §Δ1 /
// canonical contract §6: business-fact times (event.OccurredAt) must derive
// from event payload only, not from the cron's wall-clock. Clock() is an
// identifier ("which acted events have aged past the cutoff?"), not a
// business fact.

// TestBuildNoEngagement_OccurredAt_NeverUsesClock asserts that the emitted
// outcome's occurred_at is acted.OccurredAt + window — independent of any
// clock. Reproject under the same event log produces the same observed_at.
func TestBuildNoEngagement_OccurredAt_NeverUsesClock(t *testing.T) {
	t.Parallel()

	acted := actedEvent(
		100,
		uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		"entry-1",
		time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
	)

	window := 7 * 24 * time.Hour
	got, err := buildNoEngagementOutcome(acted, window)
	require.NoError(t, err)

	wantOccurredAt := acted.OccurredAt.Add(window)
	require.True(t, got.OccurredAt.Equal(wantOccurredAt),
		"occurred_at must be acted.OccurredAt + window; got %v, want %v", got.OccurredAt, wantOccurredAt)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(got.Payload, &payload))
	require.Equal(t, wantOccurredAt.UTC().Format(time.RFC3339Nano), payload["observed_at"],
		"payload.observed_at must mirror acted.OccurredAt + window — never wall-clock")
}

// TestRunBatch_BackfillCutoff_DoesNotLeakWallClockIntoPayload pins that when
// the operator passes a BackfillCutoff, the wall-clock from c.cfg.Clock is
// ignored entirely. The emitted event payloads must be identical to a run
// without BackfillCutoff against the same event log.
func TestRunBatch_BackfillCutoff_DoesNotLeakWallClockIntoPayload(t *testing.T) {
	t.Parallel()

	uid := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	acted := actedEvent(200, uid, "entry-200",
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	repoA := &fakeRepo{noOutcomeActed: []sovereign_db.KnowledgeEvent{acted}}
	repoB := &fakeRepo{noOutcomeActed: []sovereign_db.KnowledgeEvent{acted}}

	cutoff := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	cronA := New(repoA, nopLogger(), Config{
		Clock:          func() time.Time { return time.Date(3000, 1, 1, 0, 0, 0, 0, time.UTC) }, // absurd wall-clock
		BackfillCutoff: &cutoff,
	})
	cronB := New(repoB, nopLogger(), Config{
		Clock:          func() time.Time { return time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC) }, // different absurd wall-clock
		BackfillCutoff: &cutoff,
	})

	require.NoError(t, cronA.RunBatch(context.Background()))
	require.NoError(t, cronB.RunBatch(context.Background()))

	require.Len(t, repoA.appended, 1)
	require.Len(t, repoB.appended, 1)
	require.True(t, repoA.appended[0].OccurredAt.Equal(repoB.appended[0].OccurredAt),
		"different clocks must yield identical OccurredAt under the same BackfillCutoff and event log")
	require.Equal(t, repoA.appended[0].Payload, repoB.appended[0].Payload,
		"payloads must be byte-identical regardless of clock under fixed BackfillCutoff")
}

// TestRunBatch_SameEventLog_TwoDifferentClocks_ProducesIdenticalEvents is the
// strongest reproject-safety check: two crons running with different wall
// clocks must emit events that differ only in event_id (a UUID, intentionally
// non-deterministic). occurred_at, payload, dedupe_key, aggregate_id are
// pure functions of the event log.
func TestRunBatch_SameEventLog_TwoDifferentClocks_ProducesIdenticalEvents(t *testing.T) {
	t.Parallel()

	uid := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	acted := actedEvent(300, uid, "entry-300",
		time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))

	repoA := &fakeRepo{noOutcomeActed: []sovereign_db.KnowledgeEvent{acted}}
	repoB := &fakeRepo{noOutcomeActed: []sovereign_db.KnowledgeEvent{acted}}

	// Both clocks pass the cutoff (now - 7d > acted.OccurredAt) so the
	// query result is non-empty — the cutoff value differs but the emitted
	// payload must not depend on it.
	cronA := New(repoA, nopLogger(), Config{Clock: func() time.Time {
		return time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	}})
	cronB := New(repoB, nopLogger(), Config{Clock: func() time.Time {
		return time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	}})

	require.NoError(t, cronA.RunBatch(context.Background()))
	require.NoError(t, cronB.RunBatch(context.Background()))

	require.Len(t, repoA.appended, 1)
	require.Len(t, repoB.appended, 1)

	a, b := repoA.appended[0], repoB.appended[0]
	require.True(t, a.OccurredAt.Equal(b.OccurredAt),
		"occurred_at must be event-time pure: %v vs %v", a.OccurredAt, b.OccurredAt)
	require.Equal(t, a.Payload, b.Payload,
		"payload must be byte-identical under two different clocks")
	require.Equal(t, a.DedupeKey, b.DedupeKey,
		"dedupe_key must derive from acted.event_id, not clock")
	require.Equal(t, a.AggregateID, b.AggregateID)
	require.Equal(t, a.EventType, b.EventType)
	require.Equal(t, a.AggregateType, b.AggregateType)
	require.Equal(t, a.ActorID, b.ActorID)
	require.Equal(t, a.ActorType, b.ActorType)
	require.True(t, strings.HasPrefix(a.DedupeKey, "knowledge_loop.act_outcome.v1:"),
		"dedupe_key shape must remain stable: %s", a.DedupeKey)
}
