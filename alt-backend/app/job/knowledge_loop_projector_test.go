package job

import (
	"alt/domain"
	"alt/port/knowledge_loop_port"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// fakeLoopRepo is a minimal in-memory recorder that captures the sequence of upserts.
// It does NOT simulate DB-level seq_hiwater guard; the guard is exercised separately at the
// driver layer. Here we verify the projector emits the same upsert payload for the same event
// on replay — a reproject-safety invariant.
type fakeLoopRepo struct {
	entries []domain.KnowledgeLoopEntry
	session []domain.KnowledgeLoopSessionState
	surface []domain.KnowledgeLoopSurface
}

func (f *fakeLoopRepo) UpsertKnowledgeLoopEntry(ctx context.Context, entry *domain.KnowledgeLoopEntry) (*knowledge_loop_port.UpsertResult, error) {
	f.entries = append(f.entries, *entry)
	return &knowledge_loop_port.UpsertResult{Applied: true, ProjectionRevision: 1, ProjectionSeqHiwater: entry.ProjectionSeqHiwater}, nil
}

func (f *fakeLoopRepo) UpsertKnowledgeLoopSessionState(ctx context.Context, state *domain.KnowledgeLoopSessionState) (*knowledge_loop_port.UpsertResult, error) {
	f.session = append(f.session, *state)
	return &knowledge_loop_port.UpsertResult{Applied: true, ProjectionRevision: 1, ProjectionSeqHiwater: state.ProjectionSeqHiwater}, nil
}

func (f *fakeLoopRepo) UpsertKnowledgeLoopSurface(ctx context.Context, s *domain.KnowledgeLoopSurface) (*knowledge_loop_port.UpsertResult, error) {
	f.surface = append(f.surface, *s)
	return &knowledge_loop_port.UpsertResult{Applied: true, ProjectionRevision: 1, ProjectionSeqHiwater: s.ProjectionSeqHiwater}, nil
}

func makeLoopEvent(t *testing.T, eventType string, seq int64, userID uuid.UUID) *domain.KnowledgeEvent {
	t.Helper()
	payload, _ := json.Marshal(map[string]string{
		"entry_key": "article:42",
	})
	return &domain.KnowledgeEvent{
		EventID:       uuid.New(),
		EventSeq:      seq,
		OccurredAt:    time.Date(2026, 4, 23, 10, 0, 0, 0, time.UTC),
		TenantID:      uuid.New(),
		UserID:        &userID,
		EventType:     eventType,
		AggregateType: "article",
		AggregateID:   "article:42",
		DedupeKey:     eventType + ":" + uuid.NewString(),
		Payload:       payload,
	}
}

// TestProjectLoopEvent_HomeItemOpened exercises the main projection path.
func TestProjectLoopEvent_HomeItemOpened(t *testing.T) {
	repo := &fakeLoopRepo{}
	userID := uuid.New()
	ev := makeLoopEvent(t, domain.EventHomeItemOpened, 100, userID)

	res, err := projectLoopEvent(context.Background(), ev, repo, repo, repo)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.Applied)

	require.Len(t, repo.entries, 1, "one entry upsert expected")
	require.Len(t, repo.session, 1, "one session upsert expected")

	entry := repo.entries[0]
	require.Equal(t, domain.LoopStageAct, entry.ProposedStage)
	require.Equal(t, domain.SurfaceContinue, entry.SurfaceBucket)
	require.Equal(t, domain.DismissCompleted, entry.DismissState)
	require.Equal(t, ev.OccurredAt, entry.FreshnessAt, "freshness_at must come from event occurred_at, not wall-clock")

	state := repo.session[0]
	require.Equal(t, domain.LoopStageAct, state.CurrentStage)
	require.Equal(t, ev.OccurredAt, state.CurrentStageEnteredAt,
		"current_stage_entered_at must come from event occurred_at, not wall-clock (reproject-safety)")
}

// TestProjectLoopEvent_ReplayIsIdempotent verifies that projecting the same event twice
// produces identical payloads. This is the core reproject-safety invariant: replaying
// the event log must converge to the same projection state regardless of when it runs.
func TestProjectLoopEvent_ReplayIsIdempotent(t *testing.T) {
	userID := uuid.New()
	ev := makeLoopEvent(t, domain.EventHomeItemOpened, 200, userID)

	repoA := &fakeLoopRepo{}
	_, errA := projectLoopEvent(context.Background(), ev, repoA, repoA, repoA)
	require.NoError(t, errA)

	repoB := &fakeLoopRepo{}
	_, errB := projectLoopEvent(context.Background(), ev, repoB, repoB, repoB)
	require.NoError(t, errB)

	require.Equal(t, repoA.entries, repoB.entries,
		"projecting the same event twice must yield identical entry upserts")
	require.Equal(t, repoA.session, repoB.session,
		"projecting the same event twice must yield identical session upserts")
}

// TestProjectLoopEvent_NoUserIDIsNoOp guards against panicking on system-level events
// that lack a user_id (e.g. ArticleCreated). They should be a no-op in this projector.
func TestProjectLoopEvent_NoUserIDIsNoOp(t *testing.T) {
	ev := makeLoopEvent(t, domain.EventArticleCreated, 50, uuid.Nil)
	ev.UserID = nil

	repo := &fakeLoopRepo{}
	res, err := projectLoopEvent(context.Background(), ev, repo, repo, repo)
	require.NoError(t, err)
	require.Nil(t, res)
	require.Empty(t, repo.entries)
	require.Empty(t, repo.session)
}

// TestProjectLoopEvent_HomeItemSupersededSetsPointer checks supersede pointer handling.
func TestProjectLoopEvent_HomeItemSupersededSetsPointer(t *testing.T) {
	repo := &fakeLoopRepo{}
	userID := uuid.New()
	payload, _ := json.Marshal(map[string]string{
		"entry_key":     "article:42",
		"new_entry_key": "article:43",
	})
	ev := makeLoopEvent(t, domain.EventHomeItemSuperseded, 300, userID)
	ev.Payload = payload

	_, err := projectLoopEvent(context.Background(), ev, repo, repo, repo)
	require.NoError(t, err)
	require.Len(t, repo.entries, 1)
	require.NotNil(t, repo.entries[0].SupersededByEntryKey)
	require.Equal(t, "article:43", *repo.entries[0].SupersededByEntryKey)
}
