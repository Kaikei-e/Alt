//go:build integration

package sovereign_db

// The sequence gap frontier, against a real PostgreSQL. The snapshot functions
// and the xid8 casts only exist on a server — a fake can prove which statement
// was issued, not that the server accepts it or what it answers — and the
// safety of the whole gap decision rests on three properties of that answer:
// while a writer is still in flight the frontier's xmin never climbs above its
// transaction id, the ceiling handed to the reader always stands above it, and
// the run is reported empty or filled as of the same snapshot those ids
// describe.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/test_utils/pgtest"
)

func pgAppendEvent(t *testing.T, ctx context.Context, repo *Repository, key string) int64 {
	t.Helper()
	seq, appended, err := repo.AppendKnowledgeEventIfNew(ctx, KnowledgeEvent{
		EventID:       uuid.New(),
		OccurredAt:    time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
		TenantID:      uuid.New(),
		ActorType:     "system",
		ActorID:       "pgtest",
		EventType:     "ArticleCreated",
		AggregateType: "article",
		AggregateID:   uuid.NewString(),
		DedupeKey:     key,
		Payload:       json.RawMessage(`{}`),
	})
	require.NoError(t, err)
	require.True(t, appended)
	return seq
}

func TestReadSequenceGapFrontier_ReportsIdsInOrder(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)

	frontier, err := NewRepository(db).ReadSequenceGapFrontier(ctx, 1, 1)
	require.NoError(t, err)

	assert.Positive(t, frontier.Xmin)
	assert.LessOrEqual(t, frontier.Xmin, frontier.Ceiling)
}

func TestReadSequenceGapFrontier_StandsAboveAWriterStillInFlight(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)

	tx, err := db.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx) //nolint:errcheck // the transaction is deliberately never committed

	// pg_current_xact_id assigns the transaction its id, exactly as the first
	// write of an append does.
	var writerXID int64
	require.NoError(t, tx.QueryRow(ctx, `SELECT pg_current_xact_id()::text::bigint`).Scan(&writerXID))

	frontier, err := NewRepository(db).ReadSequenceGapFrontier(ctx, 1, 1)
	require.NoError(t, err)

	assert.LessOrEqual(t, frontier.Xmin, writerXID,
		"a hole this writer owns must never be judged burned while it can still commit")
	assert.Greater(t, frontier.Ceiling, writerXID,
		"ids are handed out in ascending order, so a writer that had already written stands below the reader's own")
}

// The half the ids cannot supply: an event that committed before this statement
// began is reported as filling the run, even though the ids by then call its
// writer finished and would otherwise read the run as burned.
func TestReadSequenceGapFrontier_ReportsARunFilledByAnAlreadyCommittedEvent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)
	repo := NewRepository(db)

	seq := pgAppendEvent(t, ctx, repo, "committed")

	filled, err := repo.ReadSequenceGapFrontier(ctx, seq, seq)
	require.NoError(t, err)
	assert.False(t, filled.HoleOpen, "the run holds a committed event and must not be reported empty")
	assert.Greater(t, filled.Xmin, int64(0))

	open, err := repo.ReadSequenceGapFrontier(ctx, seq+1, seq+1)
	require.NoError(t, err)
	assert.True(t, open.HoleOpen, "nothing has taken that sequence")
}

// The report covers the whole run, not just its ends: one surviving event
// anywhere inside keeps the hole closed.
func TestReadSequenceGapFrontier_ReportsARunFilledByAnEventInsideIt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)
	repo := NewRepository(db)

	seq := pgAppendEvent(t, ctx, repo, "inside")

	frontier, err := repo.ReadSequenceGapFrontier(ctx, seq-1, seq+1)
	require.NoError(t, err)
	assert.False(t, frontier.HoleOpen)
}
