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
		assert.Equal(t, input.GeneratedAt, got.GetGeneratedAt())
		assert.Equal(t, input.WindowStart, got.GetWindowStart())
		assert.Equal(t, input.WindowEnd, got.GetWindowEnd())
		assert.Equal(t, input.IsStale, got.GetIsStale())
		assert.Equal(t, input.StaleAfter, got.GetStaleAfter())
		require.NotNil(t, got.GetLatestRun())
		assert.Equal(t, input.LatestRun.JobID, got.GetLatestRun().GetJobId())
		assert.Equal(t, input.LatestRun.Status, got.GetLatestRun().GetStatus())
		assert.Equal(t, input.LatestRun.KickedAt, got.GetLatestRun().GetKickedAt())
		assert.Equal(t, input.LatestRun.UpdatedAt, got.GetLatestRun().GetUpdatedAt())
		require.Len(t, got.GetDegradationReasons(), 2)
		assert.Equal(t, "few_cards", got.GetDegradationReasons()[0].GetCode())
		assert.Equal(t, "2 cards selected", got.GetDegradationReasons()[0].GetDetail())
		assert.Equal(t, "extractive_genres", got.GetDegradationReasons()[1].GetCode())
		assert.Equal(t, "1 of 2 genres are extractive", got.GetDegradationReasons()[1].GetDetail())
	})

	t.Run("surface status without latest run or degradation reasons", func(t *testing.T) {
		input := &domain.RecapSurfaceStatus{
			GeneratedAt: "2026-09-22T17:05:00Z",
			IsStale:     false,
		}

		got := ToProto(input)
		require.NotNil(t, got)
		assert.Equal(t, input.GeneratedAt, got.GetGeneratedAt())
		assert.False(t, got.GetIsStale())
		assert.Nil(t, got.GetLatestRun())
		assert.Empty(t, got.GetDegradationReasons())
	})
}
