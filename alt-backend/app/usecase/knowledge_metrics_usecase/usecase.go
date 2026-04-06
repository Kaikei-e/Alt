package knowledge_metrics_usecase

import (
	"alt/domain"
	"alt/port/knowledge_metrics_port"
	"context"
	"fmt"
)

// Usecase provides system metrics aggregation for the admin dashboard.
type Usecase struct {
	metricsPort knowledge_metrics_port.GetSystemMetricsPort
	healthPort  knowledge_metrics_port.CheckServiceHealthPort
}

// NewUsecase creates a new system metrics usecase.
func NewUsecase(
	metricsPort knowledge_metrics_port.GetSystemMetricsPort,
	healthPort knowledge_metrics_port.CheckServiceHealthPort,
) *Usecase {
	return &Usecase{
		metricsPort: metricsPort,
		healthPort:  healthPort,
	}
}

// GetSystemMetrics returns aggregated system metrics with computed rates.
func (u *Usecase) GetSystemMetrics(ctx context.Context) (*domain.SystemMetrics, error) {
	metrics, err := u.metricsPort.GetSystemMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("get system metrics: %w", err)
	}

	// Compute derived rates
	u.computeRates(metrics)

	// Merge service health (best-effort: if health check fails, return metrics without health)
	health, err := u.healthPort.CheckHealth(ctx)
	if err == nil {
		metrics.ServiceHealth = health
	}

	return metrics, nil
}

// computeRates fills in percentage fields from raw counters.
func (u *Usecase) computeRates(m *domain.SystemMetrics) {
	// Handler degraded rate
	if m.Handler.PagesServed > 0 {
		m.Handler.DegradedRatePct = float64(m.Handler.PagesDegraded) / float64(m.Handler.PagesServed) * 100
	}

	// Tracking rates
	if m.Tracking.ItemsExposed > 0 {
		m.Tracking.OpenRatePct = float64(m.Tracking.ItemsOpened) / float64(m.Tracking.ItemsExposed) * 100
		m.Tracking.DismissRatePct = float64(m.Tracking.ItemsDismissed) / float64(m.Tracking.ItemsExposed) * 100
	}

	// Stream disconnect rate
	if m.Stream.ConnectionsTotal > 0 {
		m.Stream.DisconnectRatePct = float64(m.Stream.DisconnectsTotal) / float64(m.Stream.ConnectionsTotal) * 100
	}

	// Correctness score
	if m.Correctness.RequestsTotal > 0 {
		errorsSum := m.Correctness.EmptyResponses + m.Correctness.MalformedWhy +
			m.Correctness.OrphanItems + m.Correctness.SupersedeMismatch
		m.Correctness.CorrectnessScorePct = 100.0 - (float64(errorsSum)/float64(m.Correctness.RequestsTotal))*100
	} else {
		m.Correctness.CorrectnessScorePct = 100.0
	}

	// Sovereign error rate
	totalSovereign := m.Sovereign.MutationsApplied + m.Sovereign.MutationsErrors
	if totalSovereign > 0 {
		m.Sovereign.ErrorRatePct = float64(m.Sovereign.MutationsErrors) / float64(totalSovereign) * 100
	}
}
