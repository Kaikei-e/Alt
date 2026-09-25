package sovereign_db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// mockPgx implements PgxIface for unit-testing the dedupe registry and
// partition DDL logic without a live database.
type mockPgx struct {
	execCalls  []mockExecCall
	queryCalls []mockQueryCall
	// queryRowCalls records QueryRow separately from queryCalls so the
	// partition probes (to_regclass / the default-partition backlog EXISTS)
	// can be asserted without disturbing the Query-based expectations.
	queryRowCalls []mockQueryCall
	queryRowFunc  func(ctx context.Context, sql string, args ...interface{}) pgx.Row
	execFunc      func(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
	queryFunc     func(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	// beginFunc overrides Begin for tests that need to control the
	// transaction (e.g., force a Commit/Rollback error). When nil, Begin
	// returns a *fakeTx that delegates Exec/Query/QueryRow back to this
	// mockPgx so existing SQL-tracking assertions keep working across the
	// Begin/Commit boundary.
	beginFunc func(ctx context.Context) (pgx.Tx, error)
	lastTx    *fakeTx
}

type mockExecCall struct {
	SQL  string
	Args []interface{}
}

type mockQueryCall struct {
	SQL  string
	Args []interface{}
}

func (m *mockPgx) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	m.queryCalls = append(m.queryCalls, mockQueryCall{SQL: sql, Args: args})
	if m.queryFunc != nil {
		return m.queryFunc(ctx, sql, args...)
	}
	return &fakeEmptyRows{}, nil
}

func (m *mockPgx) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	m.queryRowCalls = append(m.queryRowCalls, mockQueryCall{SQL: sql, Args: args})
	if m.queryRowFunc != nil {
		return m.queryRowFunc(ctx, sql, args...)
	}
	return &mockRow{}
}

func (m *mockPgx) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	m.execCalls = append(m.execCalls, mockExecCall{SQL: sql, Args: args})
	if m.execFunc != nil {
		return m.execFunc(ctx, sql, args...)
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

// Begin implements PgxIface.Begin for tests. See beginFunc/fakeTx above.
func (m *mockPgx) Begin(ctx context.Context) (pgx.Tx, error) {
	if m.beginFunc != nil {
		return m.beginFunc(ctx)
	}
	tx := &fakeTx{parent: m}
	m.lastTx = tx
	return tx, nil
}

type mockRow struct {
	scanFunc func(dest ...interface{}) error
}

func (r *mockRow) Scan(dest ...interface{}) error {
	if r.scanFunc != nil {
		return r.scanFunc(dest...)
	}
	// Default: return event_seq = 1
	if len(dest) > 0 {
		if p, ok := dest[0].(*int64); ok {
			*p = 1
		}
	}
	return nil
}

// fakeTx is a minimal pgx.Tx stub for unit-testing transactional repository
// methods (AppendKnowledgeEvent, ActivateProjectionVersion) without a live
// database. Exec/Query/QueryRow delegate to the parent mockPgx so the usual
// execCalls/queryCalls tracking and execFunc/queryRowFunc overrides keep
// working across the Begin/Commit boundary.
type fakeTx struct {
	parent      *mockPgx
	commitErr   error
	rollbackErr error
	committed   bool
	rolledBack  bool
}

func (t *fakeTx) Begin(ctx context.Context) (pgx.Tx, error) { return t, nil }

func (t *fakeTx) Commit(ctx context.Context) error {
	t.committed = true
	return t.commitErr
}

func (t *fakeTx) Rollback(ctx context.Context) error {
	t.rolledBack = true
	return t.rollbackErr
}

func (t *fakeTx) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	return 0, nil
}
func (t *fakeTx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults { return nil }
func (t *fakeTx) LargeObjects() pgx.LargeObjects                               { return pgx.LargeObjects{} }
func (t *fakeTx) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	return nil, nil
}
func (t *fakeTx) Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error) {
	return t.parent.Exec(ctx, sql, arguments...)
}
func (t *fakeTx) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return t.parent.Query(ctx, sql, args...)
}
func (t *fakeTx) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return t.parent.QueryRow(ctx, sql, args...)
}
func (t *fakeTx) Conn() *pgx.Conn { return nil }

// fakeEmptyRows is a zero-row pgx.Rows stub for Query-based repository
// methods under test (e.g., GetRecallCandidates) that only need SQL-text
// assertions, not real result data.
type fakeEmptyRows struct{}

func (r *fakeEmptyRows) Close()                                       {}
func (r *fakeEmptyRows) Err() error                                   { return nil }
func (r *fakeEmptyRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *fakeEmptyRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *fakeEmptyRows) Next() bool                                   { return false }
func (r *fakeEmptyRows) Scan(dest ...any) error                       { return nil }
func (r *fakeEmptyRows) Values() ([]any, error)                       { return nil, nil }
func (r *fakeEmptyRows) RawValues() [][]byte                          { return nil }
func (r *fakeEmptyRows) Conn() *pgx.Conn                              { return nil }
