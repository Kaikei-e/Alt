package sovereign_db

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetBackfillJob_ScansAllFourteenColumns pins the column-count bug found
// in the 2026-07-06 review: the SELECT lists 14 columns (including `kind`)
// but Scan only had 13 destinations, so pgx rejected every real call with a
// field-count mismatch. This structural test fails if a future edit drops
// a column back out of the Scan call without updating the SELECT (or vice
// versa).
func TestGetBackfillJob_ScansAllFourteenColumns(t *testing.T) {
	mock := &mockPgx{}
	wantKind := "articles"
	mock.queryRowFunc = func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
		return &mockRow{scanFunc: func(dest ...interface{}) error {
			require.Len(t, dest, 14, "GetBackfillJob Scan must have 14 destinations matching the 14-column SELECT")
			kindPtr, ok := dest[2].(*string)
			require.True(t, ok, "3rd scan destination (matching SELECT column order job_id, status, kind, ...) must be *string for kind")
			*kindPtr = wantKind
			return nil
		}}
	}

	repo := &Repository{pool: mock}
	job, err := repo.GetBackfillJob(context.Background(), uuid.New())
	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, wantKind, job.Kind, "Kind must round-trip through Scan, not stay zero-valued")
}

// TestActivateProjectionVersion_RejectsUnknownVersionWithoutTouchingActive
// pins the fix for the zero-active-versions bug: an invalid version argument
// must be rejected BEFORE any active version is deactivated, so a bad call
// (or a mid-sequence crash) can never leave zero active projection versions
// (which would silently regress every reader's COALESCE(...,1) fallback).
func TestActivateProjectionVersion_RejectsUnknownVersionWithoutTouchingActive(t *testing.T) {
	mock := &mockPgx{}
	mock.queryRowFunc = func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
		return &mockRow{scanFunc: func(dest ...interface{}) error {
			return pgx.ErrNoRows // version does not exist
		}}
	}

	repo := &Repository{pool: mock}
	err := repo.ActivateProjectionVersion(context.Background(), 999)

	require.Error(t, err, "unknown version must be rejected")
	assert.Contains(t, err.Error(), "not found")
	assert.Empty(t, mock.execCalls, "no UPDATE (deactivate or activate) may run when the target version doesn't exist")
	require.NotNil(t, mock.lastTx)
	assert.True(t, mock.lastTx.rolledBack, "the opened transaction must be rolled back on the existence-check failure")
	assert.False(t, mock.lastTx.committed, "must not commit when the target version doesn't exist")
}

// TestActivateProjectionVersion_DeactivateAndActivateAreAtomic pins the
// fix that the deactivate+activate pair now runs inside a single
// transaction (Begin...Commit) instead of two independent Exec calls —
// a mid-failure between them can no longer be observed as "zero active
// versions" by a concurrent reader.
func TestActivateProjectionVersion_DeactivateAndActivateAreAtomic(t *testing.T) {
	mock := &mockPgx{}
	mock.queryRowFunc = func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
		return &mockRow{scanFunc: func(dest ...interface{}) error {
			if p, ok := dest[0].(*int); ok {
				*p = 1
			}
			return nil
		}}
	}
	mock.execFunc = func(_ context.Context, sql string, _ ...interface{}) (pgconn.CommandTag, error) {
		return pgconn.NewCommandTag("UPDATE 1"), nil
	}

	repo := &Repository{pool: mock}
	err := repo.ActivateProjectionVersion(context.Background(), 2)

	require.NoError(t, err)
	require.Len(t, mock.execCalls, 2, "expected exactly one deactivate and one activate UPDATE")
	assert.Contains(t, mock.execCalls[0].SQL, "status = 'inactive'")
	assert.Contains(t, mock.execCalls[1].SQL, "status = 'active'")
	require.NotNil(t, mock.lastTx)
	assert.True(t, mock.lastTx.committed, "both UPDATEs must be committed together")
}

// === Projection checkpoints: the compare-and-set advance ===

// checkpointWitnessAt is a fixed instant standing in for the row's stored
// updated_at. Never wall clock: it is compared for equality, not recency.
var checkpointWitnessAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// The token has to carry the row's updated_at, not just its sequence.
// last_event_seq alone cannot tell "still the 0 I read" from "reset to 0 again
// by a second rebuild while I was folding" — the state an operator reaches by
// running the documented rebuild twice, which the runbook explicitly invites.
func TestReadProjectionCheckpointForAdvance_ReturnsTheStoredRowAsAToken(t *testing.T) {
	mock := &mockPgx{}
	mock.queryRowFunc = func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
		return &mockRow{scanFunc: func(dest ...interface{}) error {
			require.Len(t, dest, 2, "the token needs the updated_at witness as well as the sequence")
			seq, ok := dest[0].(*int64)
			require.True(t, ok)
			*seq = 4242
			at, ok := dest[1].(*time.Time)
			require.True(t, ok)
			*at = checkpointWitnessAt
			return nil
		}}
	}

	repo := &Repository{pool: mock}
	cp, err := repo.ReadProjectionCheckpointForAdvance(context.Background(), "knowledge-home-projector")

	require.NoError(t, err)
	assert.EqualValues(t, 4242, cp.LastEventSeq)
	assert.True(t, cp.UpdatedAt.Equal(checkpointWitnessAt), "the witness must survive the read")
	assert.True(t, cp.Exists, "a row that was found must be reported as present")
}

// A projector that has never run has no checkpoint row. That is a distinct
// state from "a row holding 0", not a synonym for it: the rebuild inserts the
// row it did not find, so a token that cannot tell the two apart would let a
// first-ever batch overwrite a rebuild that landed underneath it.
func TestReadProjectionCheckpointForAdvance_ReportsAMissingRowAsAbsentNotAsZero(t *testing.T) {
	mock := &mockPgx{}
	mock.queryRowFunc = func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
		return &mockRow{scanFunc: func(_ ...interface{}) error { return pgx.ErrNoRows }}
	}

	repo := &Repository{pool: mock}
	cp, err := repo.ReadProjectionCheckpointForAdvance(context.Background(), "knowledge-home-projector")

	require.NoError(t, err, "a projector that has never run is not an error")
	assert.False(t, cp.Exists, "absent must be distinguishable from a stored 0")
	assert.Zero(t, cp.LastEventSeq)
	assert.True(t, cp.UpdatedAt.IsZero())
}

// The advance must be conditional on the exact state the caller read — both the
// sequence and the witness. An unconditional upsert (the wire-facing
// UpdateProjectionCheckpoint) is what let an in-flight batch overwrite a
// rebuild's reset in PM-2026-010.
func TestAdvanceProjectionCheckpointIfUnchanged_BindsTheStateTheCallerRead(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	from := ProjectionCheckpoint{LastEventSeq: 4242, UpdatedAt: checkpointWitnessAt, Exists: true}
	applied, err := repo.AdvanceProjectionCheckpointIfUnchanged(context.Background(), "knowledge-home-projector", from, 4300)

	require.NoError(t, err)
	assert.True(t, applied, "one row updated means the advance landed")
	require.Len(t, mock.execCalls, 1, "the advance is a single statement, so it cannot be interrupted halfway")

	call := mock.execCalls[0]
	assert.Contains(t, call.Args, int64(4242), "the expected sequence must be bound, or the update is unconditional")
	assert.Contains(t, call.Args, checkpointWitnessAt, "the expected updated_at must be bound, or a 0 -> 0 rebuild is invisible")
	assert.Contains(t, call.Args, int64(4300), "the new sequence must be bound")
}

// The outcome the whole change exists for: the stored row moved, so the advance
// applies to nothing and says so. It must not fall back to an unconditional
// write, and it must not report a failure either — a lost race is a normal,
// recoverable outcome that the caller has to branch on.
func TestAdvanceProjectionCheckpointIfUnchanged_ReportsNotAppliedWhenTheRowMoved(t *testing.T) {
	mock := &mockPgx{}
	mock.execFunc = func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
		return pgconn.NewCommandTag("UPDATE 0"), nil
	}
	repo := &Repository{pool: mock}

	from := ProjectionCheckpoint{LastEventSeq: 4242, UpdatedAt: checkpointWitnessAt, Exists: true}
	applied, err := repo.AdvanceProjectionCheckpointIfUnchanged(context.Background(), "knowledge-home-projector", from, 4300)

	require.NoError(t, err, "losing the race is an outcome, not a failure — callers branch on applied")
	assert.False(t, applied)
	assert.Len(t, mock.execCalls, 1, "a rejected advance must not retry with a weaker statement")
}

// From an absent row the advance may only insert. Upgrading the conflict to an
// UPDATE would overwrite exactly the row a concurrent rebuild had just created,
// which is the hole the FOR UPDATE in RebuildProjection cannot cover (there is
// no row to lock).
func TestAdvanceProjectionCheckpointIfUnchanged_FromAbsentInsertsAndNeverUpdates(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	applied, err := repo.AdvanceProjectionCheckpointIfUnchanged(context.Background(),
		"knowledge-home-projector", ProjectionCheckpoint{}, 17)

	require.NoError(t, err)
	assert.True(t, applied, "the first-ever checkpoint has to be insertable, or a fresh projector never advances")
	require.Len(t, mock.execCalls, 1)
	assert.Contains(t, mock.execCalls[0].SQL, "INSERT")
	assert.NotContains(t, mock.execCalls[0].SQL, "DO UPDATE",
		"an insert that falls through to an update would clobber a row created underneath it")
}

func TestAdvanceProjectionCheckpointIfUnchanged_FromAbsentReportsNotAppliedWhenTheRowAppeared(t *testing.T) {
	mock := &mockPgx{}
	mock.execFunc = func(_ context.Context, _ string, _ ...interface{}) (pgconn.CommandTag, error) {
		return pgconn.NewCommandTag("INSERT 0 0"), nil
	}
	repo := &Repository{pool: mock}

	applied, err := repo.AdvanceProjectionCheckpointIfUnchanged(context.Background(),
		"knowledge-home-projector", ProjectionCheckpoint{}, 17)

	require.NoError(t, err)
	assert.False(t, applied, "someone else created the row; this batch's view of the world is stale")
}

// TestAppendRecallSignal_DefaultsEmptyPayload pins that an omitted payload
// reaches Postgres as `{}` rather than as an explicit NULL.
//
// `recall_signals.payload` is `JSONB NOT NULL DEFAULT '{}'` — the schema saying
// the field is optional. But AppendRecallSignal names `payload` in the INSERT
// column list and binds it unconditionally, so a proto request that omits the
// field (arriving as a nil []byte) sends an explicit NULL, the column DEFAULT
// never applies, and the insert dies on the NOT NULL constraint. The handler
// then surfaces it as Connect `internal` — a malformed-looking request reported
// as a broken service.
//
// Contrast knowledge_events.payload, which is NOT NULL with *no* default: there
// the payload is genuinely required and the caller must supply one.
func TestAppendRecallSignal_DefaultsEmptyPayload(t *testing.T) {
	mock := &mockPgx{}
	repo := &Repository{pool: mock}

	err := repo.AppendRecallSignal(context.Background(), RecallSignal{
		SignalID:   uuid.New(),
		UserID:     uuid.New(),
		ItemKey:    "article:1",
		SignalType: "open",
		Payload:    nil,
	})
	require.NoError(t, err)
	require.Len(t, mock.execCalls, 1)

	args := mock.execCalls[0].Args
	require.Len(t, args, 7, "the INSERT binds seven columns")
	assert.EqualValues(t, []byte("{}"), args[6],
		"an omitted payload must be bound as an empty JSON object; binding nil sends an "+
			"explicit NULL, which defeats the column's own DEFAULT and violates NOT NULL")
}
