package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSummarizeJobStatusConstants(t *testing.T) {
	t.Run("should have dead_letter status constant", func(t *testing.T) {
		assert.Equal(t, SummarizeJobStatusDeadLetter, SummarizeJobStatus("dead_letter"))
	})

	t.Run("should have all expected status constants", func(t *testing.T) {
		assert.Equal(t, SummarizeJobStatusPending, SummarizeJobStatus("pending"))
		assert.Equal(t, SummarizeJobStatusRunning, SummarizeJobStatus("running"))
		assert.Equal(t, SummarizeJobStatusCompleted, SummarizeJobStatus("completed"))
		assert.Equal(t, SummarizeJobStatusFailed, SummarizeJobStatus("failed"))
		assert.Equal(t, SummarizeJobStatusDeadLetter, SummarizeJobStatus("dead_letter"))
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

func TestSummarizeOrigin(t *testing.T) {
	t.Run("accepts user and system", func(t *testing.T) {
		origin, err := ParseSummarizeOrigin("user")
		require.NoError(t, err)
		assert.Equal(t, SummarizeOriginUser, origin)
		assert.True(t, origin.IsValid())
		require.NoError(t, origin.Validate())

		origin, err = ParseSummarizeOrigin("system")
		require.NoError(t, err)
		assert.Equal(t, SummarizeOriginSystem, origin)
		assert.True(t, origin.IsValid())
		require.NoError(t, origin.Validate())
	})

	t.Run("rejects empty and unknown values", func(t *testing.T) {
		for _, invalid := range []string{"", " ", "USER", "SYSTEM", "user ", "unknown", "SUMMARIZE_ORIGIN_USER"} {
			origin, err := ParseSummarizeOrigin(invalid)
			require.ErrorIs(t, err, ErrInvalidSummarizeOrigin, "input %q", invalid)
			assert.Empty(t, origin)

			raw := SummarizeOrigin(invalid)
			assert.False(t, raw.IsValid(), "input %q", invalid)
			assert.ErrorIs(t, raw.Validate(), ErrInvalidSummarizeOrigin, "input %q", invalid)
		}
	})

	t.Run("SummarizeJob carries typed Origin", func(t *testing.T) {
		job := &SummarizeJob{Origin: SummarizeOriginUser}
		assert.Equal(t, SummarizeOriginUser, job.Origin)
	})
}
