//go:build integration

package projection_gap

// The gap verdict against a real PostgreSQL. Every number a fake could feed
// this decision is a claim about what the server hands out, and the claim the
// decision rests on — that a hole no live writer can still fill is safe to step
// over — is only checkable against a server that hands out real transaction ids
// and real sequence values.
//
// The interleaving below is the one two concurrent AppendKnowledgeEventIfNew
// calls produce on their own: the transaction id comes from the dedupe INSERT
// and the sequence from the event INSERT one round trip later, so the appender
// that ends up with the lower sequence can hold the higher transaction id.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
	"knowledge-sovereign/test_utils/pgtest"
)

// appender is one half-finished append: its transaction has taken an id from
// the dedupe INSERT and is waiting to be told when to take a sequence.
type appender struct {
	tx  pgx.Tx
	key string
}

func beginAppend(t *testing.T, ctx context.Context, db interface {
	Begin(context.Context) (pgx.Tx, error)
}, key string,
) *appender {
	t.Helper()
	tx, err := db.Begin(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })

	_, err = tx.Exec(ctx, `INSERT INTO knowledge_event_dedupes (dedupe_key, event_id, occurred_at)
		VALUES ($1, $2, $3)`, key, uuid.New(), time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	return &appender{tx: tx, key: key}
}

// takeSeq runs the event INSERT, which is where event_seq is handed out.
func (a *appender) takeSeq(t *testing.T, ctx context.Context) int64 {
	t.Helper()
	var seq int64
	require.NoError(t, a.tx.QueryRow(ctx, `INSERT INTO knowledge_events
		(event_id, occurred_at, tenant_id, user_id, actor_type, actor_id,
		 event_type, aggregate_type, aggregate_id, dedupe_key, payload)
		VALUES ($1, $2, $3, NULL, 'system', 'pgtest', 'ArticleCreated', 'article', $4, $5, $6)
		RETURNING event_seq`,
		uuid.New(), time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC), uuid.New(),
		a.key, a.key, json.RawMessage(`{}`)).Scan(&seq))
	return seq
}

// A writer still in flight owns every sequence it has taken. While it can still
// commit, the hole it leaves must never be judged burned — stepping over it
// drops the event from the read models for good, and the tip-minus-checkpoint
// lag metric never notices.
func TestTracker_NeverAbandonsASequenceAWriterStillHolds(t *testing.T) {
	ctx := context.Background()
	db := pgtest.NewDB(t)
	repo := sovereign_db.NewRepository(db)

	base := beginAppend(t, ctx, db, "base")
	baseSeq := base.takeSeq(t, ctx)
	require.NoError(t, base.tx.Commit(ctx))

	// The interleaving: `committing` takes its transaction id first, `stalled`
	// takes the next one — and then `stalled` takes the lower sequence.
	committing := beginAppend(t, ctx, db, "committing")
	stalled := beginAppend(t, ctx, db, "stalled")

	stalledSeq := stalled.takeSeq(t, ctx)
	committingSeq := committing.takeSeq(t, ctx)
	require.Less(t, stalledSeq, committingSeq, "the stalled writer must hold the lower sequence")
	require.NoError(t, committing.tx.Commit(ctx))

	events, err := repo.ListKnowledgeEventsSince(ctx, baseSeq, 100)
	require.NoError(t, err)
	batch, hole := ContiguousPrefix(events, baseSeq)
	require.Empty(t, batch, "the batch is blocked at the sequence the stalled writer holds")
	require.Equal(t, Hole{First: stalledSeq, Last: stalledSeq}, hole)

	var tracker Tracker
	first, err := repo.ReadSequenceGapFrontier(ctx, hole.First, hole.Last)
	require.NoError(t, err)
	require.False(t, tracker.MayAbandon(hole, first), "a hole is never abandoned on first sight")

	second, err := repo.ReadSequenceGapFrontier(ctx, hole.First, hole.Last)
	require.NoError(t, err)
	assert.False(t, tracker.MayAbandon(hole, second),
		"seq %d is still held by a live transaction; first sighting %+v, second %+v",
		stalledSeq, first, second)

	// What the verdict above would have cost: the sequence is real data, and it
	// arrives the moment the writer commits.
	require.NoError(t, stalled.tx.Commit(ctx))
	events, err = repo.ListKnowledgeEventsSince(ctx, baseSeq, 100)
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, stalledSeq, events[0].EventSeq, "the abandoned sequence carried an event all along")
}

// The verdict rests on the hole being open under the very snapshot that
// answered for xmin. Read Committed takes a fresh snapshot per statement — "two
// successive SELECT commands can see different data, even though they are
// within a single transaction" (PostgreSQL 16 §13.2.1) — so a writer that
// commits between the batch read and the verdict read is absent from the first
// and finished in the second. Judging on that pair burns a sequence whose event
// is committed and sitting in the table.
func TestTracker_NeverAbandonsASequenceThatCommittedBeforeTheVerdictWasRead(t *testing.T) {
	ctx := context.Background()
	db := pgtest.NewDB(t)
	repo := sovereign_db.NewRepository(db)

	base := beginAppend(t, ctx, db, "base")
	baseSeq := base.takeSeq(t, ctx)
	require.NoError(t, base.tx.Commit(ctx))

	committing := beginAppend(t, ctx, db, "committing")
	stalled := beginAppend(t, ctx, db, "stalled")

	stalledSeq := stalled.takeSeq(t, ctx)
	require.Less(t, stalledSeq, committing.takeSeq(t, ctx))
	require.NoError(t, committing.tx.Commit(ctx))

	var tracker Tracker

	events, err := repo.ListKnowledgeEventsSince(ctx, baseSeq, 100)
	require.NoError(t, err)
	_, hole := ContiguousPrefix(events, baseSeq)
	require.Equal(t, Hole{First: stalledSeq, Last: stalledSeq}, hole)
	first, err := repo.ReadSequenceGapFrontier(ctx, hole.First, hole.Last)
	require.NoError(t, err)
	require.False(t, tracker.MayAbandon(hole, first))

	// The tick that decides: the batch read still shows the hole, and the
	// writer commits before the verdict is read.
	events, err = repo.ListKnowledgeEventsSince(ctx, baseSeq, 100)
	require.NoError(t, err)
	_, hole = ContiguousPrefix(events, baseSeq)
	require.Equal(t, Hole{First: stalledSeq, Last: stalledSeq}, hole)

	require.NoError(t, stalled.tx.Commit(ctx))

	second, err := repo.ReadSequenceGapFrontier(ctx, hole.First, hole.Last)
	require.NoError(t, err)
	assert.False(t, tracker.MayAbandon(hole, second),
		"seq %d was committed before the verdict was read; first sighting %+v, second %+v",
		stalledSeq, first, second)

	events, err = repo.ListKnowledgeEventsSince(ctx, baseSeq, 100)
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, stalledSeq, events[0].EventSeq, "the abandoned sequence is committed and readable")
}

// The mirror: a sequence a rolled-back transaction burned is never filled, and
// waiting for it forever would wedge the projection at that sequence. Once no
// live transaction can still hold it, the verdict must come.
func TestTracker_AbandonsASequenceBurnedByARolledBackTransaction(t *testing.T) {
	ctx := context.Background()
	db := pgtest.NewDB(t)
	repo := sovereign_db.NewRepository(db)

	base := beginAppend(t, ctx, db, "base")
	baseSeq := base.takeSeq(t, ctx)
	require.NoError(t, base.tx.Commit(ctx))

	committing := beginAppend(t, ctx, db, "committing")
	rolledBack := beginAppend(t, ctx, db, "rolled-back")

	burnedSeq := rolledBack.takeSeq(t, ctx)
	require.Less(t, burnedSeq, committing.takeSeq(t, ctx))
	require.NoError(t, committing.tx.Commit(ctx))
	require.NoError(t, rolledBack.tx.Rollback(ctx))

	events, err := repo.ListKnowledgeEventsSince(ctx, baseSeq, 100)
	require.NoError(t, err)
	_, hole := ContiguousPrefix(events, baseSeq)
	require.Equal(t, Hole{First: burnedSeq, Last: burnedSeq}, hole)

	var tracker Tracker
	first, err := repo.ReadSequenceGapFrontier(ctx, hole.First, hole.Last)
	require.NoError(t, err)
	require.False(t, tracker.MayAbandon(hole, first))

	second, err := repo.ReadSequenceGapFrontier(ctx, hole.First, hole.Last)
	require.NoError(t, err)
	assert.True(t, tracker.MayAbandon(hole, second),
		"no transaction can still commit seq %d; first sighting %+v, second %+v",
		burnedSeq, first, second)
}
