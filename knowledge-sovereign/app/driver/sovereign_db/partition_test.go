package sovereign_db

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPgx implements PgxIface for unit-testing the dedupe registry logic.
type mockPgx struct {
	execCalls    []mockExecCall
	queryRowFunc func(ctx context.Context, sql string, args ...interface{}) pgx.Row
	execFunc     func(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
}

type mockExecCall struct {
	SQL  string
	Args []interface{}
}

func (m *mockPgx) Query(_ context.Context, _ string, _ ...interface{}) (pgx.Rows, error) {
	return nil, nil
}
func (m *mockPgx) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
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

// BeginTx is the Wave 4-D session-var entry point. The mock returns an
// error so any unit test that accidentally enters the user-scoped read
// path fails loudly rather than silently bypassing the isolation. Tests
// that need a real tx mock should use a Postgres-backed integration test
// against the real Repository — the partition / dedupe paths covered by
// this mock never call BeginTx.
func (m *mockPgx) BeginTx(_ context.Context, _ pgx.TxOptions) (pgx.Tx, error) {
	return nil, errors.New("mockPgx.BeginTx: not implemented; use integration test")
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
