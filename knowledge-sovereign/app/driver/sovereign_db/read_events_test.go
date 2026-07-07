package sovereign_db

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestEvent() KnowledgeEvent {
	return KnowledgeEvent{
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
}

// TestAppendKnowledgeEvent_CommitsBothInsertsTogether pins the fix for the
// split-transaction bug: the dedupe-key INSERT and the knowledge_events
// INSERT must run inside one Begin/Commit so a crash between them can never
// leave the dedupe key registered with no corresponding event row (which
// would permanently lose the event — any resend is then treated as an
// already-applied duplicate and silently dropped).
func TestAppendKnowledgeEvent_CommitsBothInsertsTogether(t *testing.T) {
	mock := &mockPgx{}
	mock.queryRowFunc = func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
		return &mockRow{scanFunc: func(dest ...interface{}) error {
			if p, ok := dest[0].(*int64); ok {
				*p = 7
			}
			return nil
		}}
	}

	repo := NewRepository(mock)
	seq, err := repo.AppendKnowledgeEvent(context.Background(), newTestEvent())

	require.NoError(t, err)
	assert.Equal(t, int64(7), seq)
	require.NotNil(t, mock.lastTx, "AppendKnowledgeEvent must open a transaction via Begin")
	assert.True(t, mock.lastTx.committed, "both INSERTs must be committed together")
}

// TestAppendKnowledgeEvent_RollsBackOnEventInsertFailure verifies that if
// the second INSERT (knowledge_events) fails after the dedupe key INSERT
// succeeded, the transaction is rolled back — so the dedupe key is NOT left
// registered with no corresponding event (the exact bug this fix closes).
func TestAppendKnowledgeEvent_RollsBackOnEventInsertFailure(t *testing.T) {
	mock := &mockPgx{}
	mock.queryRowFunc = func(_ context.Context, _ string, _ ...interface{}) pgx.Row {
		return &mockRow{scanFunc: func(dest ...interface{}) error {
			return fmt.Errorf("simulated event insert failure")
		}}
	}

	repo := NewRepository(mock)
	_, err := repo.AppendKnowledgeEvent(context.Background(), newTestEvent())

	require.Error(t, err)
	require.NotNil(t, mock.lastTx)
	assert.True(t, mock.lastTx.rolledBack, "the transaction must be rolled back when the event insert fails")
	assert.False(t, mock.lastTx.committed, "must not commit a half-applied transaction")
}

// TestAppendKnowledgeEvent_DuplicateReturnsZero verifies the existing
// idempotency contract still holds: a duplicate dedupe key returns 0.
func TestAppendKnowledgeEvent_DuplicateReturnsZero(t *testing.T) {
	mock := &mockPgx{}
	mock.execFunc = func(_ context.Context, sql string, _ ...interface{}) (pgconn.CommandTag, error) {
		if containsSQL(sql, "knowledge_event_dedupes") {
			return pgconn.NewCommandTag("INSERT 0 0"), nil // ON CONFLICT DO NOTHING: 0 rows
		}
		return pgconn.NewCommandTag("INSERT 0 1"), nil
	}

	repo := NewRepository(mock)
	seq, err := repo.AppendKnowledgeEvent(context.Background(), newTestEvent())

	require.NoError(t, err)
	assert.Equal(t, int64(0), seq)
}
