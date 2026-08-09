package sovereign_db

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rebuildStmt records one statement together with whether it was issued on the
// transaction or straight on the pool. The pool/tx distinction is the whole
// point: PM-2026-010 was "TRUNCATE committed, checkpoint reset ran separately,
// the live projector advanced the checkpoint in the gap".
type rebuildStmt struct {
	SQL  string
	Args []interface{}
	OnTx bool
}

type rebuildMockPgx struct {
	stmts      []rebuildStmt
	beginCalls int
	beginErr   error
	commitErr  error
	// execErrFor returns an error for the first statement whose SQL contains
	// the given fragment, letting a test fail the TRUNCATE or the checkpoint
	// reset independently.
	execErrFor string
	execErr    error
	checkpoint int64
	scanErr    error
	tx         *rebuildFakeTx
}

func (m *rebuildMockPgx) record(sql string, args []interface{}, onTx bool) {
	m.stmts = append(m.stmts, rebuildStmt{SQL: sql, Args: args, OnTx: onTx})
}

func (m *rebuildMockPgx) execResult(sql string) (pgconn.CommandTag, error) {
	if m.execErr != nil && m.execErrFor != "" && strings.Contains(sql, m.execErrFor) {
		return pgconn.CommandTag{}, m.execErr
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (m *rebuildMockPgx) row() pgx.Row {
	return &mockRow{scanFunc: func(dest ...interface{}) error {
		if m.scanErr != nil {
			return m.scanErr
		}
		if len(dest) > 0 {
			if p, ok := dest[0].(*int64); ok {
				*p = m.checkpoint
			}
		}
		return nil
	}}
}

func (m *rebuildMockPgx) Query(_ context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	m.record(sql, args, false)
	return &fakeEmptyRows{}, nil
}

func (m *rebuildMockPgx) QueryRow(_ context.Context, sql string, args ...interface{}) pgx.Row {
	m.record(sql, args, false)
	return m.row()
}

func (m *rebuildMockPgx) Exec(_ context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	m.record(sql, args, false)
	return m.execResult(sql)
}

func (m *rebuildMockPgx) Begin(_ context.Context) (pgx.Tx, error) {
	m.beginCalls++
	if m.beginErr != nil {
		return nil, m.beginErr
	}
	m.tx = &rebuildFakeTx{parent: m}
	return m.tx, nil
}

// onTx returns every statement issued inside the transaction, in order.
func (m *rebuildMockPgx) onTx() []rebuildStmt {
	var out []rebuildStmt
	for _, s := range m.stmts {
		if s.OnTx {
			out = append(out, s)
		}
	}
	return out
}

// offTx returns every statement issued straight on the pool. For
// RebuildProjection this must always be empty.
func (m *rebuildMockPgx) offTx() []rebuildStmt {
	var out []rebuildStmt
	for _, s := range m.stmts {
		if !s.OnTx {
			out = append(out, s)
		}
	}
	return out
}

func (m *rebuildMockPgx) indexOf(fragment string) int {
	for i, s := range m.stmts {
		if strings.Contains(s.SQL, fragment) {
			return i
		}
	}
	return -1
}

type rebuildFakeTx struct {
	parent     *rebuildMockPgx
	committed  bool
	rolledBack bool
}

func (t *rebuildFakeTx) Begin(context.Context) (pgx.Tx, error) { return t, nil }
func (t *rebuildFakeTx) Commit(context.Context) error {
	t.committed = true
	return t.parent.commitErr
}
func (t *rebuildFakeTx) Rollback(context.Context) error {
	t.rolledBack = true
	return nil
}
func (t *rebuildFakeTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (t *rebuildFakeTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults { return nil }
func (t *rebuildFakeTx) LargeObjects() pgx.LargeObjects                         { return pgx.LargeObjects{} }
func (t *rebuildFakeTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (t *rebuildFakeTx) Exec(_ context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	t.parent.record(sql, args, true)
	return t.parent.execResult(sql)
}
func (t *rebuildFakeTx) Query(_ context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	t.parent.record(sql, args, true)
	return &fakeEmptyRows{}, nil
}
func (t *rebuildFakeTx) QueryRow(_ context.Context, sql string, args ...interface{}) pgx.Row {
	t.parent.record(sql, args, true)
	return t.parent.row()
}
func (t *rebuildFakeTx) Conn() *pgx.Conn { return nil }

// The allowlist is the only way to name a rebuild target. A caller holding an
// arbitrary table name — above all knowledge_events or the dedupe registry —
// must not be able to turn it into a target, because ProjectionRebuildTarget's
// fields are unexported and LookupRebuildTarget resolves names, not tables.
func TestLookupRebuildTarget_AllowlistIsClosed(t *testing.T) {
	cases := []struct {
		name              string
		target            string
		wantErr           bool
		wantTables        []string
		wantProjectorName string
	}{
		{
			name:              "knowledge home read models",
			target:            "knowledge-home",
			wantTables:        []string{"knowledge_home_items", "today_digest_view", "recall_candidate_view"},
			wantProjectorName: "knowledge-home-projector",
		},
		{
			name:              "knowledge trail read models",
			target:            "knowledge-trail",
			wantTables:        []string{"knowledge_trail_footprints", "knowledge_trail_branches", "knowledge_trail_act_outcomes"},
			wantProjectorName: "knowledge-trail-projector",
		},
		{name: "the event log is not a rebuild target", target: "knowledge_events", wantErr: true},
		{name: "the dedupe registry is not a rebuild target", target: "knowledge_event_dedupes", wantErr: true},
		{name: "a read-model table name is not a target name", target: "knowledge_home_items", wantErr: true},
		{name: "empty", target: "", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := LookupRebuildTarget(tc.target)
			if tc.wantErr {
				require.Error(t, err, "%q must not resolve to a rebuild target", tc.target)
				assert.True(t, got.IsZero(), "a rejected lookup must not hand back a usable target")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.target, got.Name())
			assert.Equal(t, tc.wantTables, got.Tables())
			assert.Equal(t, tc.wantProjectorName, got.ProjectorName())
		})
	}
}

// Tables() must hand back a copy. Returning the backing array would let any
// caller rewrite an allowlisted entry to knowledge_events in place, which is
// the same hole the unexported fields exist to close.
func TestRebuildTarget_TablesIsACopy(t *testing.T) {
	target, err := LookupRebuildTarget("knowledge-home")
	require.NoError(t, err)

	tables := target.Tables()
	require.NotEmpty(t, tables)
	tables[0] = "knowledge_events"

	assert.Equal(t, "knowledge_home_items", target.Tables()[0],
		"mutating the returned slice must not reach the allowlist")
}

// The PM-2026-010 pin, and the reason this operation exists in code at all:
// the TRUNCATE of the read models and the checkpoint reset must be one
// transaction. Run apart, the always-running in-process projector advances the
// checkpoint in the gap and the events in that gap are never folded into the
// rebuilt model (~326 articles lost, 2026-02).
func TestRebuildProjection_TruncateAndCheckpointResetShareOneTransaction(t *testing.T) {
	mock := &rebuildMockPgx{checkpoint: 1379513}
	repo := &Repository{pool: mock}

	target, err := LookupRebuildTarget("knowledge-trail")
	require.NoError(t, err)

	result, err := repo.RebuildProjection(context.Background(), target)
	require.NoError(t, err)

	assert.Equal(t, 1, mock.beginCalls, "exactly one transaction")
	require.NotNil(t, mock.tx)
	assert.True(t, mock.tx.committed, "the rebuild must commit")

	assert.Empty(t, mock.offTx(),
		"every statement must run on the transaction; anything on the pool is outside the atomic swap")

	truncateIdx := mock.indexOf("TRUNCATE")
	resetIdx := mock.indexOf("last_event_seq = 0")
	require.NotEqual(t, -1, truncateIdx, "the read models must be truncated")
	require.NotEqual(t, -1, resetIdx, "the checkpoint must be reset to 0")
	assert.Less(t, truncateIdx, resetIdx, "truncate first, then reset, both inside the transaction")

	for _, s := range mock.onTx() {
		assert.NotContains(t, s.SQL, "knowledge_events",
			"the rebuild transaction must never name the event log")
		assert.NotContains(t, s.SQL, "knowledge_event_dedupes",
			"the rebuild transaction must never name the dedupe registry")
	}

	truncate := mock.stmts[truncateIdx].SQL
	for _, table := range target.Tables() {
		assert.Contains(t, truncate, table)
	}

	// The checkpoint row is locked before anything is truncated, so the live
	// projector's own checkpoint write serialises behind the rebuild instead
	// of landing in the middle of it.
	lockIdx := mock.indexOf("FOR UPDATE")
	require.NotEqual(t, -1, lockIdx, "the checkpoint row must be locked inside the transaction")
	assert.Less(t, lockIdx, truncateIdx, "lock the checkpoint before truncating")

	assert.Equal(t, "knowledge-trail", result.Target)
	assert.Equal(t, "knowledge-trail-projector", result.ProjectorName)
	assert.Equal(t, target.Tables(), result.Tables)
	assert.Equal(t, 3, result.TablesTruncated)
	assert.Equal(t, int64(1379513), result.CheckpointBefore,
		"the operator must be told which checkpoint value the rebuild reset from")
}

// A failed TRUNCATE must roll the transaction back with the checkpoint intact —
// never "checkpoint reset, tables still full", which would silently skip the
// whole history on the next projector tick.
func TestRebuildProjection_TruncateFailureRollsBackWithCheckpointIntact(t *testing.T) {
	mock := &rebuildMockPgx{
		checkpoint: 42,
		execErrFor: "TRUNCATE",
		execErr:    errors.New("lock timeout"),
	}
	repo := &Repository{pool: mock}

	target, err := LookupRebuildTarget("knowledge-home")
	require.NoError(t, err)

	_, err = repo.RebuildProjection(context.Background(), target)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lock timeout", "the driver error must be wrapped, not swallowed")

	require.NotNil(t, mock.tx)
	assert.False(t, mock.tx.committed, "a failed truncate must not commit")
	assert.True(t, mock.tx.rolledBack, "a failed truncate must roll back")
	assert.Equal(t, -1, mock.indexOf("last_event_seq = 0"),
		"the checkpoint reset must not run once the truncate has failed")
}

// The mirror case: a failed checkpoint reset must roll the TRUNCATE back, never
// leave empty read models behind an already-advanced checkpoint.
func TestRebuildProjection_CheckpointResetFailureRollsBackTheTruncate(t *testing.T) {
	mock := &rebuildMockPgx{
		checkpoint: 42,
		execErrFor: "last_event_seq = 0",
		execErr:    errors.New("deadlock detected"),
	}
	repo := &Repository{pool: mock}

	target, err := LookupRebuildTarget("knowledge-home")
	require.NoError(t, err)

	_, err = repo.RebuildProjection(context.Background(), target)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "deadlock detected")

	require.NotNil(t, mock.tx)
	assert.False(t, mock.tx.committed, "a failed checkpoint reset must not commit")
	assert.True(t, mock.tx.rolledBack, "a failed checkpoint reset must roll the truncate back")
}

// A commit failure must reach the operator. Reporting success here would mean
// "rebuild done" in the logs over an untouched projection.
func TestRebuildProjection_CommitFailureIsReported(t *testing.T) {
	mock := &rebuildMockPgx{checkpoint: 7, commitErr: errors.New("connection reset")}
	repo := &Repository{pool: mock}

	target, err := LookupRebuildTarget("knowledge-home")
	require.NoError(t, err)

	_, err = repo.RebuildProjection(context.Background(), target)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection reset")
}

// Defence in depth behind the unexported fields: even a target built inside
// this package must not be able to truncate the event log or the dedupe
// registry, and the refusal must happen before a transaction is opened.
func TestRebuildProjection_RefusesSourceOfTruthTables(t *testing.T) {
	cases := []struct {
		name   string
		target ProjectionRebuildTarget
	}{
		{
			name:   "the event log",
			target: ProjectionRebuildTarget{name: "rogue", tables: []string{"knowledge_events"}, projectorName: "knowledge-home-projector"},
		},
		{
			name:   "the dedupe registry",
			target: ProjectionRebuildTarget{name: "rogue", tables: []string{"knowledge_home_items", "knowledge_event_dedupes"}, projectorName: "knowledge-home-projector"},
		},
		{
			name:   "the user event log",
			target: ProjectionRebuildTarget{name: "rogue", tables: []string{"knowledge_user_events"}, projectorName: "knowledge-home-projector"},
		},
		{
			name:   "the zero value",
			target: ProjectionRebuildTarget{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := &rebuildMockPgx{}
			repo := &Repository{pool: mock}

			_, err := repo.RebuildProjection(context.Background(), tc.target)
			require.Error(t, err)
			assert.Equal(t, 0, mock.beginCalls, "the refusal must happen before any transaction is opened")
			assert.Empty(t, mock.stmts, "no statement may be issued for a refused target")
		})
	}
}

// An operator reading the logs must be able to see that a rebuild started, that
// it finished, which target it touched, how many tables it truncated and which
// checkpoint value it reset from.
func TestRebuildProjection_LogsStartAndFinish(t *testing.T) {
	var buf bytes.Buffer
	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(restore) })

	mock := &rebuildMockPgx{checkpoint: 1379513}
	repo := &Repository{pool: mock}

	target, err := LookupRebuildTarget("knowledge-home")
	require.NoError(t, err)

	_, err = repo.RebuildProjection(context.Background(), target)
	require.NoError(t, err)

	records := map[string]map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		msg, _ := rec["msg"].(string)
		records[msg] = rec
	}

	start, ok := records["projection.rebuild.start"]
	require.True(t, ok, "a rebuild must log that it started; got %s", buf.String())
	assert.Equal(t, "knowledge-home", start["target"])
	assert.Equal(t, float64(3), start["tables"])
	assert.Equal(t, "knowledge-home-projector", start["projector_name"])

	done, ok := records["projection.rebuild.done"]
	require.True(t, ok, "a rebuild must log that it finished; got %s", buf.String())
	assert.Equal(t, "knowledge-home", done["target"])
	assert.Equal(t, float64(3), done["tables"])
	assert.Equal(t, "knowledge-home-projector", done["projector_name"])
	assert.Equal(t, float64(1379513), done["checkpoint_reset_from"],
		"the finish log must carry the checkpoint value the rebuild reset from")
}

// A failed rebuild must not log a finish line — that is what an operator reads
// as "the rebuild completed".
func TestRebuildProjection_DoesNotLogFinishOnFailure(t *testing.T) {
	var buf bytes.Buffer
	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(restore) })

	mock := &rebuildMockPgx{execErrFor: "TRUNCATE", execErr: errors.New("lock timeout")}
	repo := &Repository{pool: mock}

	target, err := LookupRebuildTarget("knowledge-trail")
	require.NoError(t, err)

	_, err = repo.RebuildProjection(context.Background(), target)
	require.Error(t, err)
	assert.NotContains(t, buf.String(), "projection.rebuild.done")
}

// RebuildTargets is what the admin surface offers an operator; it must list
// exactly the allowlist and nothing that reaches the event log.
func TestRebuildTargets_ListsOnlyProjections(t *testing.T) {
	targets := RebuildTargets()
	require.Len(t, targets, 2)

	names := make([]string, 0, len(targets))
	for _, tgt := range targets {
		names = append(names, tgt.Name())
		for _, table := range tgt.Tables() {
			assert.NotContains(t, protectedTables, table,
				"%s lists a source-of-truth table", tgt.Name())
		}
	}
	assert.ElementsMatch(t, []string{"knowledge-home", "knowledge-trail"}, names)
}
