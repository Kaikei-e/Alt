package knowledge_loop_projector

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// fakeRepo records the upsert calls and returns canned results. It does not
// simulate the DB-side seq-hiwater guard; that guard is exercised at the driver
// layer's own tests. Here we verify the projector emits the same upsert payload
// for the same event on replay — the core reproject-safety invariant.
type fakeRepo struct {
	checkpoint  int64
	events      []sovereign_db.KnowledgeEvent
	entries     []*sovereignv1.KnowledgeLoopEntry
	sessions    []*sovereignv1.KnowledgeLoopSessionState
	checkpoints []int64
}

func (f *fakeRepo) ListKnowledgeEventsSince(ctx context.Context, afterSeq int64, limit int) ([]sovereign_db.KnowledgeEvent, error) {
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

func (f *fakeRepo) GetProjectionCheckpoint(ctx context.Context, _ string) (int64, error) {
	return f.checkpoint, nil
}

func (f *fakeRepo) UpdateProjectionCheckpoint(ctx context.Context, _ string, lastSeq int64) error {
	f.checkpoint = lastSeq
	f.checkpoints = append(f.checkpoints, lastSeq)
	return nil
}

func (f *fakeRepo) UpsertKnowledgeLoopEntry(ctx context.Context, e *sovereignv1.KnowledgeLoopEntry) (*sovereign_db.KnowledgeLoopUpsertResult, error) {
	f.entries = append(f.entries, e)
	return &sovereign_db.KnowledgeLoopUpsertResult{Applied: true, ProjectionRevision: 1, ProjectionSeqHiwater: e.ProjectionSeqHiwater}, nil
}

func (f *fakeRepo) UpsertKnowledgeLoopSessionState(ctx context.Context, s *sovereignv1.KnowledgeLoopSessionState) (*sovereign_db.KnowledgeLoopUpsertResult, error) {
	f.sessions = append(f.sessions, s)
	return &sovereign_db.KnowledgeLoopUpsertResult{Applied: true, ProjectionRevision: 1, ProjectionSeqHiwater: s.ProjectionSeqHiwater}, nil
}

func newProjector(repo Repository) *Projector {
	return NewProjector(repo, slog.New(slog.NewTextHandler(testWriter{}, nil)), Config{BatchSize: 100})
}

type testWriter struct{}

func (testWriter) Write(p []byte) (int, error) { return len(p), nil }

func makeEvent(t *testing.T, eventType string, seq int64, userID uuid.UUID, payload map[string]any) sovereign_db.KnowledgeEvent {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	return sovereign_db.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      seq,
		OccurredAt:    time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC),
		TenantID:      uuid.New(),
		UserID:        &userID,
		EventType:     eventType,
		AggregateType: "article",
		AggregateID:   "article:42",
		DedupeKey:     eventType + ":" + uuid.NewString(),
		Payload:       body,
	}
}

func TestRunBatch_HomeItemOpened_ProjectsEntryAndSession(t *testing.T) {
	userID := uuid.New()
	ev := makeEvent(t, EventHomeItemOpened, 100, userID, map[string]any{"entry_key": "article:42"})

	repo := &fakeRepo{events: []sovereign_db.KnowledgeEvent{ev}}
	p := newProjector(repo)
	require.NoError(t, p.RunBatch(context.Background()))

	require.Len(t, repo.entries, 1, "one entry upsert expected")
	require.Len(t, repo.sessions, 1, "one session upsert expected")

	entry := repo.entries[0]
	require.Equal(t, sovereignv1.LoopStage_LOOP_STAGE_ACT, entry.ProposedStage)
	require.Equal(t, sovereignv1.SurfaceBucket_SURFACE_BUCKET_CONTINUE, entry.SurfaceBucket)
	require.Equal(t, sovereignv1.DismissState_DISMISS_STATE_COMPLETED, entry.DismissState)
	require.Equal(t, ev.OccurredAt.UTC(), entry.FreshnessAt.AsTime().UTC(),
		"freshness_at must come from event occurred_at, not wall-clock")

	state := repo.sessions[0]
	require.Equal(t, sovereignv1.LoopStage_LOOP_STAGE_ACT, state.CurrentStage)
	require.Equal(t, ev.OccurredAt.UTC(), state.CurrentStageEnteredAt.AsTime().UTC(),
		"current_stage_entered_at must come from event occurred_at, not wall-clock")

	require.Equal(t, int64(100), repo.checkpoint, "checkpoint advances to last processed seq")
}

func TestRunBatch_SummaryVersionCreated_ObserveSeed(t *testing.T) {
	userID := uuid.New()
	ev := makeEvent(t, EventSummaryVersionCreated, 200, userID, map[string]any{
		"summary_version_id": "sv-1",
		"article_title":      "A Talk on Distributed Systems",
	})
	repo := &fakeRepo{events: []sovereign_db.KnowledgeEvent{ev}}
	p := newProjector(repo)
	require.NoError(t, p.RunBatch(context.Background()))

	require.Len(t, repo.entries, 1)
	entry := repo.entries[0]
	require.Equal(t, sovereignv1.LoopStage_LOOP_STAGE_OBSERVE, entry.ProposedStage)
	require.Contains(t, entry.WhyPrimary.Text, "A Talk on Distributed Systems",
		"narrative must inline the article title for real context")

	var seeded []map[string]string
	require.NoError(t, json.Unmarshal(entry.DecisionOptions, &seeded))
	intents := make([]string, 0, len(seeded))
	for _, s := range seeded {
		intents = append(intents, s["intent"])
	}
	require.Equal(t, []string{"revisit", "ask", "snooze"}, intents,
		"Observe entries must propose §7-allowed transitions; observe → act is forbidden")
}

func TestRunBatch_NoUserIDIsNoOp(t *testing.T) {
	ev := sovereign_db.KnowledgeEvent{
		EventID:    uuid.New(),
		EventSeq:   50,
		OccurredAt: time.Now().UTC(),
		EventType:  EventArticleCreated,
		UserID:     nil,
		Payload:    json.RawMessage(`{}`),
	}
	repo := &fakeRepo{events: []sovereign_db.KnowledgeEvent{ev}}
	p := newProjector(repo)
	require.NoError(t, p.RunBatch(context.Background()))
	require.Empty(t, repo.entries)
	require.Empty(t, repo.sessions)
	require.Equal(t, int64(50), repo.checkpoint, "checkpoint still advances past skipped events")
}

func TestRunBatch_ReplayIsIdempotent(t *testing.T) {
	userID := uuid.New()
	ev := makeEvent(t, EventHomeItemOpened, 300, userID, map[string]any{"entry_key": "article:42"})

	repoA := &fakeRepo{events: []sovereign_db.KnowledgeEvent{ev}}
	require.NoError(t, newProjector(repoA).RunBatch(context.Background()))

	repoB := &fakeRepo{events: []sovereign_db.KnowledgeEvent{ev}}
	require.NoError(t, newProjector(repoB).RunBatch(context.Background()))

	require.Equal(t, len(repoA.entries), len(repoB.entries))
	require.Equal(t, repoA.entries[0].EntryKey, repoB.entries[0].EntryKey)
	require.Equal(t, repoA.entries[0].FreshnessAt.AsTime(), repoB.entries[0].FreshnessAt.AsTime(),
		"reproject must produce identical freshness_at from event.occurred_at")
	require.Equal(t, repoA.entries[0].WhyPrimary.Text, repoB.entries[0].WhyPrimary.Text,
		"reproject must produce identical why_text from event payload alone")
}

func TestSeedDecisionOptions_StageAppropriate(t *testing.T) {
	cases := []struct {
		stage   sovereignv1.LoopStage
		intents []string
	}{
		{sovereignv1.LoopStage_LOOP_STAGE_OBSERVE, []string{"revisit", "ask", "snooze"}},
		{sovereignv1.LoopStage_LOOP_STAGE_ORIENT, []string{"compare", "ask", "snooze"}},
		{sovereignv1.LoopStage_LOOP_STAGE_DECIDE, []string{"open", "save", "ask"}},
		{sovereignv1.LoopStage_LOOP_STAGE_ACT, []string{"revisit", "ask"}},
	}
	for _, tc := range cases {
		raw := seedDecisionOptions(tc.stage)
		require.NotEmpty(t, raw, "stage %s must produce a seed", tc.stage)
		var seeded []map[string]string
		require.NoError(t, json.Unmarshal(raw, &seeded))
		got := make([]string, 0, len(seeded))
		for _, s := range seeded {
			got = append(got, s["intent"])
		}
		require.Equal(t, tc.intents, got, "stage %s seed intents", tc.stage)
	}
}

func TestRunBatch_EmptyBatchIsNoOp(t *testing.T) {
	repo := &fakeRepo{}
	p := newProjector(repo)
	require.NoError(t, p.RunBatch(context.Background()))
	require.Empty(t, repo.entries)
	require.Empty(t, repo.sessions)
	require.Equal(t, int64(0), repo.checkpoint)
}
