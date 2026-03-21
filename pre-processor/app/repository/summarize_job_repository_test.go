package repository

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"pre-processor/domain"

	"github.com/stretchr/testify/assert"
)

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

	// Note: The following integration tests should verify the dead_letter transition logic:
	// - When a job fails and retry_count + 1 >= max_retries, status becomes dead_letter
	// - When a job fails and retry_count + 1 < max_retries, status becomes pending (for retry)
	// - GetPendingJobs should not return dead_letter jobs
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

func TestSummarizeJobRepository_CreateJob_Idempotent(t *testing.T) {
	// Note: Integration tests should verify:
	// - First insert for an article_id creates a new job
	// - Second insert for same article_id with existing pending job is skipped (returns "", nil)
	// - Second insert for same article_id with existing running job is skipped (returns "", nil)
	// - Insert succeeds if existing jobs are completed/failed/dead_letter
}
