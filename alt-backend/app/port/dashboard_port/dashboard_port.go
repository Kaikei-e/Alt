package dashboard_port

import "context"

// DashboardMetricsPort defines the interface for fetching dashboard metrics.
type DashboardMetricsPort interface {
	GetMetrics(ctx context.Context, metricType string, windowSeconds, limit int64) ([]byte, error)
	GetOverview(ctx context.Context, windowSeconds, limit int64) ([]byte, error)
	GetLogs(ctx context.Context, windowSeconds, limit int64) ([]byte, error)
	GetJobs(ctx context.Context, windowSeconds, limit int64) ([]byte, error)
}
