package repository

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var outboxTickAt = time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)

func newMockOutboxRepo(t *testing.T) (*notificationOutboxRepository, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(mock.Close)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	return &notificationOutboxRepository{db: mock, logger: logger}, mock
}

// TestClaimBatch_CommitsBeforeReturning is what keeps a network call from
// happening inside an open transaction: the claim transaction is finished by
// the time ClaimBatch hands rows back, so the caller physically cannot hold
// it across the forward RPC.
func TestClaimBatch_CommitsBeforeReturning(t *testing.T) {
	repo, mock := newMockOutboxRepo(t)

	rows := pgxmock.NewRows([]string{"id", "dedupe_key", "user_id", "kind", "payload", "occurred_at", "attempts"}).
		AddRow("row-1", "summary:job-1", producerUserID, "summary_ready",
			[]byte(`{"kind":"summary_ready","url":"/articles/art-001"}`), outboxTickAt.Add(-time.Minute), 1)

	mock.ExpectBegin()
	mock.ExpectQuery(`FOR UPDATE SKIP LOCKED`).
		WithArgs("relay-1", (2 * time.Minute).Seconds(), 10).
		WillReturnRows(rows)
	mock.ExpectCommit()

	claimed, err := repo.ClaimBatch(context.Background(), "relay-1", 10, 2*time.Minute)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet(),
		"the claim transaction must be committed before ClaimBatch returns")
	require.Len(t, claimed, 1)
	assert.Equal(t, "row-1", claimed[0].ID)
	assert.Equal(t, "summary:job-1", claimed[0].DedupeKey)
	assert.Equal(t, 1, claimed[0].Attempts)
}

// TestClaimBatchQuery_ShapeInvariants pins the parts of the claim statement
// that make concurrent relays and crashed relays safe.
func TestClaimBatchQuery_ShapeInvariants(t *testing.T) {
	assert.Contains(t, claimNotificationBatchQuery, "FOR UPDATE SKIP LOCKED",
		"concurrent relays must not block on each other's claimed rows")
	assert.Contains(t, claimNotificationBatchQuery, "state IN ('pending', 'forwarding')",
		"a row orphaned mid-forward by a crashed relay must re-enter the claim query")
	assert.Contains(t, claimNotificationBatchQuery, "attempts = attempts + 1",
		"attempts must advance at claim time, or a relay that dies mid-RPC never reaches the dead state")
	assert.NotContains(t, claimNotificationBatchQuery, "next_attempt_at <= $",
		"due-ness is decided by the database clock, not by the relay host's")
}

func TestMarkForwarded_ClearsTheLease(t *testing.T) {
	repo, mock := newMockOutboxRepo(t)

	mock.ExpectExec(`UPDATE notification_outbox`).
		WithArgs("row-1").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	require.NoError(t, repo.MarkForwarded(context.Background(), "row-1"))
	require.NoError(t, mock.ExpectationsWereMet())
	assert.Contains(t, markNotificationForwardedQuery, "locked_by = NULL")
}

// TestMarkAttemptFailed_SchedulesTheRetry passes a delay rather than an
// instant, so the retry lands relative to the database's own clock.
func TestMarkAttemptFailed_SchedulesTheRetry(t *testing.T) {
	repo, mock := newMockOutboxRepo(t)

	mock.ExpectExec(`UPDATE notification_outbox`).
		WithArgs((40 * time.Second).Seconds(), "connection refused", "row-1").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	require.NoError(t, repo.MarkAttemptFailed(context.Background(), "row-1", 40*time.Second, "connection refused"))
	require.NoError(t, mock.ExpectationsWereMet())
	assert.Contains(t, markNotificationAttemptFailedQuery, "state = 'pending'")
	assert.Contains(t, markNotificationAttemptFailedQuery, "clock_timestamp() + make_interval")
}

func TestMarkDead_IsTerminal(t *testing.T) {
	repo, mock := newMockOutboxRepo(t)

	mock.ExpectExec(`UPDATE notification_outbox`).
		WithArgs("gave up after 8 attempts", "row-1").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	require.NoError(t, repo.MarkDead(context.Background(), "row-1", "gave up after 8 attempts"))
	require.NoError(t, mock.ExpectationsWereMet())
	assert.Contains(t, markNotificationDeadQuery, "state = 'dead'")
}

// TestOldestPendingAge_ReturnsZeroOnEmptyBacklog matters because the relay
// publishes this number as a gauge on every tick: an empty backlog has to
// produce a real 0, not an error the caller turns into "skip the update".
func TestOldestPendingAge_ReturnsZeroOnEmptyBacklog(t *testing.T) {
	repo, mock := newMockOutboxRepo(t)

	mock.ExpectQuery(`FROM notification_outbox`).
		WillReturnRows(pgxmock.NewRows([]string{"age_seconds"}).AddRow(float64(0)))

	age, err := repo.OldestPendingAge(context.Background())

	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), age)
	require.NoError(t, mock.ExpectationsWereMet())
	assert.True(t, strings.Contains(oldestPendingNotificationAgeQuery, "COALESCE"),
		"an empty backlog must scan as 0 rather than NULL")
}
