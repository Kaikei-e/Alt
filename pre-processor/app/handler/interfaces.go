package handler

import (
	"context"
)

//go:generate mockgen -source=interfaces.go -destination=../test/mocks/handler_mocks.go -package=mocks

// JobHandler handles background job orchestration.
type JobHandler interface {
	StartSummarizationJob(ctx context.Context) error
	StartQualityCheckJob(ctx context.Context) error
	StartArticleSyncJob(ctx context.Context) error
	StartSummarizeQueueWorker(ctx context.Context) error
	Stop() error
}

// HealthHandler handles health check endpoints.
type HealthHandler interface {
	CheckHealth(ctx context.Context) error
	CheckDependencies(ctx context.Context) error
	GetHealthMetrics(ctx context.Context) (map[string]interface{}, error)
	GetExtendedHealthMetrics(ctx context.Context) (map[string]interface{}, error)
	CheckSLACompliance(ctx context.Context) (map[string]interface{}, error)
	GetHealthAlerts(ctx context.Context) ([]map[string]interface{}, error)
}
