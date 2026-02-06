package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSummarizeJobStatusConstants(t *testing.T) {
	t.Run("should have dead_letter status constant", func(t *testing.T) {
		assert.Equal(t, SummarizeJobStatus("dead_letter"), SummarizeJobStatusDeadLetter)
	})

	t.Run("should have all expected status constants", func(t *testing.T) {
		assert.Equal(t, SummarizeJobStatus("pending"), SummarizeJobStatusPending)
		assert.Equal(t, SummarizeJobStatus("running"), SummarizeJobStatusRunning)
		assert.Equal(t, SummarizeJobStatus("completed"), SummarizeJobStatusCompleted)
		assert.Equal(t, SummarizeJobStatus("failed"), SummarizeJobStatusFailed)
		assert.Equal(t, SummarizeJobStatus("dead_letter"), SummarizeJobStatusDeadLetter)
	})
}

func TestSummarizeJob_IsTerminal(t *testing.T) {
	t.Run("should return true for completed status", func(t *testing.T) {
		job := &SummarizeJob{Status: SummarizeJobStatusCompleted}
		assert.True(t, job.IsTerminal())
	})

	t.Run("should return true for failed status", func(t *testing.T) {
		job := &SummarizeJob{Status: SummarizeJobStatusFailed}
		assert.True(t, job.IsTerminal())
	})

	t.Run("should return true for dead_letter status", func(t *testing.T) {
		job := &SummarizeJob{Status: SummarizeJobStatusDeadLetter}
		assert.True(t, job.IsTerminal())
	})

	t.Run("should return false for pending status", func(t *testing.T) {
		job := &SummarizeJob{Status: SummarizeJobStatusPending}
		assert.False(t, job.IsTerminal())
	})

	t.Run("should return false for running status", func(t *testing.T) {
		job := &SummarizeJob{Status: SummarizeJobStatusRunning}
		assert.False(t, job.IsTerminal())
	})
}

func TestSummarizeJob_CanRetry(t *testing.T) {
	t.Run("should return true when failed and retry_count < max_retries", func(t *testing.T) {
		job := &SummarizeJob{
			Status:     SummarizeJobStatusFailed,
			RetryCount: 1,
			MaxRetries: 3,
		}
		assert.True(t, job.CanRetry())
	})

	t.Run("should return false when retry_count >= max_retries", func(t *testing.T) {
		job := &SummarizeJob{
			Status:     SummarizeJobStatusFailed,
			RetryCount: 3,
			MaxRetries: 3,
		}
		assert.False(t, job.CanRetry())
	})

	t.Run("should return false when status is not failed", func(t *testing.T) {
		job := &SummarizeJob{
			Status:     SummarizeJobStatusPending,
			RetryCount: 0,
			MaxRetries: 3,
		}
		assert.False(t, job.CanRetry())
	})

	t.Run("should return false for dead_letter status", func(t *testing.T) {
		job := &SummarizeJob{
			Status:     SummarizeJobStatusDeadLetter,
			RetryCount: 3,
			MaxRetries: 3,
		}
		assert.False(t, job.CanRetry())
	})
}
