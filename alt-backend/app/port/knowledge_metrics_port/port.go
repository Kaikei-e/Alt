package knowledge_metrics_port

import (
	"alt/domain"
	"context"
)

// GetSystemMetricsPort reads aggregated system metrics.
type GetSystemMetricsPort interface {
	GetSystemMetrics(ctx context.Context) (*domain.SystemMetrics, error)
}

// CheckServiceHealthPort checks the health of downstream services.
type CheckServiceHealthPort interface {
	CheckHealth(ctx context.Context) ([]domain.ServiceHealthStatus, error)
}
