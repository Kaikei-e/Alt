//go:build integration

package sovereign_db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/test_utils/pgtest"
)

// The core claim, against a server rather than a script: every table in the
// target really ends up empty, the checkpoint really ends up at 0, and nothing
// outside the target is touched — not the other projection's tables, not the
// other projector's checkpoint.
func TestRebuildProjection_EmptiesTheTargetAndLeavesEverythingElseAlone(t *testing.T) {
	t.Parallel()

	cases := []struct {
		target    string
		projector string
		seq       int64
		other     string
		otherProj string
	}{
		{target: "knowledge-home", projector: pgHomeProjector, seq: 1379513, other: "knowledge-trail", otherProj: pgTrailProjector},
		{target: "knowledge-trail", projector: pgTrailProjector, seq: 4242, other: "knowledge-home", otherProj: pgHomeProjector},
	}

	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			db := pgtest.NewDB(t)

			target := pgTarget(t, tc.target)
			other := pgTarget(t, tc.other)

			var homeSeq, trailSeq int64 = 1379513, 4242
			pgSeedProjections(ctx, t, db, homeSeq, trailSeq)

			for _, table := range target.Tables() {
				require.EqualValues(t, pgSeedRows, pgCountRows(ctx, t, db, table),
					"seed must leave %s non-empty or the assertion below proves nothing", table)
			}

			result, err := NewRepository(db).RebuildProjection(ctx, target)
			require.NoError(t, err, "TRUNCATE TABLE %v was rejected by the server", target.Tables())

			for _, table := range target.Tables() {
				assert.Zero(t, pgCountRows(ctx, t, db, table), "%s must be empty after a rebuild", table)
			}

			seq, updatedAt, ok := pgReadCheckpoint(ctx, t, db, tc.projector)
			require.True(t, ok, "the rebuild must leave a checkpoint row behind")
			assert.Zero(t, seq, "the checkpoint must be reset to 0 so the projector re-folds from the start")
			assert.True(t, updatedAt.After(pgCheckpointAt),
				"the checkpoint row must actually be rewritten, not left as it was (updated_at %s)", updatedAt)

			assert.Equal(t, tc.seq, result.CheckpointBefore,
				"the operator must be told the sequence the rebuild reset from")
			assert.Equal(t, len(target.Tables()), result.TablesTruncated)

			for _, table := range other.Tables() {
				assert.EqualValues(t, pgSeedRows, pgCountRows(ctx, t, db, table),
					"rebuilding %s must not empty %s, which belongs to %s", target.Name(), table, other.Name())
			}
			otherSeq, _, ok := pgReadCheckpoint(ctx, t, db, tc.otherProj)
			require.True(t, ok)
			assert.NotZero(t, otherSeq,
				"rebuilding %s must not reset %s's checkpoint", target.Name(), other.Name())
		})
	}
}

// The invariant the whole allowlist exists to protect: the append-only log and
// the ingest dedupe barrier are the source of truth and must survive a rebuild
// of anything. A fake pool can only tell you the SQL text did not contain the
// word; a server can tell you the rows are still there.
func TestRebuildProjection_LeavesTheEventLogIntact(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)

	pgSeedProjections(ctx, t, db, 1379513, 4242)

	before := map[string]int64{}
	for _, table := range protectedTables {
		before[table] = pgCountRows(ctx, t, db, table)
		require.NotZero(t, before[table], "%s must be seeded for this test to mean anything", table)
	}

	var seqBefore []int64
	rows, err := db.Query(ctx, `SELECT event_seq FROM knowledge_events ORDER BY event_seq`)
	require.NoError(t, err)
	for rows.Next() {
		var seq int64
		require.NoError(t, rows.Scan(&seq))
		seqBefore = append(seqBefore, seq)
	}
	require.NoError(t, rows.Err())

	repo := NewRepository(db)
	for _, target := range RebuildTargets() {
		_, err := repo.RebuildProjection(ctx, target)
		require.NoError(t, err)
	}

	for _, table := range protectedTables {
		assert.Equal(t, before[table], pgCountRows(ctx, t, db, table),
			"%s is the source of truth and must survive every rebuild", table)
	}

	var seqAfter []int64
	rows, err = db.Query(ctx, `SELECT event_seq FROM knowledge_events ORDER BY event_seq`)
	require.NoError(t, err)
	for rows.Next() {
		var seq int64
		require.NoError(t, rows.Scan(&seq))
		seqAfter = append(seqAfter, seq)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, seqBefore, seqAfter,
		"the event sequence must be identical after a rebuild — the log is not renumbered")
}

// Running the same rebuild twice must be safe: an operator who is not sure the
// first one landed will run it again.
func TestRebuildProjection_IsIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)

	target := pgTarget(t, "knowledge-trail")
	pgSeedProjections(ctx, t, db, 1379513, 4242)
	repo := NewRepository(db)

	first, err := repo.RebuildProjection(ctx, target)
	require.NoError(t, err)
	assert.EqualValues(t, 4242, first.CheckpointBefore)

	second, err := repo.RebuildProjection(ctx, target)
	require.NoError(t, err)
	assert.Zero(t, second.CheckpointBefore, "the second rebuild resets from the first one's 0")

	for _, table := range target.Tables() {
		assert.Zero(t, pgCountRows(ctx, t, db, table))
	}
	seq, _, ok := pgReadCheckpoint(ctx, t, db, pgTrailProjector)
	require.True(t, ok)
	assert.Zero(t, seq)
}

// The PM-2026-010 lock, checked against the server rather than against statement
// order in a script. A concurrent session holding the checkpoint row must block
// the rebuild at its opening SELECT ... FOR UPDATE — before anything is
// truncated — so the projector's checkpoint write can never land in the middle
// of a rebuild.
func TestRebuildProjection_BlocksOnAConcurrentlyHeldCheckpointRow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)

	target := pgTarget(t, "knowledge-trail")
	pgSeedProjections(ctx, t, db, 1379513, 4242)

	projector, err := db.Acquire(ctx)
	require.NoError(t, err)
	defer projector.Release()
	projectorTx, err := projector.Begin(ctx)
	require.NoError(t, err)
	_, err = projectorTx.Exec(ctx,
		`UPDATE knowledge_projection_checkpoints SET last_event_seq = 9999, updated_at = $2
		 WHERE projector_name = $1`, pgTrailProjector, pgCheckpointAt)
	require.NoError(t, err)

	rebuilder := pgLockTimeoutConn(ctx, t, db, "500ms")
	_, err = NewRepository(rebuilder).RebuildProjection(ctx, target)
	require.Error(t, err)
	assert.ErrorContains(t, err, "lock checkpoint",
		"the rebuild must block on the checkpoint row before it truncates anything")
	assert.ErrorContains(t, err, "lock timeout")

	require.NoError(t, projectorTx.Rollback(ctx))

	for _, table := range target.Tables() {
		assert.EqualValues(t, pgSeedRows, pgCountRows(ctx, t, db, table),
			"nothing may be truncated when the checkpoint could not be locked; %s is empty", table)
	}
	seq, _, ok := pgReadCheckpoint(ctx, t, db, pgTrailProjector)
	require.True(t, ok)
	assert.EqualValues(t, 4242, seq)
}
