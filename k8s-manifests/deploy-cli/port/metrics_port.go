package port

import (
	"context"
	"time"

	"deploy-cli/domain"
)

// MetricsCollectorPort defines the interface for metrics collection
type MetricsCollectorPort interface {
	// Basic metrics collection
	CollectMetrics(ctx context.Context, namespace string) (*domain.DeploymentMetrics, error)
	CollectReleaseMetrics(ctx context.Context, releaseName, namespace string) (*domain.ReleaseMetrics, error)
	CollectResourceMetrics(ctx context.Context, resourceType, name, namespace string) (*domain.ResourceMetrics, error)
	
	// Performance metrics
	CollectPerformanceMetrics(ctx context.Context, target string) (*domain.PerformanceInfo, error)
	CollectAvailabilityMetrics(ctx context.Context, target string) (*domain.AvailabilityInfo, error)
	CollectThroughputMetrics(ctx context.Context, target string) (*domain.ThroughputInfo, error)
	
	// Resource utilization
	CollectResourceUsage(ctx context.Context, namespace string) (*domain.ResourceUsage, error)
	CollectNodeMetrics(ctx context.Context, nodeName string) (*domain.NodeMetrics, error)
	CollectPodMetrics(ctx context.Context, podName, namespace string) (*domain.PodMetrics, error)
	
	// Historical metrics
	GetHistoricalMetrics(ctx context.Context, target string, timeRange *domain.TimeRange) ([]domain.MetricData, error)
	GetTrendData(ctx context.Context, metricName, target string, duration time.Duration) ([]domain.DataPoint, error)
	
	// Custom metrics
	RecordCustomMetric(ctx context.Context, metric *domain.CustomMetric) error
	QueryCustomMetrics(ctx context.Context, query *domain.MetricsQuery) (*domain.MetricsResult, error)
	
	// Alerts and thresholds
	SetAlert(ctx context.Context, alert *domain.MetricAlert) error
	GetActiveAlerts(ctx context.Context) ([]*domain.MetricAlert, error)
	
	// Aggregation and reporting
	AggregateMetrics(ctx context.Context, aggregation *domain.MetricsAggregation) (*domain.AggregationResult, error)
	GenerateReport(ctx context.Context, reportConfig *domain.ReportConfig) (*domain.MetricsReport, error)
	
	// Stream metrics for real-time monitoring
	StreamMetrics(ctx context.Context, options *domain.StreamOptions) (<-chan *domain.MetricData, error)
	
	// Health check for metrics collector
	Health(ctx context.Context) error
}