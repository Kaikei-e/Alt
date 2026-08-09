//go:build integration

package sovereign_db

// RebuildProjection against a real PostgreSQL carrying the real Atlas migration
// history.
//
// rebuild_projection_test.go covers the same method against a hand-written pgx
// fake. That fake replays a script: it can prove the TRUNCATE and the checkpoint
// reset were issued on one transaction, and nothing at all about what a server
// does with them. Whether the allowlisted table names exist, whether
// `TRUNCATE a, b, c` is even accepted, whether a foreign key would make it fail,
// whether the rows really disappear, whether the event log really survives, and
// whether a failure mid-transaction really rolls the TRUNCATE back are all
// invisible to it. They live here.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/test_utils/pgtest"
)

const (
	pgHomeProjector  = "knowledge-home-projector"
	pgTrailProjector = "knowledge-trail-projector"

	// Rows per seeded table. More than one so a partial delete cannot pass as
	// a truncate.
	pgSeedRows = 2
)

// Fixed instants, never wall clock: the seeded checkpoint's updated_at has to
// be old enough that the rebuild's server-side now() is unambiguously newer.
var (
	pgSeedAt       = time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	pgCheckpointAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
)

// pgSeedProjections fills every read model both targets own, the append-only
// event log, the dedupe registry and the user event log, and sets both
// projector checkpoints. Every table a rebuild could plausibly touch therefore
// starts non-empty, so "empty afterwards" and "still there afterwards" are both
// falsifiable.
func pgSeedProjections(ctx context.Context, t *testing.T, db *pgxpool.Pool, homeSeq, trailSeq int64) {
	t.Helper()

	s := struct{ userID, tenantID uuid.UUID }{userID: uuid.New(), tenantID: uuid.New()}

	for i := range pgSeedRows {
		key := fmt.Sprintf("seed-%d", i)
		at := pgSeedAt.Add(time.Duration(i) * time.Minute)

		pgExec(ctx, t, db, `INSERT INTO knowledge_home_items
			(user_id, tenant_id, item_key, item_type, title, generated_at, updated_at)
			VALUES ($1, $2, $3, 'article', 'seeded title', $4, $4)`,
			s.userID, s.tenantID, key, at)

		pgExec(ctx, t, db, `INSERT INTO today_digest_view (user_id, digest_date, updated_at)
			VALUES ($1, $2, $3)`,
			s.userID, at.AddDate(0, 0, i).Format(time.DateOnly), at)

		pgExec(ctx, t, db, `INSERT INTO recall_candidate_view (user_id, item_key, updated_at)
			VALUES ($1, $2, $3)`,
			s.userID, key, at)

		pgExec(ctx, t, db, `INSERT INTO knowledge_trail_footprints
			(user_id, tenant_id, footprint_key, verb, item_key, source_event_type, occurred_at)
			VALUES ($1, $2, $3, 'read', $4, 'trail.footprint.v1', $5)`,
			s.userID, s.tenantID, key, key, at)

		pgExec(ctx, t, db, `INSERT INTO knowledge_trail_branches
			(user_id, tenant_id, branch_key, anchor_item_key, relation_kind, why, confidence, target_item_key, created_at)
			VALUES ($1, $2, $3, $4, 'contrast', 'seeded why', 'high', $4, $5)`,
			s.userID, s.tenantID, key, key, at)

		pgExec(ctx, t, db, `INSERT INTO knowledge_trail_act_outcomes
			(user_id, tenant_id, outcome_key, item_key, source_event_type, occurred_at)
			VALUES ($1, $2, $3, $4, 'trail.act_outcome.v1', $5)`,
			s.userID, s.tenantID, key, key, at)

		pgExec(ctx, t, db, `INSERT INTO knowledge_events
			(occurred_at, tenant_id, user_id, actor_type, event_type, aggregate_type, aggregate_id, dedupe_key, payload)
			VALUES ($1, $2, $3, 'user', 'article.created.v1', 'article', $4, $5, '{}'::jsonb)`,
			at, s.tenantID, s.userID, key, key)

		pgExec(ctx, t, db, `INSERT INTO knowledge_event_dedupes (dedupe_key, event_id, occurred_at)
			VALUES ($1, $2, $3)`,
			key, uuid.New(), at)

		pgExec(ctx, t, db, `INSERT INTO knowledge_user_events
			(occurred_at, user_id, tenant_id, event_type, item_key)
			VALUES ($1, $2, $3, 'home.item.opened.v1', $4)`,
			at, s.userID, s.tenantID, key)
	}

	pgSetCheckpoint(ctx, t, db, pgHomeProjector, homeSeq)
	pgSetCheckpoint(ctx, t, db, pgTrailProjector, trailSeq)
}

func pgExec(ctx context.Context, t *testing.T, db *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	_, err := db.Exec(ctx, sql, args...)
	require.NoError(t, err, "seed: %s", sql)
}

func pgSetCheckpoint(ctx context.Context, t *testing.T, db *pgxpool.Pool, projector string, seq int64) {
	t.Helper()
	pgExec(ctx, t, db, `INSERT INTO knowledge_projection_checkpoints (projector_name, last_event_seq, updated_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (projector_name) DO UPDATE SET last_event_seq = EXCLUDED.last_event_seq, updated_at = EXCLUDED.updated_at`,
		projector, seq, pgCheckpointAt)
}

func pgCountRows(ctx context.Context, t *testing.T, db *pgxpool.Pool, table string) int64 {
	t.Helper()
	var n int64
	// The table name is an allowlisted identifier from the package's own
	// rebuild targets, never operator input.
	require.NoError(t, db.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n))
	return n
}

// pgReadCheckpoint returns the stored sequence and whether the row exists at
// all. The "row does not exist" case is a real state — a projector that has
// never run — and RebuildProjection has a branch for it that the pgx fake can
// never enter, because the fake's row always scans a value.
func pgReadCheckpoint(ctx context.Context, t *testing.T, db *pgxpool.Pool, projector string) (int64, time.Time, bool) {
	t.Helper()
	var seq int64
	var updatedAt time.Time
	err := db.QueryRow(ctx,
		`SELECT last_event_seq, updated_at FROM knowledge_projection_checkpoints WHERE projector_name = $1`,
		projector).Scan(&seq, &updatedAt)
	if err != nil {
		require.ErrorContains(t, err, "no rows", "unexpected checkpoint read failure")
		return 0, time.Time{}, false
	}
	return seq, updatedAt, true
}

// pgLockTimeoutConn hands back a dedicated session with lock_timeout set, so a
// rebuild that blocks on another session's lock fails in bounded time instead of
// hanging the test binary. *pgxpool.Conn satisfies PgxIface, so a Repository can
// be pinned to exactly this session.
func pgLockTimeoutConn(ctx context.Context, t *testing.T, db *pgxpool.Pool, timeout string) *pgxpool.Conn {
	t.Helper()
	conn, err := db.Acquire(ctx)
	require.NoError(t, err)
	t.Cleanup(conn.Release)
	_, err = conn.Exec(ctx, "SET lock_timeout = '"+timeout+"'")
	require.NoError(t, err)
	return conn
}

func pgTarget(t *testing.T, name string) ProjectionRebuildTarget {
	t.Helper()
	target, err := LookupRebuildTarget(name)
	require.NoError(t, err)
	return target
}

// The allowlist names tables that must actually exist, as ordinary tables, in
// the migrated schema. A typo or a table renamed by a later migration is
// invisible to a fake pool — it happily records `TRUNCATE TABLE nonexistent` as
// a successful statement — and would only surface the first time an operator
// ran the rebuild in production.
func TestRebuildProjection_AllowlistedTablesExistInTheMigratedSchema(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)

	for _, target := range RebuildTargets() {
		for _, table := range target.Tables() {
			var kind string
			err := db.QueryRow(ctx, `SELECT c.relkind
				FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
				WHERE n.nspname = 'public' AND c.relname = $1`, table).Scan(&kind)
			require.NoError(t, err, "%s names %q, which the migration history does not create", target.Name(), table)
			assert.Contains(t, []string{"r", "p"}, kind,
				"%s is not an ordinary or partitioned table (relkind %q); TRUNCATE would reject it", table, kind)
		}
	}

	// The protected tables have to exist too — otherwise the guard in
	// RebuildProjection is comparing against names that mean nothing.
	for _, table := range protectedTables {
		var exists bool
		require.NoError(t, db.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = 'public' AND c.relname = $1)`, table).Scan(&exists))
		assert.True(t, exists, "protected table %q does not exist in the migrated schema", table)
	}
}

// TRUNCATE is issued without CASCADE on purpose, which means a foreign key
// pointing at a target table from outside that target's own table set makes the
// whole rebuild fail. Whether such a key exists is a property of the migrated
// schema, so only a real server can answer it.
func TestRebuildProjection_NoForeignKeyPointsAtARebuildTargetFromOutsideIt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)

	for _, target := range RebuildTargets() {
		tables := target.Tables()
		rows, err := db.Query(ctx, `SELECT con.conname, src.relname, tgt.relname
			FROM pg_constraint con
			JOIN pg_class src ON src.oid = con.conrelid
			JOIN pg_class tgt ON tgt.oid = con.confrelid
			WHERE con.contype = 'f' AND tgt.relname = ANY($1)`, tables)
		require.NoError(t, err)

		for rows.Next() {
			var name, referencing, referenced string
			require.NoError(t, rows.Scan(&name, &referencing, &referenced))
			assert.Contains(t, tables, referencing,
				"foreign key %s on %s references %s, which %s truncates without CASCADE",
				name, referencing, referenced, target.Name())
		}
		require.NoError(t, rows.Err())
	}
}

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

// The branch the pgx fake cannot reach: a projector that has never run has no
// checkpoint row, so the FOR UPDATE returns pgx.ErrNoRows and the reset has to
// INSERT rather than UPDATE. The fake's row always scans a value, so this path
// has never been executed before.
func TestRebuildProjection_InsertsTheCheckpointWhenTheProjectorNeverRan(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)

	target := pgTarget(t, "knowledge-home")
	pgSeedProjections(ctx, t, db, 0, 0)
	pgExec(ctx, t, db, `DELETE FROM knowledge_projection_checkpoints WHERE projector_name = $1`, pgHomeProjector)

	_, _, ok := pgReadCheckpoint(ctx, t, db, pgHomeProjector)
	require.False(t, ok, "precondition: no checkpoint row")

	result, err := NewRepository(db).RebuildProjection(ctx, target)
	require.NoError(t, err)
	assert.Zero(t, result.CheckpointBefore)

	seq, _, ok := pgReadCheckpoint(ctx, t, db, pgHomeProjector)
	require.True(t, ok, "the reset must insert the checkpoint row the projector will read")
	assert.Zero(t, seq)

	for _, table := range target.Tables() {
		assert.Zero(t, pgCountRows(ctx, t, db, table))
	}
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

// A table name that does not exist must fail the whole rebuild, and must leave
// the tables that *do* exist alone.
//
// This is the failure the fake pool cannot express at all: rebuildMockPgx
// returns a successful command tag for any statement it was not told to fail, so
// `TRUNCATE TABLE knowledge_home_itemz` reads as a passing test there. The
// allowlist is written by hand and lives beside a migration history that renames
// tables (00017 renamed a column, 00028 dropped a whole projection family) —
// only a server can say whether it still matches.
func TestRebuildProjection_UnknownTableInATargetFailsAndTruncatesNothing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)

	pgSeedProjections(ctx, t, db, 1379513, 4242)

	// Built inside the package on purpose: the exported API cannot express a
	// typo, which is exactly the protection being verified from the other side.
	typo := ProjectionRebuildTarget{
		name:          "knowledge-home",
		tables:        []string{"knowledge_home_items", "knowledge_home_itemz"},
		projectorName: pgHomeProjector,
	}

	_, err := NewRepository(db).RebuildProjection(ctx, typo)
	require.Error(t, err, "a target naming a table that does not exist must fail loudly")
	assert.ErrorContains(t, err, "knowledge_home_itemz")

	assert.EqualValues(t, pgSeedRows, pgCountRows(ctx, t, db, "knowledge_home_items"),
		"a multi-table TRUNCATE is all or nothing; the real table must be untouched")
	seq, _, ok := pgReadCheckpoint(ctx, t, db, pgHomeProjector)
	require.True(t, ok)
	assert.EqualValues(t, 1379513, seq)
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

// Atomicity, failure at the TRUNCATE. A concurrent reader holding ACCESS SHARE
// on one target table blocks the ACCESS EXCLUSIVE lock TRUNCATE needs — the
// everyday case of an API read outliving the operator's patience. With
// lock_timeout the statement aborts, and nothing may have moved: not the rows,
// not the checkpoint.
func TestRebuildProjection_TruncateBlockedByAReaderChangesNothing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)

	target := pgTarget(t, "knowledge-home")
	pgSeedProjections(ctx, t, db, 1379513, 4242)

	reader, err := db.Acquire(ctx)
	require.NoError(t, err)
	defer reader.Release()
	readerTx, err := reader.Begin(ctx)
	require.NoError(t, err)
	var n int64
	require.NoError(t, readerTx.QueryRow(ctx, "SELECT count(*) FROM recall_candidate_view").Scan(&n))

	rebuilder := pgLockTimeoutConn(ctx, t, db, "500ms")
	_, err = NewRepository(rebuilder).RebuildProjection(ctx, target)
	require.Error(t, err, "TRUNCATE must not silently skip a table it cannot lock")
	assert.ErrorContains(t, err, "truncate", "the failure must be reported as the truncate")
	assert.ErrorContains(t, err, "lock timeout", "the rebuild must fail on the lock, not for some other reason")

	require.NoError(t, readerTx.Rollback(ctx))

	for _, table := range target.Tables() {
		assert.EqualValues(t, pgSeedRows, pgCountRows(ctx, t, db, table),
			"a failed rebuild must leave %s untouched", table)
	}
	seq, updatedAt, ok := pgReadCheckpoint(ctx, t, db, pgHomeProjector)
	require.True(t, ok)
	assert.EqualValues(t, 1379513, seq, "a failed rebuild must not reset the checkpoint")
	assert.True(t, updatedAt.Equal(pgCheckpointAt), "a failed rebuild must not rewrite the checkpoint row")
}

// Atomicity, failure *after* the TRUNCATE has already run — the case the fake
// can only simulate.
//
// The scenario is real rather than contrived. When the projector has never run
// there is no checkpoint row, so the `SELECT ... FOR UPDATE` that opens the
// rebuild locks nothing: there is no row to lock. A projector starting up at
// that moment inserts its first checkpoint row, and the rebuild's
// INSERT ... ON CONFLICT then has to wait on that uncommitted insert — after it
// has already emptied the read models. If the transaction were not one unit, the
// tables would stay empty behind a checkpoint nobody reset.
func TestRebuildProjection_CheckpointFailureAfterTruncateRollsTheTruncateBack(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := pgtest.NewDB(t)

	target := pgTarget(t, "knowledge-home")
	pgSeedProjections(ctx, t, db, 0, 0)
	pgExec(ctx, t, db, `DELETE FROM knowledge_projection_checkpoints WHERE projector_name = $1`, pgHomeProjector)

	projector, err := db.Acquire(ctx)
	require.NoError(t, err)
	defer projector.Release()
	projectorTx, err := projector.Begin(ctx)
	require.NoError(t, err)
	_, err = projectorTx.Exec(ctx,
		`INSERT INTO knowledge_projection_checkpoints (projector_name, last_event_seq, updated_at)
		 VALUES ($1, 900, $2)`, pgHomeProjector, pgCheckpointAt)
	require.NoError(t, err)

	rebuilder := pgLockTimeoutConn(ctx, t, db, "500ms")
	_, err = NewRepository(rebuilder).RebuildProjection(ctx, target)
	require.Error(t, err, "a checkpoint reset that cannot complete must fail the rebuild")
	assert.ErrorContains(t, err, "reset checkpoint",
		"the failure must be reported as the checkpoint reset, which means the TRUNCATE had already run")
	assert.ErrorContains(t, err, "lock timeout",
		"the rebuild must fail waiting on the concurrent insert, not for some other reason")

	require.NoError(t, projectorTx.Rollback(ctx))

	for _, table := range target.Tables() {
		assert.EqualValues(t, pgSeedRows, pgCountRows(ctx, t, db, table),
			"the TRUNCATE must be rolled back with the failed checkpoint reset; %s is empty", table)
	}
	_, _, ok := pgReadCheckpoint(ctx, t, db, pgHomeProjector)
	assert.False(t, ok, "a rolled-back rebuild must not leave a checkpoint row behind")
}
