package knowledge_slo_usecase

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mock ports ---

type mockGetProjectionLagPort struct {
	lag time.Duration
	err error
}

func (m *mockGetProjectionLagPort) GetProjectionLag(_ context.Context) (time.Duration, error) {
	return m.lag, m.err
}

// --- tests ---

func TestGetSLOStatus(t *testing.T) {
	logger.InitLogger()

	t.Run("returns SLO status with freshness from port", func(t *testing.T) {
		lagPort := &mockGetProjectionLagPort{lag: 30 * time.Second}
		uc := NewUsecase(lagPort)

		status, err := uc.GetSLOStatus(context.Background())
		require.NoError(t, err)
		require.NotNil(t, status)

		// Verify structure
		assert.NotEmpty(t, status.OverallHealth)
		assert.NotZero(t, status.ErrorBudgetWindowDays)
		assert.False(t, status.ComputedAt.IsZero())

		// Verify freshness SLI is present and uses the port value
		var freshnessSLI *domain.SLIResult
		for i := range status.SLIs {
			if status.SLIs[i].Name == domain.SLIFreshness {
				freshnessSLI = &status.SLIs[i]
				break
			}
		}
		require.NotNil(t, freshnessSLI, "freshness SLI should be present")
		assert.Equal(t, float64(30), freshnessSLI.CurrentValue) // 30 seconds
		assert.Equal(t, "seconds", freshnessSLI.Unit)
		assert.Equal(t, domain.SLIStatusMeeting, freshnessSLI.Status) // 30s < 300s target
	})

	t.Run("freshness SLI shows burning when lag exceeds target", func(t *testing.T) {
		lagPort := &mockGetProjectionLagPort{lag: 6 * time.Minute}
		uc := NewUsecase(lagPort)

		status, err := uc.GetSLOStatus(context.Background())
		require.NoError(t, err)

		var freshnessSLI *domain.SLIResult
		for i := range status.SLIs {
			if status.SLIs[i].Name == domain.SLIFreshness {
				freshnessSLI = &status.SLIs[i]
				break
			}
		}
		require.NotNil(t, freshnessSLI)
		assert.Equal(t, domain.SLIStatusBurning, freshnessSLI.Status)
	})

	t.Run("returns degraded status on lag port error", func(t *testing.T) {
		lagPort := &mockGetProjectionLagPort{err: assert.AnError}
		uc := NewUsecase(lagPort)

		status, err := uc.GetSLOStatus(context.Background())
		require.NoError(t, err)
		require.NotNil(t, status)

		// Should still return a status, but with unknown freshness
		var freshnessSLI *domain.SLIResult
		for i := range status.SLIs {
			if status.SLIs[i].Name == domain.SLIFreshness {
				freshnessSLI = &status.SLIs[i]
				break
			}
		}
		require.NotNil(t, freshnessSLI)
		assert.Equal(t, domain.SLIStatusBreached, freshnessSLI.Status)
	})

	t.Run("includes placeholder SLIs for unimplemented metrics", func(t *testing.T) {
		lagPort := &mockGetProjectionLagPort{lag: 10 * time.Second}
		uc := NewUsecase(lagPort)

		status, err := uc.GetSLOStatus(context.Background())
		require.NoError(t, err)

		sliNames := make(map[string]bool)
		for _, sli := range status.SLIs {
			sliNames[sli.Name] = true
		}
		assert.True(t, sliNames[domain.SLIAvailability], "availability SLI should be present")
		assert.True(t, sliNames[domain.SLIFreshness], "freshness SLI should be present")
		assert.True(t, sliNames[domain.SLIActionDurability], "action_durability SLI should be present")
		assert.True(t, sliNames[domain.SLIStreamContinuity], "stream_continuity SLI should be present")
		assert.True(t, sliNames[domain.SLICorrectnessProxy], "correctness_proxy SLI should be present")
	})
}
