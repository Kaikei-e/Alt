package reading_projector

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

type queueKey struct {
	userID    uuid.UUID
	articleID uuid.UUID
}

type fakeQueueRow struct {
	item          sovereign_db.ReadingQueueItem
	removedAt     *time.Time
	removedReason string
}

type frontierReadRecord struct {
	FirstSeq int64
	LastSeq  int64
	Frontier sovereign_db.SequenceGapFrontier
}

type fakeReadingRepo struct {
	events             []sovereign_db.KnowledgeEvent
	checkpoint         sovereign_db.ProjectionCheckpoint
	sources            map[queueKey]sovereign_db.ReadingSource
	queue              map[queueKey]fakeQueueRow
	nextXID            int64
	runningWriterXID   int64
	frontierReads      []frontierReadRecord
	beforeFrontierRead func(readIndex int, firstSeq, lastSeq int64)
	beforeAdvance      func(from sovereign_db.ProjectionCheckpoint, toSeq int64) bool
	foldedQueueSeqs    []int64
}

func newFakeReadingRepo(events []sovereign_db.KnowledgeEvent) *fakeReadingRepo {
	return &fakeReadingRepo{
		events:  events,
		sources: make(map[queueKey]sovereign_db.ReadingSource),
		queue:   make(map[queueKey]fakeQueueRow),
		nextXID: 100,
	}
}

func (r *fakeReadingRepo) allocXID() int64 {
	if r.nextXID == 0 {
		r.nextXID = 100
	}
	r.nextXID++
	return r.nextXID
}

func (r *fakeReadingRepo) startWriter() int64 {
	w := r.allocXID()
	r.runningWriterXID = w
	return w
}

func (r *fakeReadingRepo) insertEventSorted(ev sovereign_db.KnowledgeEvent) {
	idx := sort.Search(len(r.events), func(i int) bool {
		return r.events[i].EventSeq >= ev.EventSeq
	})
	r.events = append(r.events, sovereign_db.KnowledgeEvent{})
	copy(r.events[idx+1:], r.events[idx:])
	r.events[idx] = ev
}

func (r *fakeReadingRepo) hasEventInRange(firstSeq, lastSeq int64) bool {
	idx := sort.Search(len(r.events), func(i int) bool {
		return r.events[i].EventSeq >= firstSeq
	})
	return idx < len(r.events) && r.events[idx].EventSeq <= lastSeq
}

func (r *fakeReadingRepo) ReadProjectionCheckpointForAdvance(_ context.Context, _ string) (sovereign_db.ProjectionCheckpoint, error) {
	return r.checkpoint, nil
}

func (r *fakeReadingRepo) AdvanceProjectionCheckpointIfUnchanged(_ context.Context, _ string, from sovereign_db.ProjectionCheckpoint, toSeq int64) (bool, error) {
	if r.beforeAdvance != nil && !r.beforeAdvance(from, toSeq) {
		return false, nil
	}
	if r.checkpoint.LastEventSeq != from.LastEventSeq {
		return false, nil
	}
	r.checkpoint.LastEventSeq = toSeq
	r.checkpoint.Exists = true
	return true, nil
}

func (r *fakeReadingRepo) ListKnowledgeEventsSince(_ context.Context, afterSeq int64, limit int) ([]sovereign_db.KnowledgeEvent, error) {
	start := sort.Search(len(r.events), func(i int) bool {
		return r.events[i].EventSeq > afterSeq
	})
	if start >= len(r.events) {
		return nil, nil
	}
	end := start + limit
	if end > len(r.events) {
		end = len(r.events)
	}
	out := make([]sovereign_db.KnowledgeEvent, end-start)
	copy(out, r.events[start:end])
	return out, nil
}

func (r *fakeReadingRepo) ReadSequenceGapFrontier(_ context.Context, firstSeq, lastSeq int64) (sovereign_db.SequenceGapFrontier, error) {
	readIdx := len(r.frontierReads) + 1
	if r.beforeFrontierRead != nil {
		r.beforeFrontierRead(readIdx, firstSeq, lastSeq)
	}
	ceiling := r.allocXID()
	xmin := ceiling
	if r.runningWriterXID > 0 {
		xmin = r.runningWriterXID
	}
	f := sovereign_db.SequenceGapFrontier{
		Ceiling:  ceiling,
		Xmin:     xmin,
		HoleOpen: !r.hasEventInRange(firstSeq, lastSeq),
	}
	r.frontierReads = append(r.frontierReads, frontierReadRecord{
		FirstSeq: firstSeq,
		LastSeq:  lastSeq,
		Frontier: f,
	})
	return f, nil
}

func (r *fakeReadingRepo) UpsertReadingSource(_ context.Context, src sovereign_db.ReadingSource) error {
	k := queueKey{userID: src.UserID, articleID: src.ArticleID}
	if cur, ok := r.sources[k]; ok {
		switch {
		case src.ReadRuleVersion > cur.ReadRuleVersion:
			// Incoming higher rule version replaces both first_read_at and version.
		case src.ReadRuleVersion < cur.ReadRuleVersion:
			return nil
		default:
			if cur.FirstReadAt.Before(src.FirstReadAt) {
				src.FirstReadAt = cur.FirstReadAt
			}
		}
	}
	r.sources[k] = src
	return nil
}

func (r *fakeReadingRepo) UpsertReadingQueueItem(_ context.Context, item sovereign_db.ReadingQueueItem) error {
	r.foldedQueueSeqs = append(r.foldedQueueSeqs, item.LastEventSeq)
	k := queueKey{userID: item.UserID, articleID: item.ArticleID}
	if cur, ok := r.queue[k]; ok {
		if cur.item.LastEventSeq > item.LastEventSeq {
			return nil
		}
		if cur.removedAt == nil && cur.item.ExpiresAt.After(item.QueuedAt) {
			cur.item.LastEventSeq = item.LastEventSeq
			r.queue[k] = cur
			return nil
		}
	}
	r.queue[k] = fakeQueueRow{item: item}
	return nil
}

func (r *fakeReadingRepo) RemoveReadingQueueItem(_ context.Context, userID, articleID uuid.UUID, removedAt time.Time, reason string, eventSeq int64) error {
	k := queueKey{userID: userID, articleID: articleID}
	cur, ok := r.queue[k]
	if !ok || cur.removedAt != nil || cur.item.LastEventSeq > eventSeq {
		return nil
	}
	t := removedAt
	cur.removedAt = &t
	cur.removedReason = reason
	cur.item.LastEventSeq = eventSeq
	r.queue[k] = cur
	return nil
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func readingEvt(t *testing.T, seq int64, eventType string, occurredAt time.Time, tenantID, userID uuid.UUID, aggregateID string, payload any) sovereign_db.KnowledgeEvent {
	t.Helper()
	u := userID
	return sovereign_db.KnowledgeEvent{
		EventID:     uuid.New(),
		EventSeq:    seq,
		OccurredAt:  occurredAt,
		TenantID:    tenantID,
		UserID:      &u,
		EventType:   eventType,
		AggregateID: aggregateID,
		Payload:     mustJSON(t, payload),
	}
}

func TestReadingProjector_FoldsSessionQueuedUnqueuedAndLegacyOpen(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	artRead := uuid.New()
	artSkimmed := uuid.New()
	artLegacy := uuid.New()
	artQueued := uuid.New()

	t0 := time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC)
	events := []sovereign_db.KnowledgeEvent{
		// 1. Queued article
		readingEvt(t, 1, eventReadingQueued, t0, tenantID, userID, "article:"+artRead.String(), map[string]any{
			"article_id":           artRead.String(),
			"queued_at":            t0.Format(time.RFC3339),
			"expires_at":           t0.Add(48 * time.Hour).Format(time.RFC3339),
			"queue_policy_version": 1,
		}),
		// 2. Second queued article (will be explicitly unqueued)
		readingEvt(t, 2, eventReadingQueued, t0.Add(time.Minute), tenantID, userID, "article:"+artQueued.String(), map[string]any{
			"article_id":           artQueued.String(),
			"queued_at":            t0.Add(time.Minute).Format(time.RFC3339),
			"expires_at":           t0.Add(time.Minute + 48*time.Hour).Format(time.RFC3339),
			"queue_policy_version": 1,
		}),
		// 3. Skimmed session (does NOT count as read -> no reading_source, stays in queue if queued)
		readingEvt(t, 3, eventSessionEnded, t0.Add(2*time.Minute), tenantID, userID, "article:"+artSkimmed.String(), map[string]any{
			"article_id":        artSkimmed.String(),
			"active_dwell_ms":   2000,
			"body_length_chars": 1500,
			"reached_end":       true,
		}),
		// 4. Qualifying session on artRead -> inserts reading_source v1 and removes from queue
		readingEvt(t, 4, eventSessionEnded, t0.Add(3*time.Minute), tenantID, userID, "article:"+artRead.String(), map[string]any{
			"article_id":        artRead.String(),
			"active_dwell_ms":   6000,
			"body_length_chars": 1500,
			"reached_end":       true,
		}),
		// 5. Explicit unqueue on artQueued
		readingEvt(t, 5, eventReadingUnqueue, t0.Add(4*time.Minute), tenantID, userID, "article:"+artQueued.String(), map[string]any{
			"article_id": artQueued.String(),
		}),
		// 6. Legacy HomeItemOpened on artLegacy -> inserts reading_source with LegacyReadRuleVersion (0)
		readingEvt(t, 6, eventHomeItemOpened, t0.Add(-24*time.Hour), tenantID, userID, "article:"+artLegacy.String(), map[string]any{
			"item_key": "article:" + artLegacy.String(),
		}),
		// 7. Re-queue artQueued at seq 7 -> becomes active again
		readingEvt(t, 7, eventReadingQueued, t0.Add(5*time.Minute), tenantID, userID, "article:"+artQueued.String(), map[string]any{
			"article_id":           artQueued.String(),
			"queued_at":            t0.Add(5 * time.Minute).Format(time.RFC3339),
			"expires_at":           t0.Add(5*time.Minute + 48*time.Hour).Format(time.RFC3339),
			"queue_policy_version": 1,
		}),
	}

	repo := newFakeReadingRepo(events)
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	assert.Equal(t, int64(7), repo.checkpoint.LastEventSeq)

	// artRead counted as read (version 1) and left the queue
	srcRead, ok := repo.sources[queueKey{userID: userID, articleID: artRead}]
	require.True(t, ok)
	assert.Equal(t, ReadRuleVersionV1, srcRead.ReadRuleVersion)
	assert.Equal(t, t0.Add(3*time.Minute), srcRead.FirstReadAt)

	qRead := repo.queue[queueKey{userID: userID, articleID: artRead}]
	require.NotNil(t, qRead.removedAt)
	assert.Equal(t, "read", qRead.removedReason)

	// artSkimmed did not count as read
	_, ok = repo.sources[queueKey{userID: userID, articleID: artSkimmed}]
	assert.False(t, ok)

	// artLegacy recorded with legacy rule version 0
	srcLegacy, ok := repo.sources[queueKey{userID: userID, articleID: artLegacy}]
	require.True(t, ok)
	assert.Equal(t, LegacyReadRuleVersion, srcLegacy.ReadRuleVersion)

	// artQueued was re-queued at seq 7 and is active
	qRequeued := repo.queue[queueKey{userID: userID, articleID: artQueued}]
	assert.Nil(t, qRequeued.removedAt)
	assert.Equal(t, t0.Add(5*time.Minute), qRequeued.item.QueuedAt)
}

func TestReadingProjector_FoldQueueMergeSemantics(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	t1 := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	exp1 := t1.Add(48 * time.Hour)
	t2 := t1.Add(2 * time.Hour)
	exp2 := t2.Add(48 * time.Hour)
	tAfterExpiry := exp1.Add(time.Hour)
	expAfterExpiry := tAfterExpiry.Add(48 * time.Hour)

	queuedEvt := func(seq int64, articleID uuid.UUID, queuedAt, expiresAt time.Time, policyVersion int) sovereign_db.KnowledgeEvent {
		return readingEvt(t, seq, eventReadingQueued, queuedAt, tenantID, userID, "article:"+articleID.String(), map[string]any{
			"article_id":           articleID.String(),
			"queued_at":            queuedAt.Format(time.RFC3339),
			"expires_at":           expiresAt.Format(time.RFC3339),
			"queue_policy_version": policyVersion,
		})
	}
	unqueuedEvt := func(seq int64, articleID uuid.UUID, occurredAt time.Time) sovereign_db.KnowledgeEvent {
		return readingEvt(t, seq, eventReadingUnqueue, occurredAt, tenantID, userID, "article:"+articleID.String(), map[string]any{
			"article_id": articleID.String(),
		})
	}

	tests := []struct {
		name              string
		buildEvents       func(articleID uuid.UUID) []sovereign_db.KnowledgeEvent
		wantQueuedAt      time.Time
		wantExpiresAt     time.Time
		wantPolicyVersion int
		wantLastSeq       int64
	}{
		{
			name: "queued then queued again while active keeps original times",
			buildEvents: func(articleID uuid.UUID) []sovereign_db.KnowledgeEvent {
				return []sovereign_db.KnowledgeEvent{
					queuedEvt(1, articleID, t1, exp1, 1),
					queuedEvt(2, articleID, t2, exp2, 2),
				}
			},
			wantQueuedAt:      t1,
			wantExpiresAt:     exp1,
			wantPolicyVersion: 1,
			wantLastSeq:       2,
		},
		{
			name: "queued then unqueued then queued re-activates with third event times",
			buildEvents: func(articleID uuid.UUID) []sovereign_db.KnowledgeEvent {
				return []sovereign_db.KnowledgeEvent{
					queuedEvt(1, articleID, t1, exp1, 1),
					unqueuedEvt(2, articleID, t1.Add(30*time.Minute)),
					queuedEvt(3, articleID, t2, exp2, 2),
				}
			},
			wantQueuedAt:      t2,
			wantExpiresAt:     exp2,
			wantPolicyVersion: 2,
			wantLastSeq:       3,
		},
		{
			name: "queued then queued after expiry uses new times",
			buildEvents: func(articleID uuid.UUID) []sovereign_db.KnowledgeEvent {
				return []sovereign_db.KnowledgeEvent{
					queuedEvt(1, articleID, t1, exp1, 1),
					queuedEvt(2, articleID, tAfterExpiry, expAfterExpiry, 2),
				}
			},
			wantQueuedAt:      tAfterExpiry,
			wantExpiresAt:     expAfterExpiry,
			wantPolicyVersion: 2,
			wantLastSeq:       2,
		},
		{
			name: "out-of-order older seq is ignored",
			buildEvents: func(articleID uuid.UUID) []sovereign_db.KnowledgeEvent {
				return []sovereign_db.KnowledgeEvent{
					queuedEvt(5, articleID, t2, exp2, 2),
					queuedEvt(3, articleID, t1, exp1, 1),
				}
			},
			wantQueuedAt:      t2,
			wantExpiresAt:     exp2,
			wantPolicyVersion: 2,
			wantLastSeq:       5,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			articleID := uuid.New()
			repo := newFakeReadingRepo(nil)
			p := NewProjector(repo, nil, Config{})

			for _, evt := range tc.buildEvents(articleID) {
				require.NoError(t, p.foldEvent(context.Background(), evt))
			}

			row, ok := repo.queue[queueKey{userID: userID, articleID: articleID}]
			require.True(t, ok)
			assert.Nil(t, row.removedAt)
			assert.Equal(t, tc.wantQueuedAt, row.item.QueuedAt)
			assert.Equal(t, tc.wantExpiresAt, row.item.ExpiresAt)
			assert.Equal(t, tc.wantPolicyVersion, row.item.QueuePolicyVersion)
			assert.Equal(t, tc.wantLastSeq, row.item.LastEventSeq)
		})
	}
}

func TestReadingProjector_SkipsUnknownEventAndStopsOnMalformedPayload(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	t0 := time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC)

	events := []sovereign_db.KnowledgeEvent{
		readingEvt(t, 1, "reading.opened.v1", t0, tenantID, userID, "article:"+uuid.NewString(), map[string]any{"article_id": uuid.NewString()}),
		{
			EventID:     uuid.New(),
			EventSeq:    2,
			OccurredAt:  t0.Add(time.Minute),
			TenantID:    tenantID,
			UserID:      &userID,
			EventType:   eventSessionEnded,
			AggregateID: "article:bad",
			Payload:     json.RawMessage(`{bad-json`),
		},
	}

	repo := newFakeReadingRepo(events)
	p := NewProjector(repo, nil, Config{})
	err := p.RunBatch(context.Background())
	require.Error(t, err)
	assert.Equal(t, int64(1), repo.checkpoint.LastEventSeq)
}

func TestReadingProjector_ReadingSourceMergeOrderIndependent(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	tOld := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	tEarly := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	tLate := time.Date(2026, 11, 18, 14, 0, 0, 0, time.UTC)

	legacyEvt := func(seq int64, articleID uuid.UUID, occurredAt time.Time) sovereign_db.KnowledgeEvent {
		return readingEvt(t, seq, eventHomeItemOpened, occurredAt, tenantID, userID, "article:"+articleID.String(), map[string]any{
			"item_key": "article:" + articleID.String(),
		})
	}
	v1Evt := func(seq int64, articleID uuid.UUID, occurredAt time.Time) sovereign_db.KnowledgeEvent {
		return readingEvt(t, seq, eventSessionEnded, occurredAt, tenantID, userID, "article:"+articleID.String(), map[string]any{
			"article_id":        articleID.String(),
			"active_dwell_ms":   6000,
			"body_length_chars": 1500,
			"reached_end":       true,
		})
	}

	tests := []struct {
		name        string
		buildEvents func(articleID uuid.UUID) []sovereign_db.KnowledgeEvent
		wantTime    time.Time
		wantVersion int
	}{
		{
			name: "legacy then v1",
			buildEvents: func(articleID uuid.UUID) []sovereign_db.KnowledgeEvent {
				return []sovereign_db.KnowledgeEvent{
					legacyEvt(1, articleID, tOld),
					v1Evt(2, articleID, tEarly),
				}
			},
			wantTime:    tEarly,
			wantVersion: ReadRuleVersionV1,
		},
		{
			name: "v1 then legacy (replay order reversed)",
			buildEvents: func(articleID uuid.UUID) []sovereign_db.KnowledgeEvent {
				return []sovereign_db.KnowledgeEvent{
					v1Evt(1, articleID, tEarly),
					legacyEvt(2, articleID, tOld),
				}
			},
			wantTime:    tEarly,
			wantVersion: ReadRuleVersionV1,
		},
		{
			name: "v1 twice earlier then later",
			buildEvents: func(articleID uuid.UUID) []sovereign_db.KnowledgeEvent {
				return []sovereign_db.KnowledgeEvent{
					v1Evt(1, articleID, tEarly),
					v1Evt(2, articleID, tLate),
				}
			},
			wantTime:    tEarly,
			wantVersion: ReadRuleVersionV1,
		},
		{
			name: "v1 twice later then earlier",
			buildEvents: func(articleID uuid.UUID) []sovereign_db.KnowledgeEvent {
				return []sovereign_db.KnowledgeEvent{
					v1Evt(1, articleID, tLate),
					v1Evt(2, articleID, tEarly),
				}
			},
			wantTime:    tEarly,
			wantVersion: ReadRuleVersionV1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			articleID := uuid.New()
			repo := newFakeReadingRepo(tc.buildEvents(articleID))
			p := NewProjector(repo, nil, Config{})
			require.NoError(t, p.RunBatch(context.Background()))

			src, ok := repo.sources[queueKey{userID: userID, articleID: articleID}]
			require.True(t, ok)
			assert.Equal(t, tc.wantTime, src.FirstReadAt)
			assert.Equal(t, tc.wantVersion, src.ReadRuleVersion)
		})
	}
}

type recordedLog struct {
	Level   slog.Level
	Message string
}

type recordingLogHandler struct {
	records []recordedLog
}

func (h *recordingLogHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordingLogHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, recordedLog{Level: r.Level, Message: r.Message})
	return nil
}
func (h *recordingLogHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *recordingLogHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *recordingLogHandler) count(msg string) int {
	n := 0
	for _, r := range h.records {
		if r.Message == msg {
			n++
		}
	}
	return n
}

func (h *recordingLogHandler) find(msg string) (recordedLog, bool) {
	for _, r := range h.records {
		if r.Message == msg {
			return r, true
		}
	}
	return recordedLog{}, false
}

func noopEvents(t *testing.T, count int) []sovereign_db.KnowledgeEvent {
	t.Helper()
	t0 := time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC)
	tenantID := uuid.New()
	userID := uuid.New()
	events := make([]sovereign_db.KnowledgeEvent, count)
	for i := range count {
		u := userID
		events[i] = sovereign_db.KnowledgeEvent{
			EventID:     uuid.New(),
			EventSeq:    int64(i + 1),
			OccurredAt:  t0,
			TenantID:    tenantID,
			UserID:      &u,
			EventType:   "ArticleCreated",
			AggregateID: "article:noop",
			Payload:     json.RawMessage(`{}`),
		}
	}
	return events
}

func TestReadingProjector_BacklogDrainsAndLogsCatchupLifecycle(t *testing.T) {
	t.Run("drains multi-batch backlog up to configured limit and logs catchup once", func(t *testing.T) {
		events := noopEvents(t, 32)
		repo := newFakeReadingRepo(events)
		logs := &recordingLogHandler{}
		p := NewProjector(repo, slog.New(logs), Config{BatchSize: 5, MaxBatchesPerTick: 3})

		// Tick 1: drains 15 events (3 batches of 5) -> behind by more than 1 full tick
		require.NoError(t, p.RunBatch(context.Background()))
		assert.Equal(t, int64(15), repo.checkpoint.LastEventSeq)
		assert.Equal(t, 1, logs.count("reading_projector.catchup_started"))
		assert.Equal(t, 0, logs.count("reading_projector.catchup_completed"))
		rec, ok := logs.find("reading_projector.catchup_started")
		require.True(t, ok)
		assert.Equal(t, slog.LevelInfo, rec.Level)

		// Tick 2: drains next 15 events (seq 16..30) -> still behind, no duplicate catchup_started
		require.NoError(t, p.RunBatch(context.Background()))
		assert.Equal(t, int64(30), repo.checkpoint.LastEventSeq)
		assert.Equal(t, 1, logs.count("reading_projector.catchup_started"))
		assert.Equal(t, 0, logs.count("reading_projector.catchup_completed"))

		// Tick 3: drains final 2 events (short batch) -> caught up logged once at Info
		require.NoError(t, p.RunBatch(context.Background()))
		assert.Equal(t, int64(32), repo.checkpoint.LastEventSeq)
		assert.Equal(t, 1, logs.count("reading_projector.catchup_started"))
		assert.Equal(t, 1, logs.count("reading_projector.catchup_completed"))
		doneRec, ok := logs.find("reading_projector.catchup_completed")
		require.True(t, ok)
		assert.Equal(t, slog.LevelInfo, doneRec.Level)

		// Tick 4: steady-state empty poll -> no extra catchup logs
		require.NoError(t, p.RunBatch(context.Background()))
		assert.Equal(t, 1, logs.count("reading_projector.catchup_started"))
		assert.Equal(t, 1, logs.count("reading_projector.catchup_completed"))
	})

	t.Run("exact batch multiple catches up on subsequent empty read", func(t *testing.T) {
		events := noopEvents(t, 15)
		repo := newFakeReadingRepo(events)
		logs := &recordingLogHandler{}
		p := NewProjector(repo, slog.New(logs), Config{BatchSize: 5, MaxBatchesPerTick: 2})

		require.NoError(t, p.RunBatch(context.Background()))
		assert.Equal(t, int64(10), repo.checkpoint.LastEventSeq)
		assert.Equal(t, 1, logs.count("reading_projector.catchup_started"))
		assert.Equal(t, 0, logs.count("reading_projector.catchup_completed"))

		require.NoError(t, p.RunBatch(context.Background()))
		assert.Equal(t, int64(15), repo.checkpoint.LastEventSeq)
		assert.Equal(t, 1, logs.count("reading_projector.catchup_completed"))
	})

	t.Run("small steady-state multi-batch under one full tick does not emit catchup logs", func(t *testing.T) {
		events := noopEvents(t, 7)
		repo := newFakeReadingRepo(events)
		logs := &recordingLogHandler{}
		p := NewProjector(repo, slog.New(logs), Config{BatchSize: 5, MaxBatchesPerTick: 3})

		require.NoError(t, p.RunBatch(context.Background()))
		assert.Equal(t, int64(7), repo.checkpoint.LastEventSeq)
		assert.Empty(t, logs.records)
	})

	t.Run("default config drains 20000 events in one RunBatch call", func(t *testing.T) {
		events := noopEvents(t, 20500)
		repo := newFakeReadingRepo(events)
		logs := &recordingLogHandler{}
		p := NewProjector(repo, slog.New(logs), Config{})

		require.NoError(t, p.RunBatch(context.Background()))
		assert.Equal(t, int64(20000), repo.checkpoint.LastEventSeq)
		assert.Equal(t, 1, logs.count("reading_projector.catchup_started"))
		assert.Equal(t, 0, logs.count("reading_projector.catchup_completed"))

		require.NoError(t, p.RunBatch(context.Background()))
		assert.Equal(t, int64(20500), repo.checkpoint.LastEventSeq)
		assert.Equal(t, 1, logs.count("reading_projector.catchup_completed"))
	})
}
