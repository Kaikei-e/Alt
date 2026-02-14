package dashboard_usecase

import (
	"context"

	"alt/port/dashboard_port"
)

// DashboardMetricsUsecase provides dashboard metrics operations.
type DashboardMetricsUsecase struct {
	port dashboard_port.DashboardMetricsPort
}

// NewDashboardMetricsUsecase creates a new DashboardMetricsUsecase.
func NewDashboardMetricsUsecase(port dashboard_port.DashboardMetricsPort) *DashboardMetricsUsecase {
	return &DashboardMetricsUsecase{port: port}
}

// GetMetrics fetches system metrics.
func (u *DashboardMetricsUsecase) GetMetrics(ctx context.Context, metricType string, windowSeconds, limit int64) ([]byte, error) {
	return u.port.GetMetrics(ctx, metricType, windowSeconds, limit)
}

// GetOverview fetches recent activity overview.
func (u *DashboardMetricsUsecase) GetOverview(ctx context.Context, windowSeconds, limit int64) ([]byte, error) {
	return u.port.GetOverview(ctx, windowSeconds, limit)
}

// GetLogs fetches error logs.
func (u *DashboardMetricsUsecase) GetLogs(ctx context.Context, windowSeconds, limit int64) ([]byte, error) {
	return u.port.GetLogs(ctx, windowSeconds, limit)
}

// GetJobs fetches admin jobs.
func (u *DashboardMetricsUsecase) GetJobs(ctx context.Context, windowSeconds, limit int64) ([]byte, error) {
	return u.port.GetJobs(ctx, windowSeconds, limit)
}
