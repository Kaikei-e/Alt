package sovereign_db

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestAppendKnowledgeEvent_DedupeRegistryInsert(t *testing.T) {
	// After partitioning, AppendKnowledgeEvent should:
	// 1. Try to INSERT into knowledge_event_dedupes first
	// 2. If dedupe succeeds (no conflict), INSERT into knowledge_events
	// 3. Return event_seq from the INSERT
	// This test verifies the dedupe registry is used for idempotency.

	t.Run("new event inserts into dedupes then events", func(t *testing.T) {
		mock := &mockPgx{}
		dedupeInserted := false
		eventInserted := false

		mock.execFunc = func(_ context.Context, sql string, _ ...interface{}) (pgconn.CommandTag, error) {
			if containsSQL(sql, "knowledge_event_dedupes") {
				dedupeInserted = true
				return pgconn.NewCommandTag("INSERT 0 1"), nil
			}
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		}
		mock.queryRowFunc = func(_ context.Context, sql string, _ ...interface{}) pgx.Row {
			if containsSQL(sql, "knowledge_events") {
				eventInserted = true
			}
			return &mockRow{scanFunc: func(dest ...interface{}) error {
				if len(dest) > 0 {
					if p, ok := dest[0].(*int64); ok {
						*p = 42
					}
				}
				return nil
			}}
		}

		repo := NewRepository(mock)
		event := KnowledgeEvent{
			EventID:       uuid.New(),
			OccurredAt:    time.Now(),
			TenantID:      uuid.New(),
			ActorType:     "system",
			EventType:     "ArticleCreated",
			AggregateType: "article",
			AggregateID:   uuid.New().String(),
			DedupeKey:     "ArticleCreated:" + uuid.New().String(),
			Payload:       json.RawMessage(`{}`),
		}

		seq, err := repo.AppendKnowledgeEvent(context.Background(), event)
		require.NoError(t, err)
		assert.Equal(t, int64(42), seq)
		assert.True(t, dedupeInserted, "should insert into dedupe registry")
		assert.True(t, eventInserted, "should insert into event table")
	})

	t.Run("duplicate event returns 0 without inserting into events", func(t *testing.T) {
		mock := &mockPgx{}
		eventInserted := false

		mock.execFunc = func(_ context.Context, sql string, _ ...interface{}) (pgconn.CommandTag, error) {
			if containsSQL(sql, "knowledge_event_dedupes") {
				// Simulate ON CONFLICT DO NOTHING (0 rows affected)
				return pgconn.NewCommandTag("INSERT 0 0"), nil
			}
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		}
		mock.queryRowFunc = func(_ context.Context, sql string, _ ...interface{}) pgx.Row {
			if containsSQL(sql, "knowledge_events") {
				eventInserted = true
			}
			return &mockRow{}
		}

		repo := NewRepository(mock)
		event := KnowledgeEvent{
			EventID:       uuid.New(),
			OccurredAt:    time.Now(),
			TenantID:      uuid.New(),
			ActorType:     "system",
			EventType:     "ArticleCreated",
			AggregateType: "article",
			AggregateID:   uuid.New().String(),
			DedupeKey:     "ArticleCreated:" + uuid.New().String(),
			Payload:       json.RawMessage(`{}`),
		}

		seq, err := repo.AppendKnowledgeEvent(context.Background(), event)
		require.NoError(t, err)
		assert.Equal(t, int64(0), seq, "duplicate should return 0")
		assert.False(t, eventInserted, "duplicate should NOT insert into event table")
	})
}

func TestEnsurePartitions_GeneratesCorrectRanges(t *testing.T) {
	// EnsurePartitions should create monthly partition tables
	// covering from the given start month to target month + 1 (pre-create next).

	t.Run("generates correct partition DDL for given range", func(t *testing.T) {
		partitions := GeneratePartitionDDL("knowledge_events", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), 2)
		require.Len(t, partitions, 2)

		assert.Contains(t, partitions[0].Name, "knowledge_events_y2026m03")
		assert.Contains(t, partitions[0].DDL, "FOR VALUES FROM ('2026-03-01')")
		assert.Contains(t, partitions[0].DDL, "TO ('2026-04-01')")

		assert.Contains(t, partitions[1].Name, "knowledge_events_y2026m04")
		assert.Contains(t, partitions[1].DDL, "FOR VALUES FROM ('2026-04-01')")
		assert.Contains(t, partitions[1].DDL, "TO ('2026-05-01')")
	})

	t.Run("handles year boundary", func(t *testing.T) {
		partitions := GeneratePartitionDDL("knowledge_events", time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), 2)
		require.Len(t, partitions, 2)

		assert.Contains(t, partitions[0].Name, "knowledge_events_y2026m12")
		assert.Contains(t, partitions[1].Name, "knowledge_events_y2027m01")
	})
}

// partitionNameFromDDL pulls the partition table name out of a
// "CREATE TABLE IF NOT EXISTS <name> PARTITION OF ..." statement.
func partitionNameFromDDL(ddl string) string {
	fields := strings.Fields(ddl)
	if len(fields) < 6 {
		return ""
	}
	return fields[5]
}

// newPartitionMock returns a mockPgx whose to_regclass probe answers from
// `existing` and whose CREATE TABLE execs add to it — a stand-in for the
// catalog that survives repeated EnsurePartitions runs. No month is blocked by
// a default-partition backlog.
func newPartitionMock(existing map[string]bool) *mockPgx {
	return newPartitionMockWithBacklog(existing, nil)
}

// newPartitionMockWithBacklog additionally answers the backlog probe: `backlog`
// is keyed by the month ("2006-01") whose range the parent table still holds
// rows for, i.e. the months whose CREATE the DEFAULT partition would doom.
func newPartitionMockWithBacklog(existing map[string]bool, backlog map[string]bool) *mockPgx {
	mock := &mockPgx{}
	mock.queryRowFunc = func(_ context.Context, sql string, args ...interface{}) pgx.Row {
		return &mockRow{scanFunc: func(dest ...interface{}) error {
			p, ok := dest[0].(*bool)
			if !ok || len(args) == 0 {
				return nil
			}
			if strings.Contains(sql, "to_regclass") {
				name, _ := args[0].(string)
				*p = existing[name]
				return nil
			}
			from, _ := args[0].(string) // '2006-01-02', as the DDL bounds are
			if len(from) >= 7 {
				*p = backlog[from[:7]]
			}
			return nil
		}}
	}
	mock.execFunc = func(_ context.Context, sql string, _ ...interface{}) (pgconn.CommandTag, error) {
		if strings.HasPrefix(sql, "CREATE TABLE") {
			existing[partitionNameFromDDL(sql)] = true
			return pgconn.NewCommandTag("CREATE TABLE"), nil
		}
		return pgconn.NewCommandTag("SELECT 1"), nil
	}
	return mock
}

// backlogProbes lists the default-partition backlog probes a mock answered.
func backlogProbes(mock *mockPgx) []mockQueryCall {
	var probes []mockQueryCall
	for _, q := range mock.queryRowCalls {
		if strings.Contains(q.SQL, "SELECT EXISTS") {
			probes = append(probes, q)
		}
	}
	return probes
}

// createdPartitions lists the partitions a mock actually ran CREATE TABLE for.
func createdPartitions(mock *mockPgx) []string {
	names := []string{}
	for _, c := range mock.execCalls {
		if strings.HasPrefix(c.SQL, "CREATE TABLE") {
			names = append(names, partitionNameFromDDL(c.SQL))
		}
	}
	return names
}

// createStatements lists the CREATE TABLE statements a mock ran, in order.
func createStatements(mock *mockPgx) []string {
	ddls := []string{}
	for _, c := range mock.execCalls {
		if strings.HasPrefix(c.SQL, "CREATE TABLE") {
			ddls = append(ddls, c.SQL)
		}
	}
	return ddls
}

// GeneratePartitionDDL had no production caller: migrations 00006/00007 stop at
// TO ('2026-05-01') and nothing ever created the next month, so every event
// after that month landed in knowledge_events_default. EnsurePartitions is that
// caller. It must create only the absent months (so the default-partition scan
// each CREATE forces happens once per month, not once per tick) and must
// serialize concurrent replicas on an advisory lock, because two concurrent
// CREATE TABLE IF NOT EXISTS still race into a pg_type unique violation.
func TestEnsurePartitions(t *testing.T) {
	july := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	t.Run("creates every absent month under an advisory lock", func(t *testing.T) {
		mock := newPartitionMock(map[string]bool{})
		repo := NewRepository(mock)

		created, err := repo.EnsurePartitions(context.Background(), "knowledge_events", july, 3)
		require.NoError(t, err)
		assert.Equal(t, []string{
			"knowledge_events_y2026m07",
			"knowledge_events_y2026m08",
			"knowledge_events_y2026m09",
		}, created)

		// Every CREATE must be preceded by the advisory lock in the same
		// transaction; otherwise two replicas booting together collide.
		var pairs [][2]string
		for i := 0; i+1 < len(mock.execCalls); i++ {
			if strings.HasPrefix(mock.execCalls[i+1].SQL, "CREATE TABLE") {
				pairs = append(pairs, [2]string{mock.execCalls[i].SQL, mock.execCalls[i+1].SQL})
			}
		}
		require.Len(t, pairs, 3, "each created partition needs a preceding statement")
		for _, p := range pairs {
			assert.Contains(t, p[0], "pg_advisory_xact_lock", "CREATE must run under the advisory lock")
		}

		ddls := createStatements(mock)
		require.Len(t, ddls, 3)
		assert.Contains(t, ddls[0], "FOR VALUES FROM ('2026-07-01') TO ('2026-08-01')")
		assert.Contains(t, ddls[1], "FOR VALUES FROM ('2026-08-01') TO ('2026-09-01')")
		assert.Contains(t, ddls[2], "FOR VALUES FROM ('2026-09-01') TO ('2026-10-01')")
	})

	// The CREATE takes ACCESS EXCLUSIVE on the parent — the hot append path —
	// and there is no lock_timeout or statement_timeout anywhere else in this
	// service or in DATABASE_URL. Unbounded, the 6-hourly attempt can queue
	// behind a long reader and block every knowledge_events INSERT for as long
	// as that reader runs. SET LOCAL caps the wait to the transaction.
	t.Run("bounds the lock wait before it asks for the parent lock", func(t *testing.T) {
		mock := newPartitionMock(map[string]bool{})
		repo := NewRepository(mock)

		created, err := repo.EnsurePartitions(context.Background(), "knowledge_events", july, 1)
		require.NoError(t, err)
		require.Len(t, created, 1)

		require.Len(t, mock.execCalls, 3, "expected exactly SET LOCAL, advisory lock, CREATE")
		assert.Contains(t, mock.execCalls[0].SQL, "SET LOCAL lock_timeout",
			"the lock wait must be bounded before anything queues for a lock")
		assert.Contains(t, mock.execCalls[1].SQL, "pg_advisory_xact_lock")
		assert.True(t, strings.HasPrefix(mock.execCalls[2].SQL, "CREATE TABLE"))
	})

	t.Run("is idempotent: a second run creates nothing", func(t *testing.T) {
		existing := map[string]bool{}

		first := newPartitionMock(existing)
		created, err := NewRepository(first).EnsurePartitions(context.Background(), "knowledge_events", july, 3)
		require.NoError(t, err)
		require.Len(t, created, 3)

		second := newPartitionMock(existing)
		created, err = NewRepository(second).EnsurePartitions(context.Background(), "knowledge_events", july, 3)
		require.NoError(t, err)
		assert.Empty(t, created, "second run must report nothing created")
		assert.Empty(t, second.execCalls,
			"a month that already exists must cost the catalog probe and nothing else — no transaction, no lock, no CREATE")
		assert.Empty(t, backlogProbes(second),
			"an existing month must not be probed against the parent table")
	})

	t.Run("skips the months that already exist", func(t *testing.T) {
		mock := newPartitionMock(map[string]bool{"knowledge_events_y2026m08": true})
		repo := NewRepository(mock)

		created, err := repo.EnsurePartitions(context.Background(), "knowledge_events", july, 3)
		require.NoError(t, err)
		assert.Equal(t, []string{
			"knowledge_events_y2026m07",
			"knowledge_events_y2026m09",
		}, created)
	})

	t.Run("rolls over the year boundary", func(t *testing.T) {
		mock := newPartitionMock(map[string]bool{})
		repo := NewRepository(mock)

		created, err := repo.EnsurePartitions(context.Background(), "knowledge_user_events",
			time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), 3)
		require.NoError(t, err)
		assert.Equal(t, []string{
			"knowledge_user_events_y2026m12",
			"knowledge_user_events_y2027m01",
			"knowledge_user_events_y2027m02",
		}, created)
		ddls := createStatements(mock)
		require.Len(t, ddls, 3)
		assert.Contains(t, ddls[0], "FOR VALUES FROM ('2026-12-01') TO ('2027-01-01')")
		assert.Contains(t, ddls[1], "FOR VALUES FROM ('2027-01-01') TO ('2027-02-01')")
	})

	t.Run("a failing month does not block the later ones", func(t *testing.T) {
		// The real failure this guards: while the default partition still
		// holds the un-partitioned backlog, creating the *current* month
		// fails (its rows live in default), but the future months — the
		// ones that stop the bleeding — must still be created.
		mock := newPartitionMock(map[string]bool{})
		inner := mock.execFunc
		mock.execFunc = func(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "knowledge_events_y2026m07") && strings.HasPrefix(sql, "CREATE TABLE") {
				return pgconn.CommandTag{}, errors.New("updated partition constraint for default partition would be violated by some row")
			}
			return inner(ctx, sql, args...)
		}
		repo := NewRepository(mock)

		created, err := repo.EnsurePartitions(context.Background(), "knowledge_events", july, 3)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "knowledge_events_y2026m07")
		assert.Equal(t, []string{
			"knowledge_events_y2026m08",
			"knowledge_events_y2026m09",
		}, created, "later months must still be created")
	})

	// The month whose rows are still in the DEFAULT partition is *known* to
	// fail on this database until an operator splits the backlog. Discovering
	// that by running the CREATE costs ACCESS EXCLUSIVE on the parent plus a
	// scan of the default partition, every tick, forever, on the hot append
	// path — the recurring version of the very lock the task calls urgent.
	// The same question is answerable under ACCESS SHARE: no partition covers
	// this range yet, so a row inside it can only be sitting in default.
	t.Run("refuses a month the default-partition backlog dooms, without taking the lock", func(t *testing.T) {
		mock := newPartitionMockWithBacklog(map[string]bool{}, map[string]bool{"2026-07": true})
		repo := NewRepository(mock)

		created, err := repo.EnsurePartitions(context.Background(), "knowledge_events", july, 3)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrDefaultPartitionBacklog)
		assert.Contains(t, err.Error(), "knowledge_events_y2026m07")
		assert.Equal(t, []string{
			"knowledge_events_y2026m08",
			"knowledge_events_y2026m09",
		}, created, "the months that stop the bleeding must still be created")

		// Two months created, three statements each, and nothing at all for
		// the doomed one: no transaction, no advisory lock, no CREATE.
		assert.Len(t, mock.execCalls, 6)
		assert.Equal(t, []string{
			"knowledge_events_y2026m08",
			"knowledge_events_y2026m09",
		}, createdPartitions(mock))

		// The probe must ask the parent for exactly the partition's range, and
		// must express the bounds the way the DDL does — the same date
		// literals through the same timestamptz cast, so probe and partition
		// bound cannot disagree on a server whose TimeZone is not UTC.
		probes := backlogProbes(mock)
		require.NotEmpty(t, probes)
		assert.Contains(t, probes[0].SQL, "FROM knowledge_events")
		assert.Contains(t, probes[0].SQL, "occurred_at >= $1::timestamptz")
		assert.Contains(t, probes[0].SQL, "occurred_at < $2::timestamptz")
		require.Len(t, probes[0].Args, 2)
		assert.Equal(t, "2026-07-01", probes[0].Args[0])
		assert.Equal(t, "2026-08-01", probes[0].Args[1])
	})

	// Self-healing: the tick after the operator split procedure empties the
	// default partition, the same month is created with no restart and no
	// process-lifetime memory to clear.
	t.Run("creates the month once the backlog is gone", func(t *testing.T) {
		backlog := map[string]bool{"2026-07": true}
		existing := map[string]bool{}

		_, err := NewRepository(newPartitionMockWithBacklog(existing, backlog)).
			EnsurePartitions(context.Background(), "knowledge_events", july, 1)
		require.ErrorIs(t, err, ErrDefaultPartitionBacklog)

		delete(backlog, "2026-07") // operator ran the split
		after := newPartitionMockWithBacklog(existing, backlog)
		created, err := NewRepository(after).EnsurePartitions(context.Background(), "knowledge_events", july, 1)
		require.NoError(t, err)
		assert.Equal(t, []string{"knowledge_events_y2026m07"}, created)
	})
}

// containsSQL checks if a SQL string contains a substring (case-insensitive-ish).
func containsSQL(sql, substr string) bool {
	return len(sql) > 0 && len(substr) > 0 && contains(sql, substr)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
