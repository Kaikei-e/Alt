package knowledge_metrics_usecase

import (
	"alt/domain"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mock ports ---

type mockGetSystemMetricsPort struct {
	result *domain.SystemMetrics
	err    error
}

func (m *mockGetSystemMetricsPort) GetSystemMetrics(_ context.Context) (*domain.SystemMetrics, error) {
	return m.result, m.err
}

type mockCheckServiceHealthPort struct {
	result []domain.ServiceHealthStatus
	err    error
}

func (m *mockCheckServiceHealthPort) CheckHealth(_ context.Context) ([]domain.ServiceHealthStatus, error) {
	return m.result, m.err
}

// --- tests ---

func TestGetSystemMetrics(t *testing.T) {
	t.Run("computes rates correctly from raw counters", func(t *testing.T) {
		metricsPort := &mockGetSystemMetricsPort{
			result: &domain.SystemMetrics{
				Handler: domain.HandlerMetrics{
					PagesServed:   1000,
					PagesDegraded: 50,
				},
				Tracking: domain.TrackingMetrics{
					ItemsExposed:   500,
					ItemsOpened:    100,
					ItemsDismissed: 25,
				},
				Stream: domain.StreamMetrics{
					ConnectionsTotal: 200,
					DisconnectsTotal: 10,
					ReconnectsTotal:  8,
					DeliveriesTotal:  5000,
				},
				Correctness: domain.CorrectnessMetrics{
					EmptyResponses:    5,
					MalformedWhy:      2,
					OrphanItems:       1,
					SupersedeMismatch: 0,
					RequestsTotal:     1000,
				},
				Sovereign: domain.SovereignMetrics{
					MutationsApplied: 300,
					MutationsErrors:  3,
				},
			},
		}
		healthPort := &mockCheckServiceHealthPort{
			result: []domain.ServiceHealthStatus{
				{ServiceName: "sovereign", Status: domain.ServiceHealthy, LatencyMs: 5, CheckedAt: time.Now()},
				{ServiceName: "meilisearch", Status: domain.ServiceUnhealthy, LatencyMs: 0, CheckedAt: time.Now(), ErrorMessage: "timeout"},
			},
		}

		uc := NewUsecase(metricsPort, healthPort)
		result, err := uc.GetSystemMetrics(context.Background())
		require.NoError(t, err)
		require.NotNil(t, result)

		// Handler degraded rate: 50/1000 * 100 = 5.0%
		assert.InDelta(t, 5.0, result.Handler.DegradedRatePct, 0.01)

		// Tracking open rate: 100/500 * 100 = 20.0%
		assert.InDelta(t, 20.0, result.Tracking.OpenRatePct, 0.01)
		// Tracking dismiss rate: 25/500 * 100 = 5.0%
		assert.InDelta(t, 5.0, result.Tracking.DismissRatePct, 0.01)

		// Stream disconnect rate: 10/200 * 100 = 5.0%
		assert.InDelta(t, 5.0, result.Stream.DisconnectRatePct, 0.01)

		// Correctness score: 100 - ((5+2+1+0) / 1000 * 100) = 99.2%
		assert.InDelta(t, 99.2, result.Correctness.CorrectnessScorePct, 0.01)

		// Sovereign error rate: 3/303 * 100 ≈ 0.99%
		assert.InDelta(t, 0.99, result.Sovereign.ErrorRatePct, 0.01)

		// Service health merged
		assert.Len(t, result.ServiceHealth, 2)
		assert.Equal(t, domain.ServiceHealthy, result.ServiceHealth[0].Status)
		assert.Equal(t, domain.ServiceUnhealthy, result.ServiceHealth[1].Status)
	})

	t.Run("handles zero denominators gracefully", func(t *testing.T) {
		metricsPort := &mockGetSystemMetricsPort{
			result: &domain.SystemMetrics{
				Handler:     domain.HandlerMetrics{PagesServed: 0, PagesDegraded: 0},
				Tracking:    domain.TrackingMetrics{ItemsExposed: 0},
				Stream:      domain.StreamMetrics{ConnectionsTotal: 0},
				Correctness: domain.CorrectnessMetrics{RequestsTotal: 0},
				Sovereign:   domain.SovereignMetrics{MutationsApplied: 0, MutationsErrors: 0},
			},
		}
		healthPort := &mockCheckServiceHealthPort{result: nil}

		uc := NewUsecase(metricsPort, healthPort)
		result, err := uc.GetSystemMetrics(context.Background())
		require.NoError(t, err)

		assert.Equal(t, 0.0, result.Handler.DegradedRatePct)
		assert.Equal(t, 0.0, result.Tracking.OpenRatePct)
		assert.Equal(t, 0.0, result.Stream.DisconnectRatePct)
		assert.Equal(t, 100.0, result.Correctness.CorrectnessScorePct)
		assert.Equal(t, 0.0, result.Sovereign.ErrorRatePct)
	})

	t.Run("returns metrics even when health check fails", func(t *testing.T) {
		metricsPort := &mockGetSystemMetricsPort{
			result: &domain.SystemMetrics{
				Projector: domain.ProjectorMetrics{EventsProcessed: 42},
			},
		}
		healthPort := &mockCheckServiceHealthPort{err: assert.AnError}

		uc := NewUsecase(metricsPort, healthPort)
		result, err := uc.GetSystemMetrics(context.Background())
		require.NoError(t, err)
		assert.Equal(t, int64(42), result.Projector.EventsProcessed)
		assert.Empty(t, result.ServiceHealth)
	})

	t.Run("returns error when metrics port fails", func(t *testing.T) {
		metricsPort := &mockGetSystemMetricsPort{err: assert.AnError}
		healthPort := &mockCheckServiceHealthPort{}

		uc := NewUsecase(metricsPort, healthPort)
		result, err := uc.GetSystemMetrics(context.Background())
		require.Error(t, err)
		assert.Nil(t, result)
	})
}
