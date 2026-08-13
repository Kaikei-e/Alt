package alt_db

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/require"
)

// The outbox claim has the same crash-recovery obligation as the push
// delivery queue, and the same answer: the claim predicate itself must reach
// rows a dead worker left mid-flight. A claim that reads only PENDING strands
// them, because nothing else ever moves a PROCESSING row — the prune deletes
// PROCESSED only, and the release back to PENDING lives in the worker that
// just died. Those articles are never RAG-indexed and their ArticleCreated is
// never emitted.
func TestFetchAndLockPendingOutboxEvents_ReclaimsExpiredLeases(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &OutboxRepository{pool: mock}
	eventID := uuid.MustParse("8f14e45f-ceea-467a-9d0c-1a2b3c4d5e6f")

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)FROM outbox_events.*'PROCESSING'.*next_attempt_at <= clock_timestamp\(\)`).
		WithArgs(10).
		WillReturnRows(pgxmock.NewRows([]string{"id", "event_type", "payload", "status", "created_at"}).
			AddRow(eventID, "ARTICLE_UPSERT", []byte(`{"article_id":"a"}`), "PROCESSING", time.Unix(1700000000, 0)))
	mock.ExpectExec(`(?s)UPDATE outbox_events.*status = 'PROCESSING'.*next_attempt_at = clock_timestamp\(\) \+ make_interval`).
		WithArgs(eventID.String(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()

	events, err := repo.FetchAndLockPendingOutboxEvents(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, eventID.String(), events[0].ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Releasing a claim must also drop the lease. The worker releases a row it
// could not deliver so the next tick five seconds later retries it; leaving
// next_attempt_at at the claim's lease horizon would stall every retry for
// the length of the lease and blow the worker's attempt budget on waiting.
func TestUpdateOutboxEventStatus_ReleaseClearsTheLease(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	repo := &OutboxRepository{pool: mock}
	id := "8f14e45f-ceea-467a-9d0c-1a2b3c4d5e6f"

	mock.ExpectExec(`(?s)UPDATE outbox_events.*next_attempt_at = CASE WHEN \$1 = 'PENDING' THEN clock_timestamp\(\)`).
		WithArgs("PENDING", nil, (*string)(nil), id).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	require.NoError(t, repo.UpdateOutboxEventStatus(context.Background(), id, "PENDING", nil))
	require.NoError(t, mock.ExpectationsWereMet())
}
