package reading_projector

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

func queuedReadingEvt(t *testing.T, seq int64, t0 time.Time, tenantID, userID, articleID uuid.UUID) sovereign_db.KnowledgeEvent {
	t.Helper()
	return readingEvt(t, seq, eventReadingQueued, t0.Add(time.Duration(seq)*time.Second), tenantID, userID, "article:"+articleID.String(), map[string]any{
		"article_id":           articleID.String(),
		"queued_at":            t0.Format(time.RFC3339),
		"expires_at":           t0.Add(48 * time.Hour).Format(time.RFC3339),
		"queue_policy_version": 1,
	})
}

func TestRunBatch_CrossesBurnedHolesWithinOneTick(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	t0 := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)

	const numHoles = 15
	const runLen = 2

	var events []sovereign_db.KnowledgeEvent
	var expectedSeqs []int64
	seq := int64(1)
	for r := 0; r <= numHoles; r++ {
		for j := 0; j < runLen; j++ {
			events = append(events, queuedReadingEvt(t, seq, t0, tenantID, userID, uuid.New()))
			expectedSeqs = append(expectedSeqs, seq)
			seq++
		}
		if r < numHoles {
			// Skip one sequence to represent a burned hole.
			seq++
		}
	}
	headSeq := expectedSeqs[len(expectedSeqs)-1]

	repo := newFakeReadingRepo(events)
	p := NewProjector(repo, nil, Config{MaxBatchesPerTick: 40})

	require.NoError(t, p.RunBatch(context.Background()))

	assert.Equal(t, headSeq, repo.checkpoint.LastEventSeq)
	assert.Equal(t, expectedSeqs, repo.foldedQueueSeqs)
	assert.Len(t, repo.frontierReads, numHoles*2, "expected two frontier reads per burned hole")
	for i := 1; i < len(repo.frontierReads); i++ {
		assert.Greater(t, repo.frontierReads[i].Frontier.Ceiling, repo.frontierReads[i-1].Frontier.Ceiling)
	}
}

func TestRunBatch_ManyBurnedHolesNeedBoundedTicks(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	t0 := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)

	const numHoles = 1000
	events := make([]sovereign_db.KnowledgeEvent, 0, numHoles+1)
	expectedSeqs := make([]int64, 0, numHoles+1)
	for i := 0; i <= numHoles; i++ {
		seq := int64(i*2 + 1)
		events = append(events, queuedReadingEvt(t, seq, t0, tenantID, userID, uuid.New()))
		expectedSeqs = append(expectedSeqs, seq)
	}
	headSeq := expectedSeqs[len(expectedSeqs)-1]

	repo := newFakeReadingRepo(events)
	p := NewProjector(repo, nil, Config{MaxBatchesPerTick: 40})

	calls := 0
	const safetyCap = 5000
	for repo.checkpoint.LastEventSeq < headSeq && calls < safetyCap {
		require.NoError(t, p.RunBatch(context.Background()))
		calls++
	}

	require.Equal(t, headSeq, repo.checkpoint.LastEventSeq)
	assert.LessOrEqual(t, calls, 55)
	assert.Equal(t, expectedSeqs, repo.foldedQueueSeqs)
}

func TestRunBatch_HoleHeldByRunningWriterIsNotAbandoned(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	t0 := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)

	aboveArt := uuid.New()
	repo := newFakeReadingRepo([]sovereign_db.KnowledgeEvent{
		queuedReadingEvt(t, 2, t0, tenantID, userID, aboveArt),
	})
	writerXID := repo.startWriter()

	p := NewProjector(repo, nil, Config{MaxBatchesPerTick: 40})

	// While an older writer runs (Xmin < Ceiling), each tick reads the frontier once and stops.
	for tick := 1; tick <= 4; tick++ {
		require.NoError(t, p.RunBatch(context.Background()))
		assert.Zero(t, repo.checkpoint.LastEventSeq)
		assert.Empty(t, repo.foldedQueueSeqs)
		require.Len(t, repo.frontierReads, tick)
		last := repo.frontierReads[tick-1].Frontier
		assert.Equal(t, writerXID, last.Xmin)
		assert.Less(t, last.Xmin, last.Ceiling)
	}

	// Writer commits inside the hole (seq 1).
	holeArt := uuid.New()
	repo.insertEventSorted(queuedReadingEvt(t, 1, t0, tenantID, userID, holeArt))
	repo.runningWriterXID = 0

	require.NoError(t, p.RunBatch(context.Background()))
	assert.Equal(t, int64(2), repo.checkpoint.LastEventSeq)
	assert.Equal(t, []int64{1, 2}, repo.foldedQueueSeqs)
	assert.Contains(t, repo.queue, queueKey{userID: userID, articleID: holeArt})
	assert.Contains(t, repo.queue, queueKey{userID: userID, articleID: aboveArt})
}

func TestRunBatch_HoleFilledBeforeTheSecondReadIsNotAbandoned(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	t0 := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)

	holeArt := uuid.New()
	aboveArt := uuid.New()
	repo := newFakeReadingRepo([]sovereign_db.KnowledgeEvent{
		queuedReadingEvt(t, 2, t0, tenantID, userID, aboveArt),
	})

	// No running writer: first read sees HoleOpen && Xmin == Ceiling, triggering a second read
	// in the same tick; an event arrives into seq 1 right before that second read.
	repo.beforeFrontierRead = func(readIndex int, _, _ int64) {
		if readIndex == 2 {
			repo.insertEventSorted(queuedReadingEvt(t, 1, t0, tenantID, userID, holeArt))
		}
	}

	p := NewProjector(repo, nil, Config{MaxBatchesPerTick: 40})

	require.NoError(t, p.RunBatch(context.Background()))
	assert.Zero(t, repo.checkpoint.LastEventSeq, "must not advance past the hole when the second read sees it filled")
	assert.Len(t, repo.frontierReads, 2)
	if len(repo.frontierReads) >= 1 {
		f1 := repo.frontierReads[0].Frontier
		assert.True(t, f1.HoleOpen)
		assert.Equal(t, f1.Ceiling, f1.Xmin)
	}
	if len(repo.frontierReads) >= 2 {
		assert.False(t, repo.frontierReads[1].Frontier.HoleOpen)
	}
	assert.Empty(t, repo.foldedQueueSeqs)

	require.NoError(t, p.RunBatch(context.Background()))
	assert.Equal(t, int64(2), repo.checkpoint.LastEventSeq)
	assert.Equal(t, []int64{1, 2}, repo.foldedQueueSeqs)
}

func TestRunBatch_HoleFreedByRollbackIsAbandonedOnTheNextTick(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	t0 := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)

	repo := newFakeReadingRepo([]sovereign_db.KnowledgeEvent{
		queuedReadingEvt(t, 2, t0, tenantID, userID, uuid.New()),
	})
	repo.startWriter()

	logs := &recordingLogHandler{}
	p := NewProjector(repo, slog.New(logs), Config{MaxBatchesPerTick: 40})

	// Tick 1: writer is running (Xmin < Ceiling) -> single read, no advance; Tracker records first sighting.
	require.NoError(t, p.RunBatch(context.Background()))
	assert.Zero(t, repo.checkpoint.LastEventSeq)
	assert.Empty(t, repo.foldedQueueSeqs)
	require.Len(t, repo.frontierReads, 1)
	assert.Less(t, repo.frontierReads[0].Frontier.Xmin, repo.frontierReads[0].Frontier.Ceiling)

	// Writer rolls back: no transaction running, hole stays empty.
	repo.runningWriterXID = 0

	// Tick 2: Tracker already holds the first sighting from tick 1, so the single read in tick 2
	// sees Xmin >= firstCeiling with HoleOpen == true, abandons the hole, and RunBatch folds seq 2.
	require.NoError(t, p.RunBatch(context.Background()))
	assert.Len(t, repo.frontierReads, 2)
	assert.Equal(t, int64(2), repo.checkpoint.LastEventSeq)
	assert.Equal(t, []int64{2}, repo.foldedQueueSeqs)
	assert.Equal(t, 1, logs.count("reading_projector.sequence_gap_abandoned"))
}

func TestRunBatch_AdvanceRejectedWhileCrossingHolesEndsTheTick(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	t0 := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)

	repo := newFakeReadingRepo([]sovereign_db.KnowledgeEvent{
		queuedReadingEvt(t, 1, t0, tenantID, userID, uuid.New()),
		// seq 2 is a burned hole
		queuedReadingEvt(t, 3, t0, tenantID, userID, uuid.New()),
		// seq 4 is a burned hole
		queuedReadingEvt(t, 5, t0, tenantID, userID, uuid.New()),
	})

	// Reject the first CAS attempt to abandon hole seq 4; must end the tick without retrying or folding seq 5.
	rejectedOnce := false
	repo.beforeAdvance = func(_ sovereign_db.ProjectionCheckpoint, toSeq int64) bool {
		if !rejectedOnce && toSeq == 4 {
			rejectedOnce = true
			return false
		}
		return true
	}

	logs := &recordingLogHandler{}
	p := NewProjector(repo, slog.New(logs), Config{MaxBatchesPerTick: 40})

	require.NoError(t, p.RunBatch(context.Background()))

	assert.True(t, rejectedOnce, "expected CAS rejection to be reached on the middle hole")
	assert.Equal(t, int64(3), repo.checkpoint.LastEventSeq)
	assert.Equal(t, []int64{1, 3}, repo.foldedQueueSeqs)
	assert.Equal(t, 1, logs.count("reading_projector.checkpoint_advance_rejected"))
}
