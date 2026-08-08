package repository

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"pre-processor/domain"

	"github.com/pashagolub/pgxmock/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMockJobRepo returns a summarizeJobRepository backed by a pgxmock pool
// instead of a real *pgxpool.Pool, so tests below can assert on the actual
// SQL text and argument bindings the repository sends — not just the
// nil-db/empty-articleID early returns, which never reach the query at all.
func newMockJobRepo(t *testing.T) (*summarizeJobRepository, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(mock.Close)
	return &summarizeJobRepository{db: mock, logger: testSummarizeJobLogger()}, mock
}

func testSummarizeJobLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Only errors in tests
	}))
}

func TestSummarizeJobRepository_InterfaceCompliance(t *testing.T) {
	t.Run("should implement SummarizeJobRepository interface", func(t *testing.T) {
		repo := NewSummarizeJobRepository(nil, testSummarizeJobLogger())

		assert.NotNil(t, repo)
	})
}

func TestSummarizeJobRepository_CreateJob(t *testing.T) {
	t.Run("should handle nil database gracefully", func(t *testing.T) {
		repo := NewSummarizeJobRepository(nil, testSummarizeJobLogger())

		jobID, err := repo.CreateJob(context.Background(), "test-article-id")

		assert.Error(t, err)
		assert.Empty(t, jobID)
	})

	t.Run("should reject empty article ID", func(t *testing.T) {
		repo := NewSummarizeJobRepository(nil, testSummarizeJobLogger())

		jobID, err := repo.CreateJob(context.Background(), "")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "article ID cannot be empty")
		assert.Empty(t, jobID)
	})
}

func TestSummarizeJobRepository_HasRecentSuccessfulJob(t *testing.T) {
	t.Run("should handle nil database gracefully", func(t *testing.T) {
		repo := NewSummarizeJobRepository(nil, testSummarizeJobLogger())

		exists, err := repo.HasRecentSuccessfulJob(context.Background(), "test-article-id", time.Now().Add(-time.Hour))

		assert.Error(t, err)
		assert.False(t, exists)
	})

	t.Run("should reject empty article ID", func(t *testing.T) {
		repo := NewSummarizeJobRepository(nil, testSummarizeJobLogger())

		exists, err := repo.HasRecentSuccessfulJob(context.Background(), "", time.Now().Add(-time.Hour))

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "article ID cannot be empty")
		assert.False(t, exists)
	})
}

func TestSummarizeJobRepository_HasInFlightJob(t *testing.T) {
	t.Run("should handle nil database gracefully", func(t *testing.T) {
		repo := NewSummarizeJobRepository(nil, testSummarizeJobLogger())

		exists, err := repo.HasInFlightJob(context.Background(), "test-article-id", time.Now().Add(-10*time.Minute))

		assert.Error(t, err)
		assert.False(t, exists)
	})

	t.Run("should reject empty article ID", func(t *testing.T) {
		repo := NewSummarizeJobRepository(nil, testSummarizeJobLogger())

		exists, err := repo.HasInFlightJob(context.Background(), "", time.Now().Add(-10*time.Minute))

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "article ID cannot be empty")
		assert.False(t, exists)
	})

	// Note: Integration tests should verify:
	// - Pending jobs within the window are reported as in-flight
	// - Running jobs within the window are reported as in-flight
	// - Jobs whose updated_at is older than the window are excluded (stale)
	// - Jobs for other article_ids are excluded
	// - Completed / failed / dead_letter jobs are excluded
}

func TestSummarizeJobRepository_HasDeadLetterJob(t *testing.T) {
	t.Run("should handle nil database gracefully", func(t *testing.T) {
		repo := NewSummarizeJobRepository(nil, testSummarizeJobLogger())

		exists, err := repo.HasDeadLetterJob(context.Background(), "test-article-id")

		assert.Error(t, err)
		assert.False(t, exists)
	})

	t.Run("should reject empty article ID", func(t *testing.T) {
		repo := NewSummarizeJobRepository(nil, testSummarizeJobLogger())

		exists, err := repo.HasDeadLetterJob(context.Background(), "")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "article ID cannot be empty")
		assert.False(t, exists)
	})

	// hasDeadLetterJobQueryPattern pins the query shape that makes
	// HasDeadLetterJob look at the article's MOST RECENT job row rather than
	// "any row ever" (see the method's doc comment): a stale dead_letter row
	// from an earlier attempt must not out-vote a later row that shows the
	// article moved on (e.g. after InvalidateCompletedJobSummary). Losing the
	// ORDER BY ... LIMIT 1 shape silently reintroduces exactly that bug.
	const hasDeadLetterJobQueryPattern = `SELECT status FROM summarize_job_queue WHERE article_id = \$1 ORDER BY created_at DESC, id DESC LIMIT 1`

	t.Run("reports true when the most recent job row is dead_letter", func(t *testing.T) {
		repo, mock := newMockJobRepo(t)

		mock.ExpectQuery(hasDeadLetterJobQueryPattern).
			WithArgs("article-dead").
			WillReturnRows(pgxmock.NewRows([]string{"status"}).AddRow(string(domain.SummarizeJobStatusDeadLetter)))

		exists, err := repo.HasDeadLetterJob(context.Background(), "article-dead")

		require.NoError(t, err)
		assert.True(t, exists)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("reports false when the most recent job row is a later, non-dead_letter status", func(t *testing.T) {
		repo, mock := newMockJobRepo(t)

		// Simulates a stale dead_letter row existing in history while the
		// LATEST row (what the query returns) is 'completed' — e.g. quality
		// check invalidated its summary via InvalidateCompletedJobSummary.
		// The guard must not block re-enqueue in this case.
		mock.ExpectQuery(hasDeadLetterJobQueryPattern).
			WithArgs("article-recovered").
			WillReturnRows(pgxmock.NewRows([]string{"status"}).AddRow(string(domain.SummarizeJobStatusCompleted)))

		exists, err := repo.HasDeadLetterJob(context.Background(), "article-recovered")

		require.NoError(t, err)
		assert.False(t, exists)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("reports false without error when the article has no job rows", func(t *testing.T) {
		repo, mock := newMockJobRepo(t)

		mock.ExpectQuery(hasDeadLetterJobQueryPattern).
			WithArgs("article-unseen").
			WillReturnRows(pgxmock.NewRows([]string{"status"}))

		exists, err := repo.HasDeadLetterJob(context.Background(), "article-unseen")

		require.NoError(t, err)
		assert.False(t, exists)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestSummarizeJobRepository_HasRecentFailedJob(t *testing.T) {
	t.Run("should handle nil database gracefully", func(t *testing.T) {
		repo := NewSummarizeJobRepository(nil, testSummarizeJobLogger())

		exists, err := repo.HasRecentFailedJob(context.Background(), "test-article-id", time.Now().Add(-time.Hour))

		assert.Error(t, err)
		assert.False(t, exists)
	})

	t.Run("should reject empty article ID", func(t *testing.T) {
		repo := NewSummarizeJobRepository(nil, testSummarizeJobLogger())

		exists, err := repo.HasRecentFailedJob(context.Background(), "", time.Now().Add(-time.Hour))

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "article ID cannot be empty")
		assert.False(t, exists)
	})

	// hasRecentFailedJobQueryPattern pins that the predicate targets the
	// 'failed' status specifically. HasDeadLetterJob and HasRecentFailedJob
	// must classify jobs into disjoint buckets — dead_letter permanent,
	// failed time-bounded — so this predicate silently matching the wrong
	// status would either let a permanently-blocked article slip past (if
	// it matched dead_letter too) or block an article that never failed.
	const hasRecentFailedJobQueryPattern = `status = 'failed'\s+AND completed_at >= \$2`

	t.Run("reports true when a failed job within the window exists", func(t *testing.T) {
		repo, mock := newMockJobRepo(t)
		since := time.Now().Add(-time.Hour)

		mock.ExpectQuery(hasRecentFailedJobQueryPattern).
			WithArgs("article-failed", since).
			WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))

		exists, err := repo.HasRecentFailedJob(context.Background(), "article-failed", since)

		require.NoError(t, err)
		assert.True(t, exists)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("reports false when no failed job is within the window", func(t *testing.T) {
		repo, mock := newMockJobRepo(t)
		since := time.Now().Add(-time.Hour)

		mock.ExpectQuery(hasRecentFailedJobQueryPattern).
			WithArgs("article-clean", since).
			WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))

		exists, err := repo.HasRecentFailedJob(context.Background(), "article-clean", since)

		require.NoError(t, err)
		assert.False(t, exists)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestSummarizeJobRepository_GetJob(t *testing.T) {
	t.Run("should handle nil database gracefully", func(t *testing.T) {
		repo := NewSummarizeJobRepository(nil, testSummarizeJobLogger())

		job, err := repo.GetJob(context.Background(), "test-job-id")

		assert.Error(t, err)
		assert.Nil(t, job)
	})

	t.Run("should reject empty job ID", func(t *testing.T) {
		repo := NewSummarizeJobRepository(nil, testSummarizeJobLogger())

		job, err := repo.GetJob(context.Background(), "")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "job ID cannot be empty")
		assert.Nil(t, job)
	})
}

func TestSummarizeJobRepository_UpdateJobStatus(t *testing.T) {
	t.Run("should handle nil database gracefully", func(t *testing.T) {
		repo := NewSummarizeJobRepository(nil, testSummarizeJobLogger())

		err := repo.UpdateJobStatus(context.Background(), "test-job-id", domain.SummarizeJobStatusRunning, "", "")

		assert.Error(t, err)
	})

	t.Run("should reject empty job ID", func(t *testing.T) {
		repo := NewSummarizeJobRepository(nil, testSummarizeJobLogger())

		err := repo.UpdateJobStatus(context.Background(), "", domain.SummarizeJobStatusRunning, "", "")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "job ID cannot be empty")
	})

	t.Run("should handle dead_letter status with error message on nil database", func(t *testing.T) {
		repo := NewSummarizeJobRepository(nil, testSummarizeJobLogger())

		err := repo.UpdateJobStatus(context.Background(), "test-job-id", domain.SummarizeJobStatusDeadLetter, "", "max retries exceeded")

		assert.Error(t, err)
	})

	// Retry exhaustion (status=Failed argument) must land on the 'failed'
	// terminal status, NOT 'dead_letter': dead_letter is reserved for the
	// explicit domain.ErrContentNotProcessable classification (the
	// SummarizeJobStatusDeadLetter case below), because it covers any error
	// including transient infrastructure failures — see HasRecentFailedJob's
	// bounded cooldown vs. HasDeadLetterJob's permanent block.
	t.Run("writes 'failed' (not dead_letter) when retries are exhausted for a generic error", func(t *testing.T) {
		repo, mock := newMockJobRepo(t)

		mock.ExpectExec(`UPDATE summarize_job_queue\s+SET\s+status = CASE\s+WHEN retry_count \+ 1 >= max_retries THEN 'failed'\s+ELSE 'pending'\s+END`).
			WithArgs("boom", pgxmock.AnyArg(), "job-exhausted").
			WillReturnResult(pgxmock.NewResult("UPDATE", 1))

		err := repo.UpdateJobStatus(context.Background(), "job-exhausted", domain.SummarizeJobStatusFailed, "", "boom")

		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	// Note: The following integration tests should verify the dead_letter transition logic:
	// - domain.ErrContentNotProcessable moves a job straight to dead_letter, bypassing retry
	// - GetPendingJobs should not return dead_letter or failed jobs
}

func TestSummarizeJobRepository_GetPendingJobs(t *testing.T) {
	t.Run("should handle nil database gracefully", func(t *testing.T) {
		repo := NewSummarizeJobRepository(nil, testSummarizeJobLogger())

		jobs, err := repo.GetPendingJobs(context.Background(), 10)

		assert.Error(t, err)
		assert.Nil(t, jobs)
	})

	t.Run("should reject non-positive limit", func(t *testing.T) {
		repo := NewSummarizeJobRepository(nil, testSummarizeJobLogger())

		jobs, err := repo.GetPendingJobs(context.Background(), 0)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "limit must be positive")
		assert.Nil(t, jobs)
	})

	t.Run("should query pending jobs oldest first", func(t *testing.T) {
		assert.True(t, strings.Contains(getPendingJobsQuery, "ORDER BY created_at ASC"),
			"pending jobs should be dequeued oldest-first to avoid backlog starvation")
	})
}

func TestSummarizeJobRepository_DequeueJobs(t *testing.T) {
	t.Run("should handle nil database gracefully", func(t *testing.T) {
		repo := NewSummarizeJobRepository(nil, testSummarizeJobLogger())

		jobs, err := repo.DequeueJobs(context.Background(), 10)

		assert.Error(t, err)
		assert.Nil(t, jobs)
	})

	t.Run("should reject non-positive limit", func(t *testing.T) {
		repo := NewSummarizeJobRepository(nil, testSummarizeJobLogger())

		jobs, err := repo.DequeueJobs(context.Background(), 0)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "limit must be positive")
		assert.Nil(t, jobs)
	})

	t.Run("should reject negative limit", func(t *testing.T) {
		repo := NewSummarizeJobRepository(nil, testSummarizeJobLogger())

		jobs, err := repo.DequeueJobs(context.Background(), -1)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "limit must be positive")
		assert.Nil(t, jobs)
	})
}

func TestSummarizeJobRepository_RecoverStuckJobs(t *testing.T) {
	t.Run("should handle nil database gracefully", func(t *testing.T) {
		repo := NewSummarizeJobRepository(nil, testSummarizeJobLogger())

		recovered, err := repo.RecoverStuckJobs(context.Background())

		assert.Error(t, err)
		assert.Equal(t, int64(0), recovered)
	})

	// Note: Integration tests should verify:
	// - Jobs stuck in 'running' for >10 minutes are reset to 'pending'
	// - Jobs running for <10 minutes are not affected
	// - started_at is set to NULL on recovery
	// - Returns count of recovered jobs
}

func TestSummarizeJobRepository_InvalidateCompletedJobSummary(t *testing.T) {
	t.Run("should reject empty article ID", func(t *testing.T) {
		repo := NewSummarizeJobRepository(nil, testSummarizeJobLogger())

		err := repo.InvalidateCompletedJobSummary(context.Background(), "")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "article ID cannot be empty")
	})

	t.Run("should handle nil database gracefully", func(t *testing.T) {
		repo := NewSummarizeJobRepository(nil, testSummarizeJobLogger())

		err := repo.InvalidateCompletedJobSummary(context.Background(), "test-article-id")

		assert.Error(t, err)
	})

	// Note: Integration tests should verify:
	// - Completed jobs with non-empty summary get summary set to NULL
	// - Pending/running jobs are NOT affected
	// - Multiple completed jobs for same article all get summary NULLed
	// - No-op when article has no completed jobs (idempotent)
	// - After invalidation, HasRecentSuccessfulJob returns false
}

func TestSummarizeJobRepository_CreateJob_Idempotent(t *testing.T) {
	// Note: Integration tests should verify:
	// - First insert for an article_id creates a new job
	// - Second insert for same article_id with existing pending job is skipped (returns "", nil)
	// - Second insert for same article_id with existing running job is skipped (returns "", nil)
	// - Insert succeeds if existing jobs are completed/failed/dead_letter
}
