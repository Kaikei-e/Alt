package surfacestatus

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/domain"
)

func TestToProto(t *testing.T) {
	t.Run("nil input returns nil", func(t *testing.T) {
		assert.Nil(t, ToProto(nil))
	})

	t.Run("populated surface status with latest run and degradation reasons", func(t *testing.T) {
		input := &domain.RecapSurfaceStatus{
			GeneratedAt: "2026-09-22T17:05:00Z",
			WindowStart: "2026-09-19T17:00:00Z",
			WindowEnd:   "2026-09-22T17:00:00Z",
			IsStale:     true,
			StaleAfter:  "2026-09-23T23:05:00Z",
			LatestRun: &domain.RecapSurfaceRun{
				JobID:     "job-123",
				Status:    "completed",
				KickedAt:  "2026-09-22T17:00:00Z",
				UpdatedAt: "2026-09-22T17:05:00Z",
			},
			DegradationReasons: []domain.RecapDegradationReason{
				{Code: "few_cards", Detail: "2 cards selected"},
				{Code: "extractive_genres", Detail: "1 of 2 genres are extractive"},
			},
		}

		got := ToProto(input)
		require.NotNil(t, got)
		assert.Equal(t, input.GeneratedAt, got.GeneratedAt)
		assert.Equal(t, input.WindowStart, got.WindowStart)
		assert.Equal(t, input.WindowEnd, got.WindowEnd)
		assert.Equal(t, input.IsStale, got.IsStale)
		assert.Equal(t, input.StaleAfter, got.StaleAfter)
		require.NotNil(t, got.LatestRun)
		assert.Equal(t, input.LatestRun.JobID, got.LatestRun.JobId)
		assert.Equal(t, input.LatestRun.Status, got.LatestRun.Status)
		assert.Equal(t, input.LatestRun.KickedAt, got.LatestRun.KickedAt)
		assert.Equal(t, input.LatestRun.UpdatedAt, got.LatestRun.UpdatedAt)
		require.Len(t, got.DegradationReasons, 2)
		assert.Equal(t, "few_cards", got.DegradationReasons[0].Code)
		assert.Equal(t, "2 cards selected", got.DegradationReasons[0].Detail)
		assert.Equal(t, "extractive_genres", got.DegradationReasons[1].Code)
		assert.Equal(t, "1 of 2 genres are extractive", got.DegradationReasons[1].Detail)
	})

	t.Run("surface status without latest run or degradation reasons", func(t *testing.T) {
		input := &domain.RecapSurfaceStatus{
			GeneratedAt: "2026-09-22T17:05:00Z",
			IsStale:     false,
		}

		got := ToProto(input)
		require.NotNil(t, got)
		assert.Equal(t, input.GeneratedAt, got.GeneratedAt)
		assert.False(t, got.IsStale)
		assert.Nil(t, got.LatestRun)
		assert.Empty(t, got.DegradationReasons)
	})
}
