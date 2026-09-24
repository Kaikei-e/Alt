package knowledge_home_projector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

// ── checkpoint / unknown events ──

func TestProjector_SkipsUnknownEventTypeButAdvancesCheckpoint(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	occurredAt := time.Date(2026, 7, 14, 17, 0, 0, 0, time.UTC)

	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "SomeFutureEventType", "whatever", occurredAt, tenant, user, mustJSON(t, map[string]any{})),
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{})
	require.NoError(t, p.RunBatch(context.Background()))

	assert.Empty(t, repo.homeItems)
	assert.Equal(t, int64(1), repo.checkpoint, "checkpoint must still advance past an unrecognized event type")
}

func TestProjector_MalformedPayloadStopsBatchButPreservesPriorCheckpoint(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	articleID1 := uuid.New()
	occurredAt := time.Date(2026, 7, 14, 18, 0, 0, 0, time.UTC)

	goodPayload := mustJSON(t, map[string]any{
		"article_id": articleID1.String(),
		"title":      "Good article",
		"url":        "https://example.com/good",
	})
	badPayload := json.RawMessage(`{"article_id": not-json`)

	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "ArticleCreated", articleID1.String(), occurredAt, tenant, user, goodPayload),
		homeEvent(2, "ArticleCreated", "broken", occurredAt.Add(time.Minute), tenant, user, badPayload),
	}
	repo := newFakeRepo(events)
	p := NewProjector(repo, nil, Config{})

	err := p.RunBatch(context.Background())
	require.Error(t, err, "a malformed payload must fail the batch so the event is retried, not silently dropped")

	itemKey1 := fmt.Sprintf("article:%s", articleID1)
	_, ok := repo.homeItems[itemKey1]
	assert.True(t, ok, "events processed before the malformed one must still be applied")
	assert.Equal(t, int64(1), repo.checkpoint, "checkpoint must stop at the last successfully-folded event, not skip past the failure")
}

// ── checkpoint advance ──

// recordingHandler captures log records so a test can assert that a condition
// was reported, and at a level an operator will actually see.
type recordingHandler struct {
	records []recordedLog
}

type recordedLog struct {
	Level   slog.Level
	Message string
	Attrs   map[string]any
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := map[string]any{}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	h.records = append(h.records, recordedLog{Level: r.Level, Message: r.Message, Attrs: attrs})
	return nil
}

func (h *recordingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *recordingHandler) find(message string) (recordedLog, bool) {
	for _, r := range h.records {
		if r.Message == message {
			return r, true
		}
	}
	return recordedLog{}, false
}

// The checkpoint must be advanced with the state the batch actually read, not
// with a value assembled at write time. The fake refuses any other token, the
// same way the guarded UPDATE refuses a row that has moved.
func TestProjector_AdvancesTheCheckpointWithTheStateItReadAtTheStartOfTheBatch(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	occurredAt := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)

	events := []sovereign_db.KnowledgeEvent{
		homeEvent(11, "SomeFutureEventType", "whatever", occurredAt, tenant, user, mustJSON(t, map[string]any{})),
		homeEvent(12, "SomeFutureEventType", "whatever", occurredAt, tenant, user, mustJSON(t, map[string]any{})),
	}
	repo := newFakeRepo(events)
	repo.checkpoint = 10

	require.NoError(t, NewProjector(repo, nil, Config{}).RunBatch(context.Background()))

	require.Len(t, repo.advances, 1)
	assert.EqualValues(t, 10, repo.advances[0].From.LastEventSeq, "the token must carry the sequence the batch folded from")
	assert.True(t, repo.advances[0].From.UpdatedAt.Equal(fakeCheckpointAt), "the token must carry the witness that was read")
	assert.True(t, repo.advances[0].From.Exists)
	assert.EqualValues(t, 12, repo.advances[0].ToSeq)
	assert.EqualValues(t, 12, repo.checkpoint)
}

// PM-2026-010's ending, at unit level: something else wrote the checkpoint
// while this batch was folding. The advance is refused, and the projector must
// not carry on as if it had landed — it stops the tick, says so loudly, and
// lets the next tick re-read whatever the other writer decided.
func TestProjector_RejectedCheckpointAdvanceStopsTheTickAndIsReportedLoudly(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	occurredAt := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)

	events := []sovereign_db.KnowledgeEvent{
		homeEvent(1, "SomeFutureEventType", "a", occurredAt, tenant, user, mustJSON(t, map[string]any{})),
		homeEvent(2, "SomeFutureEventType", "b", occurredAt, tenant, user, mustJSON(t, map[string]any{})),
		homeEvent(3, "SomeFutureEventType", "c", occurredAt, tenant, user, mustJSON(t, map[string]any{})),
		homeEvent(4, "SomeFutureEventType", "d", occurredAt, tenant, user, mustJSON(t, map[string]any{})),
	}
	repo := newFakeRepo(events)
	repo.advanceRejected = true

	logs := &recordingHandler{}
	// Batches of two, four allowed per tick: without an explicit stop the loop
	// would fold the same events again and again behind an unmoving checkpoint.
	p := NewProjector(repo, slog.New(logs), Config{BatchSize: 2, MaxBatchesPerTick: 4})

	require.NoError(t, p.RunBatch(context.Background()),
		"losing the checkpoint race is a recoverable outcome, not a batch failure")

	assert.Zero(t, repo.checkpoint, "a refused advance must leave the stored checkpoint exactly as the other writer left it")
	assert.Len(t, repo.advances, 1, "a refused advance must not be retried in a loop")
	assert.Equal(t, 1, repo.listCalls, "the tick must stop instead of folding the next batch on a checkpoint it could not move")

	rec, ok := logs.find("knowledge_home_projector.checkpoint_advance_rejected")
	require.True(t, ok, "a refused advance must be reported, never swallowed")
	assert.Equal(t, slog.LevelError, rec.Level, "the batch's work was abandoned; that is error-level")
	assert.EqualValues(t, 0, rec.Attrs["expected_seq"])
	assert.EqualValues(t, 2, rec.Attrs["attempted_seq"])
	assert.Equal(t, projectorName, rec.Attrs["projector"])
}

// event_seq is handed out when a transaction inserts, not when it commits, so
// a transaction holding seq 1 can still be in flight while a later
// transaction's seq 2 is already visible. Folding whatever the query returned
// and advancing to its maximum moves the checkpoint to 2, and seq 1 — asked
// for as "event_seq > 2" from then on — is never folded: the article never
// reaches knowledge_home_items, and the lag metric (tip minus checkpoint)
// reads zero while it is gone.
func TestProjector_StopsAtASequenceGapLeftByAnUncommittedTransaction(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	occurredAt := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	first := uuid.New()
	second := uuid.New()

	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		homeEvent(2, "ArticleCreated", second.String(), occurredAt, tenant, user, mustJSON(t, map[string]any{
			"article_id": second.String(),
			"title":      "Second",
			"url":        "https://example.com/second",
		})),
	})
	p := NewProjector(repo, nil, Config{})

	require.NoError(t, p.RunBatch(context.Background()))

	assert.Zero(t, repo.checkpoint, "the checkpoint must not step over a sequence that is missing rather than absent")
	assert.Empty(t, repo.homeItems, "an event beyond the gap must wait for the gap to resolve, so folds stay in sequence order")

	// The transaction holding seq 1 commits.
	repo.events = append([]sovereign_db.KnowledgeEvent{
		homeEvent(1, "ArticleCreated", first.String(), occurredAt, tenant, user, mustJSON(t, map[string]any{
			"article_id": first.String(),
			"title":      "First",
			"url":        "https://example.com/first",
		})),
	}, repo.events...)

	require.NoError(t, p.RunBatch(context.Background()))

	assert.EqualValues(t, 2, repo.checkpoint, "once the gap is filled the batch folds through to the tip")
	assert.Contains(t, repo.homeItems, fmt.Sprintf("article:%s", first), "the event behind the gap must still reach knowledge_home_items")
	assert.Contains(t, repo.homeItems, fmt.Sprintf("article:%s", second))
}

// A rolled-back transaction burns its sequence value: the hole it leaves is
// never filled, and waiting for it forever would wedge every user's Knowledge
// Home at that sequence. Once every transaction that had written when the hole
// was first seen has finished, no live writer can still hold the value, and
// the projection steps over it — loudly, because a burned sequence is a
// producer-side rollback worth seeing.
func TestProjector_StepsPastASequenceBurnedByARolledBackTransaction(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	occurredAt := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	articleID := uuid.New()

	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		homeEvent(2, "ArticleCreated", articleID.String(), occurredAt, tenant, user, mustJSON(t, map[string]any{
			"article_id": articleID.String(),
			"title":      "Second",
			"url":        "https://example.com/second",
		})),
	})
	repo.frontiers = []sovereign_db.SequenceGapFrontier{
		{Ceiling: 101, Xmin: 100}, // the writer that took seq 1 wrote below this ceiling and is still in flight
		{Ceiling: 140, Xmin: 101}, // xmin has reached that ceiling: it finished, and seq 1 never arrived
	}
	logs := &recordingHandler{}
	p := NewProjector(repo, slog.New(logs), Config{})

	require.NoError(t, p.RunBatch(context.Background()))
	assert.Zero(t, repo.checkpoint, "the hole is still fillable on this tick")

	require.NoError(t, p.RunBatch(context.Background()))

	assert.EqualValues(t, 2, repo.checkpoint, "a sequence no live transaction can still commit must not wedge the projection")
	assert.Contains(t, repo.homeItems, fmt.Sprintf("article:%s", articleID), "the events past the burned sequence must be folded")

	rec, ok := logs.find("knowledge_home_projector.sequence_gap_abandoned")
	require.True(t, ok, "abandoning a sequence must be reported, never silent")
	assert.Equal(t, slog.LevelWarn, rec.Level)
	assert.EqualValues(t, 1, rec.Attrs["gap_seq"])
}

// The mirror of the case above: while the transaction that took the missing
// sequence is still running, no number of ticks may step over it.
func TestProjector_WaitsWhileTheTransactionHoldingTheMissingSequenceIsInFlight(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	occurredAt := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	articleID := uuid.New()

	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		homeEvent(2, "ArticleCreated", articleID.String(), occurredAt, tenant, user, mustJSON(t, map[string]any{
			"article_id": articleID.String(),
			"title":      "Second",
			"url":        "https://example.com/second",
		})),
	})
	p := NewProjector(repo, nil, Config{})

	for i := 0; i < 3; i++ {
		require.NoError(t, p.RunBatch(context.Background()))
	}

	assert.Zero(t, repo.checkpoint, "an in-flight writer still owns seq 1; the checkpoint must stay behind it")
	assert.Empty(t, repo.homeItems)
}

// The verdict is a second round trip, and Read Committed gives it its own
// snapshot: a writer that commits between the batch read and the verdict is
// missing from the batch while the ids already count it finished. Judging on
// that pair alone steps the checkpoint over an event that is committed and
// readable, and — asked for as "event_seq > 2" from then on — it never reaches
// knowledge_home_items. The frontier re-reads the run itself for exactly this
// case.
func TestProjector_NeverStepsOverASequenceThatCommittedAfterTheBatchWasRead(t *testing.T) {
	tenant := uuid.New()
	user := userPtr()
	occurredAt := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	first := uuid.New()
	second := uuid.New()

	repo := newFakeRepo([]sovereign_db.KnowledgeEvent{
		homeEvent(2, "ArticleCreated", second.String(), occurredAt, tenant, user, mustJSON(t, map[string]any{
			"article_id": second.String(),
			"title":      "Second",
			"url":        "https://example.com/second",
		})),
	})
	repo.frontiers = []sovereign_db.SequenceGapFrontier{
		{Ceiling: 101, Xmin: 100},
		{Ceiling: 140, Xmin: 101}, // by the ids alone, every writer below the first ceiling has finished
	}
	p := NewProjector(repo, nil, Config{})

	require.NoError(t, p.RunBatch(context.Background()))
	require.Zero(t, repo.checkpoint, "the hole is still fillable on this tick")

	// The writer holding seq 1 commits after this tick's batch read and before
	// its verdict.
	repo.beforeGapFrontier = func() {
		repo.beforeGapFrontier = nil
		repo.events = append([]sovereign_db.KnowledgeEvent{
			homeEvent(1, "ArticleCreated", first.String(), occurredAt, tenant, user, mustJSON(t, map[string]any{
				"article_id": first.String(),
				"title":      "First",
				"url":        "https://example.com/first",
			})),
		}, repo.events...)
	}

	require.NoError(t, p.RunBatch(context.Background()))
	assert.Zero(t, repo.checkpoint, "the sequence arrived; the checkpoint must not step over it")

	require.NoError(t, p.RunBatch(context.Background()))

	assert.EqualValues(t, 2, repo.checkpoint)
	assert.Contains(t, repo.homeItems, fmt.Sprintf("article:%s", first),
		"the event that committed mid-tick must still be folded")
	assert.Contains(t, repo.homeItems, fmt.Sprintf("article:%s", second))
}
