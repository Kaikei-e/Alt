package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	producerJobID     = "c0ffee00-0000-4000-8000-000000000001"
	producerArticleID = "art-001"
	producerUserID    = "11111111-2222-4333-8444-555555555555"
	producerSummary   = "記事の要約本文"
)

var producerCompletedAt = time.Date(2026, 8, 8, 9, 30, 0, 0, time.UTC)

// newFixedClockJobRepo returns a repository whose completion timestamp is
// fixed, so a test can assert that occurred_at on the outbox row is the job's
// completion time rather than a second, later clock read.
func newFixedClockJobRepo(t *testing.T, at time.Time) (*summarizeJobRepository, pgxmock.PgxPoolIface) {
	t.Helper()
	repo, mock := newMockJobRepo(t)
	repo.clock = func() time.Time { return at }
	return repo, mock
}

// TestCompleteJobWithNotification_OneTransaction is the property the outbox
// pattern exists for: the notification is enqueued if and only if the job
// completion commits. Both sub-tests drive the same call and differ only in
// whether the second statement succeeds — one asserts both statements land
// inside a single Begin/Commit, the other asserts the transaction unwinds so
// neither row survives.
func TestCompleteJobWithNotification_OneTransaction(t *testing.T) {
	t.Run("commits the completion and the outbox row together", func(t *testing.T) {
		repo, mock := newFixedClockJobRepo(t, producerCompletedAt)

		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE summarize_job_queue`).
			WithArgs(producerSummary, producerCompletedAt, "", producerJobID).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`INSERT INTO notification_outbox`).
			WithArgs(
				"summary:"+producerJobID,
				producerUserID,
				"summary_ready",
				[]byte(`{"kind":"summary_ready","url":"/articles/art-001"}`),
				producerCompletedAt,
			).
			WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectCommit()

		err := repo.CompleteJobWithNotification(context.Background(),
			producerJobID, producerSummary, producerUserID, producerArticleID)

		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("rolls the completion back when the outbox insert fails", func(t *testing.T) {
		repo, mock := newFixedClockJobRepo(t, producerCompletedAt)

		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE summarize_job_queue`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))
		mock.ExpectExec(`INSERT INTO notification_outbox`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnError(errors.New("outbox unavailable"))
		mock.ExpectRollback()

		err := repo.CompleteJobWithNotification(context.Background(),
			producerJobID, producerSummary, producerUserID, producerArticleID)

		require.Error(t, err)
		require.NoError(t, mock.ExpectationsWereMet(),
			"the completion UPDATE must be rolled back, not committed, when the outbox insert fails")
	})

	t.Run("rolls back when the job row does not exist", func(t *testing.T) {
		repo, mock := newFixedClockJobRepo(t, producerCompletedAt)

		mock.ExpectBegin()
		mock.ExpectExec(`UPDATE summarize_job_queue`).
			WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
			WillReturnResult(pgxmock.NewResult("UPDATE", 0))
		mock.ExpectRollback()

		err := repo.CompleteJobWithNotification(context.Background(),
			producerJobID, producerSummary, producerUserID, producerArticleID)

		require.Error(t, err)
		require.NoError(t, mock.ExpectationsWereMet(),
			"no outbox row may be enqueued for a job that was never completed")
	})
}

// TestCompleteJobWithNotification_DedupeKeyIsDerived pins the key to the job
// id. A key minted per attempt would make a re-run of the same completion
// look like a second notification downstream.
func TestCompleteJobWithNotification_DedupeKeyIsDerived(t *testing.T) {
	first := summaryReadyDedupeKey(producerJobID)
	second := summaryReadyDedupeKey(producerJobID)

	assert.Equal(t, first, second)
	assert.Equal(t, "summary:"+producerJobID, first)
}

// TestSummaryReadyPayload_CarriesNoContent guards the product rule that a
// notification is a trigger, not a delivery vehicle: only a discriminator and
// a navigate target travel in the payload.
func TestSummaryReadyPayload_CarriesNoContent(t *testing.T) {
	payload, err := summaryReadyPayload(producerArticleID)
	require.NoError(t, err)

	assert.JSONEq(t, `{"kind":"summary_ready","url":"/articles/art-001"}`, string(payload))
	assert.NotContains(t, string(payload), producerSummary)
}

func TestCompleteJobWithNotification_RejectsIncompleteInput(t *testing.T) {
	cases := []struct {
		name      string
		jobID     string
		userID    string
		articleID string
	}{
		{name: "empty job id", jobID: "", userID: producerUserID, articleID: producerArticleID},
		{name: "empty user id", jobID: producerJobID, userID: "", articleID: producerArticleID},
		{name: "empty article id", jobID: producerJobID, userID: producerUserID, articleID: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, mock := newFixedClockJobRepo(t, producerCompletedAt)

			err := repo.CompleteJobWithNotification(context.Background(),
				tc.jobID, producerSummary, tc.userID, tc.articleID)

			require.Error(t, err)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
