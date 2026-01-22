package repository

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"pre-processor/models"

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

		err := repo.UpdateJobStatus(context.Background(), "test-job-id", models.SummarizeJobStatusRunning, "", "")

		assert.Error(t, err)
	})

	t.Run("should reject empty job ID", func(t *testing.T) {
		repo := NewSummarizeJobRepository(nil, testSummarizeJobLogger())

		err := repo.UpdateJobStatus(context.Background(), "", models.SummarizeJobStatusRunning, "", "")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "job ID cannot be empty")
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
}
