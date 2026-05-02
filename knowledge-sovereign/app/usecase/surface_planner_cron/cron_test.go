package surface_planner_cron

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

type fakeRepo struct {
	checkpoint     int64
	events         []sovereign_db.KnowledgeEvent
	appended       []sovereign_db.KnowledgeEvent
	checkpointSets []int64
	windowEvents   []sovereign_db.KnowledgeEvent
}

func (f *fakeRepo) GetProjectionCheckpoint(_ context.Context, _ string) (int64, error) {
	return f.checkpoint, nil
}

func (f *fakeRepo) UpdateProjectionCheckpoint(_ context.Context, _ string, lastSeq int64) error {
	f.checkpoint = lastSeq
	f.checkpointSets = append(f.checkpointSets, lastSeq)
	return nil
}

func (f *fakeRepo) ListKnowledgeEventsSince(_ context.Context, afterSeq int64, limit int) ([]sovereign_db.KnowledgeEvent, error) {
	out := make([]sovereign_db.KnowledgeEvent, 0)
	for _, e := range f.events {
		if e.EventSeq > afterSeq {
			out = append(out, e)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (f *fakeRepo) ListKnowledgeEventsForUserInWindow(_ context.Context, _ uuid.UUID, _ []string, _, _ time.Time, _ int) ([]sovereign_db.KnowledgeEvent, error) {
	return f.windowEvents, nil
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

func augurEvent(seq int64, userID uuid.UUID, entryKey string) sovereign_db.KnowledgeEvent {
	body, _ := json.Marshal(map[string]any{
		"entry_key":       entryKey,
		"conversation_id": uuid.NewString(),
		"linked_at":       "2026-05-01T10:00:00Z",
	})
	return sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      seq,
		OccurredAt:    time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC),
		TenantID:      uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		UserID:        &userID,
		EventType:     knowledge_loop_projector.EventAugurConversationLinked,
		AggregateType: knowledge_loop_projector.AggregateLoopSession,
		AggregateID:   entryKey,
		DedupeKey:     "augur.conversation_linked.v1:" + entryKey,
		Payload:       body,
	}
}

func TestRunBatch_NoSignalEvents_NoEmit(t *testing.T) {
	repo := &fakeRepo{}
	c := New(repo, nopLogger(), Config{BatchSize: 100})
	require.NoError(t, c.RunBatch(context.Background()))

	require.Empty(t, repo.appended, "no signal events → no SurfacePlanRecomputed emitted")
}

func TestRunBatch_SingleAugurLink_EmitsOneEventWithOneEntryInput(t *testing.T) {
	userID := uuid.New()
	repo := &fakeRepo{events: []sovereign_db.KnowledgeEvent{augurEvent(101, userID, "article:42")}}
	c := New(repo, nopLogger(), Config{BatchSize: 100})

	require.NoError(t, c.RunBatch(context.Background()))

	require.Len(t, repo.appended, 1, "one SurfacePlanRecomputed emitted")
	emitted := repo.appended[0]
	require.Equal(t, knowledge_loop_projector.EventKnowledgeLoopSurfacePlanRecomputed, emitted.EventType)
	require.NotNil(t, emitted.UserID)
	require.Equal(t, userID, *emitted.UserID)
	require.Equal(t, knowledge_loop_projector.AggregateLoopSession, emitted.AggregateType)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(emitted.Payload, &payload))

	entries, ok := payload["entry_inputs"].([]any)
	require.True(t, ok, "payload must contain entry_inputs[]")
	require.Len(t, entries, 1)

	entry := entries[0].(map[string]any)
	require.Equal(t, "article:42", entry["entry_key"])
	require.Equal(t, "default", payload["lens_mode_id"])
	require.Equal(t, "SURFACE_PLANNER_VERSION_V2", payload["planner_version"])

	require.Equal(t, []int64{101}, repo.checkpointSets, "checkpoint advances to last processed seq")
}

func TestRunBatch_TwoAugurLinks_SameUser_CoalesceIntoOneEvent(t *testing.T) {
	userID := uuid.New()
	repo := &fakeRepo{events: []sovereign_db.KnowledgeEvent{
		augurEvent(101, userID, "article:42"),
		augurEvent(102, userID, "article:99"),
	}}
	c := New(repo, nopLogger(), Config{BatchSize: 100})

	require.NoError(t, c.RunBatch(context.Background()))

	require.Len(t, repo.appended, 1, "two signals from same user coalesce into one batch event")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(repo.appended[0].Payload, &payload))
	entries := payload["entry_inputs"].([]any)
	require.Len(t, entries, 2, "both entry_keys carried in entry_inputs[]")
}

func TestRunBatch_TwoAugurLinks_DifferentUsers_EmitTwo(t *testing.T) {
	userA := uuid.New()
	userB := uuid.New()
	repo := &fakeRepo{events: []sovereign_db.KnowledgeEvent{
		augurEvent(101, userA, "article:42"),
		augurEvent(102, userB, "article:99"),
	}}
	c := New(repo, nopLogger(), Config{BatchSize: 100})

	require.NoError(t, c.RunBatch(context.Background()))

	require.Len(t, repo.appended, 2, "different users → one event per user")
}

func TestRunBatch_AugurLinkWithoutEntryKey_Skip(t *testing.T) {
	userID := uuid.New()
	body, _ := json.Marshal(map[string]any{"conversation_id": uuid.NewString()})
	bad := sovereign_db.KnowledgeEvent{
		EventID:    uuid.New(),
		EventSeq:   101,
		OccurredAt: time.Now(),
		TenantID:   uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		UserID:     &userID,
		EventType:  knowledge_loop_projector.EventAugurConversationLinked,
		DedupeKey:  "bad-augur:1",
		Payload:    body,
	}
	repo := &fakeRepo{events: []sovereign_db.KnowledgeEvent{bad}}
	c := New(repo, nopLogger(), Config{BatchSize: 100})

	require.NoError(t, c.RunBatch(context.Background()))
	require.Empty(t, repo.appended, "no entry_key → producer skips, projector branch would have nothing to patch")
	require.Equal(t, []int64{101}, repo.checkpointSets, "checkpoint still advances past skipped events")
}

func TestRunBatch_NonSignalEvent_Skip(t *testing.T) {
	userID := uuid.New()
	body, _ := json.Marshal(map[string]any{"entry_key": "article:42"})
	homeOpen := sovereign_db.KnowledgeEvent{
		EventID:    uuid.New(),
		EventSeq:   101,
		OccurredAt: time.Now(),
		TenantID:   uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		UserID:     &userID,
		EventType:  knowledge_loop_projector.EventHomeItemOpened,
		DedupeKey:  "home-open:1",
		Payload:    body,
	}
	repo := &fakeRepo{events: []sovereign_db.KnowledgeEvent{homeOpen}}
	c := New(repo, nopLogger(), Config{BatchSize: 100})

	require.NoError(t, c.RunBatch(context.Background()))

	require.Empty(t, repo.appended,
		"HomeItemOpened is already projected directly; producer must not double-emit SurfacePlanRecomputed")
}

func TestRunBatch_DedupeKey_IsBatchSeqBased_Idempotent(t *testing.T) {
	userID := uuid.New()
	repo := &fakeRepo{events: []sovereign_db.KnowledgeEvent{augurEvent(101, userID, "article:42")}}
	c := New(repo, nopLogger(), Config{BatchSize: 100})

	require.NoError(t, c.RunBatch(context.Background()))
	require.Len(t, repo.appended, 1)
	firstKey := repo.appended[0].DedupeKey

	repo.checkpoint = 0
	require.NoError(t, c.RunBatch(context.Background()))
	require.Len(t, repo.appended, 1, "second run with same batch_max_seq must dedupe; AppendKnowledgeEvent returns 0")
	require.Equal(t, firstKey, repo.appended[0].DedupeKey)
}

func TestRunBatch_MaxBatchesPerTickCatchesUpMultipleWindows(t *testing.T) {
	userID := uuid.New()
	repo := &fakeRepo{events: []sovereign_db.KnowledgeEvent{
		augurEvent(101, userID, "article:42"),
		augurEvent(102, userID, "article:99"),
		augurEvent(103, userID, "article:100"),
	}}
	c := New(repo, nopLogger(), Config{BatchSize: 1, MaxBatchesPerTick: 3})

	require.NoError(t, c.RunBatch(context.Background()))

	require.Len(t, repo.appended, 3, "three one-event windows should drain in one tick")
	require.Equal(t, []int64{101, 102, 103}, repo.checkpointSets)
	require.Equal(t, int64(103), repo.checkpoint)
}

func TestRunBatch_DoesNotUseWallClock_UsesTriggerOccurredAt(t *testing.T) {
	userID := uuid.New()
	ev := augurEvent(101, userID, "article:42")
	repo := &fakeRepo{events: []sovereign_db.KnowledgeEvent{ev}}
	c := New(repo, nopLogger(), Config{BatchSize: 100})

	require.NoError(t, c.RunBatch(context.Background()))
	require.Len(t, repo.appended, 1)

	emitted := repo.appended[0]
	require.Equal(t, ev.OccurredAt.UTC(), emitted.OccurredAt.UTC(),
		"reproject-safety: emitted occurred_at must equal triggering signal's occurred_at, not time.Now()")
}
