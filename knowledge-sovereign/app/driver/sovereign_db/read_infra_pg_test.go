//go:build integration

package sovereign_db

// The projection-checkpoint compare-and-set, against a real PostgreSQL carrying
// the real Atlas migration history.
//
// read_infra_test.go covers the same pair against a hand-written pgx fake. The
// fake can prove which statement was issued with which arguments; it cannot
// answer the only question that matters here, which is what the *server* does
// when two writers reach the same row in an interleaving. Whether the guarded
// UPDATE really matches zero rows once RebuildProjection has rewritten them,
// whether an INSERT ... ON CONFLICT DO NOTHING really declines to overwrite a
// row created underneath it, and whether a timestamptz witness really
// round-trips to an exact equality match are all invisible to a script.
//
// Helpers (pgSeedProjections, pgReadCheckpoint, pgCountRows, pgTarget and the
// fixed instants) live in rebuild_projection_pg_test.go.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/test_utils/pgtest"
)

// The ordinary case: nothing else wrote, so the advance lands and the witness
// moves with it.
func TestAdvanceProjectionCheckpointIfUnchanged_AppliesWhenNothingElseWrote(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)

	pgSeedProjections(ctx, t, db, 1379513, 4242)
	repo := NewRepository(db)

	from, err := repo.ReadProjectionCheckpointForAdvance(ctx, pgHomeProjector)
	require.NoError(t, err)
	require.True(t, from.Exists)
	require.EqualValues(t, 1379513, from.LastEventSeq)

	applied, err := repo.AdvanceProjectionCheckpointIfUnchanged(ctx, pgHomeProjector, from, 1379600)
	require.NoError(t, err)
	assert.True(t, applied, "an uncontended advance must land, or the projector never makes progress")

	seq, updatedAt, ok := pgReadCheckpoint(ctx, t, db, pgHomeProjector)
	require.True(t, ok)
	assert.EqualValues(t, 1379600, seq)
	assert.True(t, updatedAt.After(pgCheckpointAt),
		"the witness must move too, so a token read before this advance is invalidated by it")
}

// PM-2026-010, at the level of the two statements that raced.
//
// The projector reads its checkpoint, folds a batch, and writes the checkpoint
// back. A rebuild that commits in that window empties the read models and
// resets the checkpoint to 0. The rebuild's FOR UPDATE makes the projector's
// write *wait*; it does not make it re-read. An unconditional upsert therefore
// restores the pre-rebuild tip over an empty read model, and every event up to
// that tip is never folded again.
func TestAdvanceProjectionCheckpointIfUnchanged_RefusesToOverwriteARebuildsReset(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)

	target := pgTarget(t, "knowledge-home")
	pgSeedProjections(ctx, t, db, 1379513, 4242)
	repo := NewRepository(db)

	// t0: the projector reads the checkpoint it is about to fold from.
	from, err := repo.ReadProjectionCheckpointForAdvance(ctx, pgHomeProjector)
	require.NoError(t, err)
	require.EqualValues(t, 1379513, from.LastEventSeq)

	// t1..t3: the operator's rebuild runs to completion.
	_, err = repo.RebuildProjection(ctx, target)
	require.NoError(t, err)

	// t4: the in-flight batch writes the sequence it folded up to.
	applied, err := repo.AdvanceProjectionCheckpointIfUnchanged(ctx, pgHomeProjector, from, 1379600)
	require.NoError(t, err)
	assert.False(t, applied, "the batch read a checkpoint the rebuild has since reset; its advance must not land")

	seq, _, ok := pgReadCheckpoint(ctx, t, db, pgHomeProjector)
	require.True(t, ok)
	assert.Zero(t, seq,
		"the rebuild's reset must survive: at 1379600 over emptied tables the projector only ever "+
			"fetches events > 1379600, so everything below it is lost")
	for _, table := range target.Tables() {
		assert.Zero(t, pgCountRows(ctx, t, db, table), "%s must still be empty, waiting to be re-folded", table)
	}
}

// The same race, with the sequence identical on both sides of it.
//
// After one rebuild the checkpoint sits at 0. A projector that read that 0 and
// then met a *second* rebuild — the runbook invites re-running it, and
// TestRebuildProjection_IsIdempotent encodes that operators do — would see 0
// before and 0 after. A sequence-only compare-and-set cannot tell those apart
// and would let the advance through; the updated_at witness is what closes it.
func TestAdvanceProjectionCheckpointIfUnchanged_RefusesWhenASecondRebuildResetTheSameZero(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)

	target := pgTarget(t, "knowledge-home")
	pgSeedProjections(ctx, t, db, 0, 0) // already rebuilt once
	repo := NewRepository(db)

	from, err := repo.ReadProjectionCheckpointForAdvance(ctx, pgHomeProjector)
	require.NoError(t, err)
	require.True(t, from.Exists)
	require.Zero(t, from.LastEventSeq, "precondition: the sequence is identical before and after the rebuild")

	_, err = repo.RebuildProjection(ctx, target)
	require.NoError(t, err)

	applied, err := repo.AdvanceProjectionCheckpointIfUnchanged(ctx, pgHomeProjector, from, 900)
	require.NoError(t, err)
	assert.False(t, applied, "0 before and 0 after is still a rebuild the batch did not observe")

	seq, _, ok := pgReadCheckpoint(ctx, t, db, pgHomeProjector)
	require.True(t, ok)
	assert.Zero(t, seq)
}

// The hole the rebuild's row lock cannot cover. With no checkpoint row there is
// nothing for SELECT ... FOR UPDATE to lock, so a first-ever batch and a
// rebuild overlap freely. The rebuild inserts the row; the batch's advance must
// then find its "there was no row" view stale and decline.
func TestAdvanceProjectionCheckpointIfUnchanged_RefusesWhenARebuildCreatedTheRowFirst(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)

	target := pgTarget(t, "knowledge-home")
	pgSeedProjections(ctx, t, db, 0, 0)
	pgExec(ctx, t, db, `DELETE FROM knowledge_projection_checkpoints WHERE projector_name = $1`, pgHomeProjector)
	repo := NewRepository(db)

	from, err := repo.ReadProjectionCheckpointForAdvance(ctx, pgHomeProjector)
	require.NoError(t, err)
	require.False(t, from.Exists, "precondition: the projector has never run")

	_, err = repo.RebuildProjection(ctx, target)
	require.NoError(t, err)

	applied, err := repo.AdvanceProjectionCheckpointIfUnchanged(ctx, pgHomeProjector, from, 900)
	require.NoError(t, err)
	assert.False(t, applied, "the rebuild created the row this batch believed did not exist")

	seq, _, ok := pgReadCheckpoint(ctx, t, db, pgHomeProjector)
	require.True(t, ok)
	assert.Zero(t, seq, "the rebuild's reset must survive a first-ever batch too")
}

// ...and uncontended, a first-ever batch must still be able to write its
// checkpoint, or a fresh deployment re-folds the same events forever.
func TestAdvanceProjectionCheckpointIfUnchanged_InsertsTheFirstEverCheckpoint(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)

	pgSeedProjections(ctx, t, db, 0, 0)
	pgExec(ctx, t, db, `DELETE FROM knowledge_projection_checkpoints WHERE projector_name = $1`, pgHomeProjector)
	repo := NewRepository(db)

	from, err := repo.ReadProjectionCheckpointForAdvance(ctx, pgHomeProjector)
	require.NoError(t, err)
	require.False(t, from.Exists)

	applied, err := repo.AdvanceProjectionCheckpointIfUnchanged(ctx, pgHomeProjector, from, 900)
	require.NoError(t, err)
	assert.True(t, applied)

	seq, _, ok := pgReadCheckpoint(ctx, t, db, pgHomeProjector)
	require.True(t, ok)
	assert.EqualValues(t, 900, seq)
}

// The rebuild is not the only other writer: alt-backend's reproject swap calls
// the UpdateProjectionCheckpoint RPC, which lands on the unconditional upsert.
// A batch that read the checkpoint before that call must lose to it as well —
// otherwise the swap's chosen sequence is silently reverted.
func TestAdvanceProjectionCheckpointIfUnchanged_RefusesAfterTheWireUpsertWrote(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)

	pgSeedProjections(ctx, t, db, 1379513, 4242)
	repo := NewRepository(db)

	from, err := repo.ReadProjectionCheckpointForAdvance(ctx, pgHomeProjector)
	require.NoError(t, err)

	require.NoError(t, repo.UpdateProjectionCheckpoint(ctx, pgHomeProjector, 500))

	applied, err := repo.AdvanceProjectionCheckpointIfUnchanged(ctx, pgHomeProjector, from, 1379600)
	require.NoError(t, err)
	assert.False(t, applied)

	seq, _, ok := pgReadCheckpoint(ctx, t, db, pgHomeProjector)
	require.True(t, ok)
	assert.EqualValues(t, 500, seq, "the wire caller's sequence must stand")
}
