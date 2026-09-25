package knowledge_trail_projector

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
	"knowledge-sovereign/usecase/trail_planner"
)

type recordingHandler struct {
	records []recordedLog
}

type recordedLog struct {
	Level   slog.Level
	Message string
	Attrs   map[string]any
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordingHandler) WithAttrs(_ []slog.Attr) slog.Handler     { return h }
func (h *recordingHandler) WithGroup(_ string) slog.Handler          { return h }
func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := map[string]any{}
	r.Attrs(func(a slog.Attr) bool { attrs[a.Key] = a.Value.Any(); return true })
	h.records = append(h.records, recordedLog{Level: r.Level, Message: r.Message, Attrs: attrs})
	return nil
}

func (h *recordingHandler) find(message string) (recordedLog, bool) {
	for _, r := range h.records {
		if r.Message == message {
			return r, true
		}
	}
	return recordedLog{}, false
}

type fakeRepo struct {
	events            []sovereign_db.KnowledgeEvent
	checkpoint        int64
	checkpointAt      time.Time
	checkpointExists  bool
	advances          []fakeAdvance
	advanceRejected   bool
	listCalls         int
	frontiers         []sovereign_db.SequenceGapFrontier
	beforeGapFrontier func()
	upserts           map[string]sovereign_db.TrailFootprint
	branches          map[string]sovereign_db.TrailBranch
	states            map[string]string
	outcomes          map[string]sovereign_db.TrailActOutcome
}

type fakeAdvance struct {
	From  sovereign_db.ProjectionCheckpoint
	ToSeq int64
}

var fakeCheckpointAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func newFakeRepo(events []sovereign_db.KnowledgeEvent) *fakeRepo {
	return &fakeRepo{
		events: events, checkpointAt: fakeCheckpointAt, checkpointExists: true,
		frontiers: []sovereign_db.SequenceGapFrontier{{Ceiling: 101, Xmin: 100}},
		upserts:   map[string]sovereign_db.TrailFootprint{},
		branches:  map[string]sovereign_db.TrailBranch{},
		states:    map[string]string{}, outcomes: map[string]sovereign_db.TrailActOutcome{},
	}
}

func (f *fakeRepo) InsertTrailActOutcome(_ context.Context, o sovereign_db.TrailActOutcome, _ int) error {
	if _, ok := f.outcomes[o.OutcomeKey]; !ok {
		f.outcomes[o.OutcomeKey] = o
	}
	return nil
}

func (f *fakeRepo) UpsertTrailBranch(_ context.Context, _, _ uuid.UUID, b sovereign_db.TrailBranch, _ time.Time, _ int) error {
	f.branches[b.BranchKey], f.states[b.BranchKey] = b, "open"
	return nil
}

func (f *fakeRepo) SetTrailBranchState(_ context.Context, _ uuid.UUID, k, s string) error {
	f.states[k] = s
	return nil
}

func (f *fakeRepo) ReadProjectionCheckpointForAdvance(_ context.Context, _ string) (sovereign_db.ProjectionCheckpoint, error) {
	if !f.checkpointExists {
		return sovereign_db.ProjectionCheckpoint{}, nil
	}
	return sovereign_db.ProjectionCheckpoint{LastEventSeq: f.checkpoint, UpdatedAt: f.checkpointAt, Exists: true}, nil
}

func (f *fakeRepo) UpsertTrailFootprint(_ context.Context, fp sovereign_db.TrailFootprint, _ int) error {
	f.upserts[fp.FootprintKey] = fp
	return nil
}

func (f *fakeRepo) AdvanceProjectionCheckpointIfUnchanged(_ context.Context, _ string, from sovereign_db.ProjectionCheckpoint, toSeq int64) (bool, error) {
	f.advances = append(f.advances, fakeAdvance{From: from, ToSeq: toSeq})
	if f.advanceRejected || from.Exists != f.checkpointExists || from.LastEventSeq != f.checkpoint || !from.UpdatedAt.Equal(f.checkpointAt) {
		return false, nil
	}
	f.checkpoint = toSeq
	f.checkpointAt = f.checkpointAt.Add(time.Second)
	f.checkpointExists = true
	return true, nil
}

func (f *fakeRepo) ListKnowledgeEventsSince(_ context.Context, afterSeq int64, limit int) ([]sovereign_db.KnowledgeEvent, error) {
	f.listCalls++
	var out []sovereign_db.KnowledgeEvent
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

func (f *fakeRepo) ReadSequenceGapFrontier(_ context.Context, firstSeq, lastSeq int64) (sovereign_db.SequenceGapFrontier, error) {
	if f.beforeGapFrontier != nil {
		f.beforeGapFrontier()
	}
	next := f.frontiers[0]
	if len(f.frontiers) > 1 {
		f.frontiers = f.frontiers[1:]
	}
	next.HoleOpen = true
	for _, evt := range f.events {
		if evt.EventSeq >= firstSeq && evt.EventSeq <= lastSeq {
			next.HoleOpen = false
		}
	}
	return next, nil
}

func userPtr() *uuid.UUID { u := uuid.New(); return &u }

func actEvent(seq int64, eventType, itemKey, dedupe string, at time.Time, user *uuid.UUID) sovereign_db.KnowledgeEvent {
	return sovereign_db.KnowledgeEvent{
		EventID: uuid.New(), EventSeq: seq, OccurredAt: at, TenantID: uuid.New(),
		UserID: user, EventType: eventType, AggregateID: itemKey, DedupeKey: dedupe,
	}
}

// ── Engine loop & Reproject tests ──

func TestProjector_ReprojectIsDeterministic(t *testing.T) {
	user := userPtr()
	base := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	proposed := validBranchPayload()
	events := []sovereign_db.KnowledgeEvent{
		actEvent(1, "HomeItemOpened", "article:a", "open:a", base, user),
		actEvent(2, "knowledge_loop.acted.v1", "article:c", "acted:c", base.Add(time.Minute), user),
		branchEvent(3, proposed, user),
		resolvedEvent(4, trail_planner.BranchResolvedPayload{BranchKey: proposed.BranchKey, Resolution: "taken"}, user),
	}

	first := newFakeRepo(events)
	require.NoError(t, NewProjector(first, nil, Config{}).RunBatch(context.Background()))
	second := newFakeRepo(events)
	require.NoError(t, NewProjector(second, nil, Config{}).RunBatch(context.Background()))

	assert.Equal(t, first.upserts, second.upserts)
	assert.Equal(t, first.branches, second.branches)
	assert.Equal(t, first.states, second.states)
	assert.Equal(t, "read", first.upserts["acted:c"].Verb)
}

func TestProjector_HighDensityReplayIsExactAtBatchBoundaries(t *testing.T) {
	user, base := userPtr(), time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	var events []sovereign_db.KnowledgeEvent
	seq := int64(0)
	next := func() int64 { seq++; return seq }
	const items = 1200
	for i := range items {
		item := fmt.Sprintf("article:%04d", i)
		at := base.Add(time.Duration(i) * time.Minute)
		events = append(events, actEvent(next(), "HomeItemOpened", item, "open:"+item, at, user))
		if i%3 == 0 {
			events = append(events, outcomeEvent(next(), "trail.act_outcome.v1", "b:"+item, "trail.act_outcome.v1:b:"+item,
				map[string]any{"branch_key": "b:" + item, "item_key": item, "dwell_ms": 31000}, user))
		}
		if i%5 == 0 {
			events = append(events, outcomeEvent(next(), "knowledge_loop.act_outcome.v1", "entry:"+item,
				"knowledge_loop.act_outcome.v1:entry:"+item+":default",
				map[string]any{"entry_key": item, "outcome": "no_engagement"}, user))
		}
	}
	run := func() *fakeRepo {
		repo := newFakeRepo(events)
		p := NewProjector(repo, nil, Config{BatchSize: 256, MaxBatchesPerTick: 1})
		for {
			before := repo.checkpoint
			require.NoError(t, p.RunBatch(context.Background()))
			if repo.checkpoint == before {
				break
			}
		}
		return repo
	}
	first, second := run(), run()
	assert.Equal(t, seq, first.checkpoint)
	assert.Len(t, first.upserts, items)
	assert.Len(t, first.outcomes, items/3+items/5)
	assert.Equal(t, first.upserts, second.upserts)
	assert.Equal(t, first.outcomes, second.outcomes)
}

// ── Checkpoint advance & CAS tests ──

func TestProjector_AdvancesTheCheckpointWithTheStateItReadAtTheStartOfTheBatch(t *testing.T) {
	user := userPtr()
	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		actEvent(11, "HomeItemOpened", "article:a", "open:a", fakeCheckpointAt, user),
	})
	repo.checkpoint = 10
	repo.checkpointExists = true
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	require.Len(t, repo.advances, 1)
	assert.True(t, repo.advances[0].From.Exists)
	assert.Equal(t, int64(10), repo.advances[0].From.LastEventSeq)
	assert.Equal(t, fakeCheckpointAt, repo.advances[0].From.UpdatedAt)
	assert.Equal(t, int64(11), repo.advances[0].ToSeq)
	assert.Equal(t, int64(11), repo.checkpoint)
}

func TestProjector_RejectedCheckpointAdvanceStopsTheTickAndIsReportedLoudly(t *testing.T) {
	user := userPtr()
	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		actEvent(1, "HomeItemOpened", "article:a", "open:a", fakeCheckpointAt, user),
		actEvent(2, "HomeItemOpened", "article:b", "open:b", fakeCheckpointAt, user),
	})
	repo.advanceRejected = true
	logs := &recordingHandler{}
	p := NewProjector(repo, slog.New(logs), Config{BatchSize: 1, MaxBatchesPerTick: 2})

	require.NoError(t, p.RunBatch(context.Background()))
	require.Len(t, repo.advances, 1)
	assert.Equal(t, int64(0), repo.checkpoint)
	assert.Equal(t, 1, repo.listCalls)
	rec, ok := logs.find("trail.checkpoint_advance_rejected")
	require.True(t, ok)
	assert.Equal(t, slog.LevelError, rec.Level)
	assert.Equal(t, projectorName, rec.Attrs["projector"])
	assert.Equal(t, int64(0), rec.Attrs["expected_seq"])
	assert.Equal(t, int64(1), rec.Attrs["attempted_seq"])
}

// ── Gap tests ──

func TestProjector_StopsAtASequenceGapLeftByAnUncommittedTransaction(t *testing.T) {
	user := userPtr()
	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		actEvent(2, "HomeItemOpened", "article:b", "open:b", fakeCheckpointAt, user),
	})
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))
	assert.Zero(t, repo.checkpoint, "the batch must stop before the gap; checkpoint must not advance")
	assert.Empty(t, repo.upserts, "an event beyond the gap must not be folded prematurely")

	repo.events = append([]sovereign_db.KnowledgeEvent{
		actEvent(1, "HomeItemOpened", "article:a", "open:a", fakeCheckpointAt, user),
	}, repo.events...)

	require.NoError(t, p.RunBatch(context.Background()))
	assert.EqualValues(t, 2, repo.checkpoint, "once the gap is filled the batch folds through to the tip")
	assert.Len(t, repo.upserts, 2, "both footprints must reach the spine")
}

func TestProjector_StepsPastASequenceBurnedByARolledBackTransaction(t *testing.T) {
	user := userPtr()
	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		actEvent(2, "HomeItemOpened", "article:b", "d-2", fakeCheckpointAt, user),
	})
	repo.frontiers = []sovereign_db.SequenceGapFrontier{
		{Ceiling: 101, Xmin: 100},
		{Ceiling: 140, Xmin: 101},
	}
	logs := &recordingHandler{}
	p := NewProjector(repo, slog.New(logs), Config{})

	require.NoError(t, p.RunBatch(context.Background()))
	assert.Zero(t, repo.checkpoint, "the hole is still fillable on this tick")

	require.NoError(t, p.RunBatch(context.Background()))
	assert.EqualValues(t, 2, repo.checkpoint, "a sequence no live transaction can still commit must not wedge the projection")
	assert.Len(t, repo.upserts, 1, "the events past the burned sequence must be folded")

	rec, ok := logs.find("trail.sequence_gap_abandoned")
	require.True(t, ok, "abandoning a sequence must be reported, never silent")
	assert.Equal(t, slog.LevelWarn, rec.Level)
	assert.EqualValues(t, 1, rec.Attrs["gap_seq"])
}

func TestProjector_WaitsWhileTheTransactionHoldingTheMissingSequenceIsInFlight(t *testing.T) {
	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		actEvent(2, "HomeItemOpened", "article:b", "d-2", fakeCheckpointAt, userPtr()),
	})
	p := NewProjector(repo, nil, Config{})
	for i := 0; i < 3; i++ {
		require.NoError(t, p.RunBatch(context.Background()))
	}
	assert.Zero(t, repo.checkpoint, "an in-flight writer still owns seq 1; the checkpoint must stay behind it")
	assert.Empty(t, repo.upserts)
}

func TestProjector_FoldsUpToAMidBatchSequenceGapAndResumesAcrossIt(t *testing.T) {
	user := userPtr()
	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		actEvent(1, "HomeItemOpened", "article:a", "d-1", fakeCheckpointAt, user),
		actEvent(2, "HomeItemOpened", "article:b", "d-2", fakeCheckpointAt, user),
		actEvent(4, "HomeItemOpened", "article:d", "d-4", fakeCheckpointAt, user),
	})
	p := NewProjector(repo, nil, Config{})

	require.NoError(t, p.RunBatch(context.Background()))
	assert.EqualValues(t, 2, repo.checkpoint, "the checkpoint must stop one short of the hole")
	assert.Len(t, repo.upserts, 2)

	repo.events = append(repo.events[:2], append([]sovereign_db.KnowledgeEvent{
		actEvent(3, "HomeItemOpened", "article:c", "d-3", fakeCheckpointAt, user),
	}, repo.events[2:]...)...)

	require.NoError(t, p.RunBatch(context.Background()))
	assert.EqualValues(t, 4, repo.checkpoint)
	assert.Len(t, repo.upserts, 4, "the event that was in flight must reach the spine")
}

func TestProjector_StepsPastAWholeRunOfBurnedSequencesAtOnce(t *testing.T) {
	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		actEvent(4, "HomeItemOpened", "article:d", "d-4", fakeCheckpointAt, userPtr()),
	})
	repo.frontiers = []sovereign_db.SequenceGapFrontier{
		{Ceiling: 101, Xmin: 100},
		{Ceiling: 140, Xmin: 101},
	}
	logs := &recordingHandler{}
	p := NewProjector(repo, slog.New(logs), Config{})

	require.NoError(t, p.RunBatch(context.Background()))
	require.NoError(t, p.RunBatch(context.Background()))

	assert.EqualValues(t, 4, repo.checkpoint)
	assert.Len(t, repo.upserts, 1)

	rec, ok := logs.find("trail.sequence_gap_abandoned")
	require.True(t, ok)
	assert.EqualValues(t, 1, rec.Attrs["gap_seq"])
	assert.EqualValues(t, 3, rec.Attrs["gap_through"], "the whole burned run is reported, not just its head")
}

func TestProjector_NeverStepsOverASequenceThatCommittedAfterTheBatchWasRead(t *testing.T) {
	user := userPtr()
	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		actEvent(2, "HomeItemOpened", "article:b", "d-2", fakeCheckpointAt, user),
	})
	repo.frontiers = []sovereign_db.SequenceGapFrontier{
		{Ceiling: 101, Xmin: 100},
		{Ceiling: 140, Xmin: 101},
	}
	p := NewProjector(repo, nil, Config{})

	require.NoError(t, p.RunBatch(context.Background()))
	require.Zero(t, repo.checkpoint, "the hole is still fillable on this tick")

	repo.beforeGapFrontier = func() {
		repo.beforeGapFrontier = nil
		repo.events = append([]sovereign_db.KnowledgeEvent{
			actEvent(1, "HomeItemOpened", "article:a", "d-1", fakeCheckpointAt, user),
		}, repo.events...)
	}

	require.NoError(t, p.RunBatch(context.Background()))
	assert.Zero(t, repo.checkpoint, "the sequence arrived; the checkpoint must not step over it")

	require.NoError(t, p.RunBatch(context.Background()))
	assert.EqualValues(t, 2, repo.checkpoint)
	assert.Len(t, repo.upserts, 2, "the event that committed mid-tick must still reach the spine")
}
