package act_outcome_cron

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
	"knowledge-sovereign/usecase/knowledge_loop_projector"
)

// fakeRepo captures the cron's calls and lets a test inject the
// "acted-without-outcome" query result deterministically. Mirrors the
// surface_planner_cron fake to keep the patterns aligned.
type fakeRepo struct {
	noOutcomeActed []sovereign_db.KnowledgeEvent
	appended       []sovereign_db.KnowledgeEvent

	// recorded query parameters so we can assert event-time binding.
	lastCutoff time.Time
	lastLimit  int
}

func (f *fakeRepo) ListActedEventsWithoutOutcome(_ context.Context, cutoff time.Time, limit int) ([]sovereign_db.KnowledgeEvent, error) {
	f.lastCutoff = cutoff
	f.lastLimit = limit
	return f.noOutcomeActed, nil
}

func (f *fakeRepo) AppendKnowledgeEvent(_ context.Context, ev sovereign_db.KnowledgeEvent) (int64, error) {
	for _, prior := range f.appended {
		if prior.DedupeKey == ev.DedupeKey {
			return 0, nil
		}
	}
	f.appended = append(f.appended, ev)
	return int64(len(f.appended)), nil
}

func nopLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func actedEvent(seq int64, userID uuid.UUID, entryKey string, occurredAt time.Time) sovereign_db.KnowledgeEvent {
	body, _ := json.Marshal(map[string]any{
		"entry_key":     entryKey,
		"acted_intent":  "open",
		"continue_flag": true,
		"to_stage":      "LOOP_STAGE_ACT",
	})
	uid := userID
	return sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      seq,
		OccurredAt:    occurredAt,
		TenantID:      uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		UserID:        &uid,
		ActorType:     "user",
		EventType:     knowledge_loop_projector.EventKnowledgeLoopActed,
		AggregateType: knowledge_loop_projector.AggregateLoopSession,
		AggregateID:   entryKey,
		DedupeKey:     "knowledge_loop.acted.v1:" + entryKey + ":" + uuid.NewString(),
		Payload:       body,
	}
}

// fixedClock returns a deterministic wall-clock. The cron uses it only as a
// boundary to decide which acted events have aged past 7d; the emitted
// outcome event's occurred_at is always derived from the acted event's
// occurred_at + 7d (event-time purity).
func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

func TestRunBatch_NoActedWithoutOutcome_NoEmit(t *testing.T) {
	repo := &fakeRepo{}
	c := New(repo, nopLogger(), Config{Clock: fixedClock(time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC))})

	require.NoError(t, c.RunBatch(context.Background()))
	require.Empty(t, repo.appended, "no acted events past the cutoff → no fallback emit")
}

// Core happy path: an acted event whose occurred_at + 7d ≤ cron_now and no
// outcome yet → the cron appends a single no_engagement outcome with
// observed_at == acted.occurred_at + 7d (event-time purity).
func TestRunBatch_AgedActedWithoutOutcome_EmitsNoEngagement(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	actedAt := now.Add(-8 * 24 * time.Hour) // 8 days old, well past 7d
	userID := uuid.New()
	acted := actedEvent(101, userID, "article:42", actedAt)

	repo := &fakeRepo{noOutcomeActed: []sovereign_db.KnowledgeEvent{acted}}
	c := New(repo, nopLogger(), Config{Clock: fixedClock(now)})
	require.NoError(t, c.RunBatch(context.Background()))

	require.Len(t, repo.appended, 1, "one no_engagement outcome must be emitted")

	emitted := repo.appended[0]
	require.Equal(t, knowledge_loop_projector.EventKnowledgeLoopActOutcome, emitted.EventType)
	require.Equal(t, "system", emitted.ActorType)
	require.NotNil(t, emitted.UserID)
	require.Equal(t, userID, *emitted.UserID)

	// Business fact: observed_at must be acted.occurred_at + 7d (event-time
	// purity — not the wall-clock cron_now).
	wantObservedAt := actedAt.Add(7 * 24 * time.Hour)
	require.True(t, emitted.OccurredAt.Equal(wantObservedAt),
		"occurred_at must be acted.occurred_at + 7d (event-time bound), got %v want %v",
		emitted.OccurredAt, wantObservedAt)

	var payload struct {
		ActedEventID string `json:"acted_event_id"`
		EntryKey     string `json:"entry_key"`
		Outcome      string `json:"outcome"`
		ObservedAt   string `json:"observed_at"`
	}
	require.NoError(t, json.Unmarshal(emitted.Payload, &payload))
	require.Equal(t, acted.EventID.String(), payload.ActedEventID,
		"payload.acted_event_id must reference the originating Acted event")
	require.Equal(t, "article:42", payload.EntryKey)
	require.Equal(t, "no_engagement", payload.Outcome)
	require.Equal(t, wantObservedAt.UTC().Format(time.RFC3339Nano), payload.ObservedAt,
		"payload.observed_at must mirror the event header's occurred_at")
}

// The cutoff passed to the repository must be (cron_now - 7d). The repo is
// responsible for the NOT-EXISTS check, but the cron must bind the window
// from wall-clock - 7d so the SQL boundary catches the right rows.
func TestRunBatch_BindsCutoffToCronNowMinus7d(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	repo := &fakeRepo{}
	c := New(repo, nopLogger(), Config{Clock: fixedClock(now)})
	require.NoError(t, c.RunBatch(context.Background()))

	wantCutoff := now.Add(-7 * 24 * time.Hour)
	require.True(t, repo.lastCutoff.Equal(wantCutoff),
		"cutoff must be cron_now - 7d, got %v want %v", repo.lastCutoff, wantCutoff)
}

// Reproject-safety: emitting the same fallback twice for the same acted
// event must be a no-op at the dedupe layer. We assert by re-running the
// cron with the same input and confirming no second append.
func TestRunBatch_IdempotentDedupeKey(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	actedAt := now.Add(-8 * 24 * time.Hour)
	userID := uuid.New()
	acted := actedEvent(101, userID, "article:42", actedAt)

	repo := &fakeRepo{noOutcomeActed: []sovereign_db.KnowledgeEvent{acted}}
	c := New(repo, nopLogger(), Config{Clock: fixedClock(now)})

	require.NoError(t, c.RunBatch(context.Background()))
	require.Len(t, repo.appended, 1)

	// Second tick with the same input — the repo's dedupe layer rejects the
	// duplicate dedupe_key, so no second event gets appended.
	require.NoError(t, c.RunBatch(context.Background()))
	require.Len(t, repo.appended, 1, "re-running the cron must not double-emit")

	require.Equal(t,
		"knowledge_loop.act_outcome.v1:"+acted.EventID.String()+":no_engagement",
		repo.appended[0].DedupeKey,
		"dedupe_key must key on (event_type, acted_event_id, outcome) so reruns idempotent",
	)
}

// Multiple acted events without outcomes → multiple no_engagement emits
// (one per acted), each event-time bound to its source acted.
func TestRunBatch_MultipleAged_EmitsOnePerActed(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	userA := uuid.New()
	userB := uuid.New()

	actedA := actedEvent(101, userA, "article:42", now.Add(-9*24*time.Hour))
	actedB := actedEvent(102, userB, "article:99", now.Add(-7*24*time.Hour-time.Hour))

	repo := &fakeRepo{noOutcomeActed: []sovereign_db.KnowledgeEvent{actedA, actedB}}
	c := New(repo, nopLogger(), Config{Clock: fixedClock(now)})
	require.NoError(t, c.RunBatch(context.Background()))

	require.Len(t, repo.appended, 2, "one outcome per aged acted event")
	for _, e := range repo.appended {
		require.Equal(t, "system", e.ActorType)
		require.Equal(t, knowledge_loop_projector.EventKnowledgeLoopActOutcome, e.EventType)
	}
}

// A nil UserID on the acted event is unexpected (system events do not enter
// this query) but the cron must skip rather than crash so a malformed
// upstream cannot break the batch.
func TestRunBatch_SkipsActedWithNilUserID(t *testing.T) {
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	acted := actedEvent(101, uuid.New(), "article:42", now.Add(-8*24*time.Hour))
	acted.UserID = nil // simulate malformed upstream

	repo := &fakeRepo{noOutcomeActed: []sovereign_db.KnowledgeEvent{acted}}
	c := New(repo, nopLogger(), Config{Clock: fixedClock(now)})

	require.NoError(t, c.RunBatch(context.Background()))
	require.Empty(t, repo.appended, "nil UserID must be skipped, not crash the batch")
}

// Reproject-safety / invariant: the emitted outcome event's occurred_at
// MUST be derived purely from the source acted event's occurred_at, never
// from cron_now. Replaying the same event log with a different cron_now
// must produce the same outcome event.
func TestRunBatch_OutcomeOccurredAtIndependentOfCronNow(t *testing.T) {
	actedAt := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	userID := uuid.New()
	acted := actedEvent(101, userID, "article:42", actedAt)

	now1 := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	now2 := time.Date(2026, 6, 1, 3, 0, 0, 0, time.UTC)

	repo1 := &fakeRepo{noOutcomeActed: []sovereign_db.KnowledgeEvent{acted}}
	require.NoError(t, New(repo1, nopLogger(), Config{Clock: fixedClock(now1)}).RunBatch(context.Background()))

	repo2 := &fakeRepo{noOutcomeActed: []sovereign_db.KnowledgeEvent{acted}}
	require.NoError(t, New(repo2, nopLogger(), Config{Clock: fixedClock(now2)}).RunBatch(context.Background()))

	require.Len(t, repo1.appended, 1)
	require.Len(t, repo2.appended, 1)
	require.True(t, repo1.appended[0].OccurredAt.Equal(repo2.appended[0].OccurredAt),
		"outcome.occurred_at must depend on acted.occurred_at, not on cron wall-clock")
	require.Equal(t, repo1.appended[0].DedupeKey, repo2.appended[0].DedupeKey,
		"dedupe_key must be deterministic across cron runs")
}
