package knowledge_metrics_gateway

import (
	"context"

	"alt/domain"
	"alt/orchestrator/driver/health_checker"
	"alt/orchestrator/port/knowledge_metrics_port"
)

// ServiceHealthGateway adapts health_checker.Checker to knowledge_metrics_port.CheckServiceHealthPort.
type ServiceHealthGateway struct {
	checker *health_checker.Checker
}

// NewServiceHealthGateway creates a new ServiceHealthGateway.
func NewServiceHealthGateway(checker *health_checker.Checker) knowledge_metrics_port.CheckServiceHealthPort {
	return &ServiceHealthGateway{checker: checker}
}

// CheckHealth calls downstream health checker and maps results to domain types.
func (g *ServiceHealthGateway) CheckHealth(ctx context.Context) ([]domain.ServiceHealthStatus, error) {
	results, err := g.checker.CheckHealth(ctx)
	if err != nil {
		return nil, err
	}

	statuses := make([]domain.ServiceHealthStatus, len(results))
	for i, r := range results {
		var status string
		switch r.Status {
		case health_checker.StatusHealthy:
			status = domain.ServiceHealthy
		case health_checker.StatusUnhealthy:
			status = domain.ServiceUnhealthy
		default:
			status = domain.ServiceUnknown
		}

		statuses[i] = domain.ServiceHealthStatus{
			ServiceName:  r.ServiceName,
			Endpoint:     r.Endpoint,
			CheckedAt:    r.CheckedAt,
			LatencyMs:    r.LatencyMs,
			Status:       status,
			ErrorMessage: r.ErrorMessage,
		}
	}

	return statuses, nil
}
