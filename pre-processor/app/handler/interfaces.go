package handler

import (
	"context"
)

//go:generate mockgen -source=interfaces.go -destination=../test/mocks/handler_mocks.go -package=mocks

// JobHandler handles background job orchestration.
type JobHandler interface {
	StartFeedProcessingJob(ctx context.Context) error
	StartSummarizationJob(ctx context.Context) error
	StartQualityCheckJob(ctx context.Context) error
	Stop() error
}

// HealthHandler handles health check endpoints.
type HealthHandler interface {
	CheckHealth(ctx context.Context) error
	CheckDependencies(ctx context.Context) error
}

// JobScheduler handles job scheduling and coordination.
type JobScheduler interface {
	Schedule(ctx context.Context, jobName string, interval string, jobFunc func() error) error
	Stop(jobName string) error
	StopAll() error
	GetJobStatus(jobName string) (JobStatus, error)
}

// JobStatus represents the status of a scheduled job.
type JobStatus struct {
	LastError  error
	LastRun    *string
	NextRun    *string
	Name       string
	ErrorCount int
	IsRunning  bool
}
