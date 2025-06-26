package handler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// JobScheduler implementation.
type jobScheduler struct {
	jobs   map[string]*scheduledJob
	logger *slog.Logger
	mutex  sync.RWMutex
}

type scheduledJob struct {
	ctx        context.Context
	lastError  error
	jobFunc    func() error
	ticker     *time.Ticker
	cancel     context.CancelFunc
	lastRun    *time.Time
	nextRun    *time.Time
	name       string
	interval   time.Duration
	errorCount int
	isRunning  bool
}

// NewJobScheduler creates a new job scheduler.
func NewJobScheduler(logger *slog.Logger) JobScheduler {
	return &jobScheduler{
		jobs:   make(map[string]*scheduledJob),
		logger: logger,
	}
}

// Schedule schedules a job to run at the specified interval.
func (s *jobScheduler) Schedule(ctx context.Context, jobName string, intervalStr string, jobFunc func() error) error {
	s.logger.Info("scheduling job", "name", jobName, "interval", intervalStr)

	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		s.logger.Error("failed to parse interval", "error", err, "interval", intervalStr)
		return fmt.Errorf("failed to parse interval %s: %w", intervalStr, err)
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Stop existing job if it exists
	if existingJob, exists := s.jobs[jobName]; exists {
		s.stopJobLocked(existingJob)
	}

	// Create new job
	jobCtx, cancel := context.WithCancel(ctx)
	job := &scheduledJob{
		name:     jobName,
		interval: interval,
		jobFunc:  jobFunc,
		ticker:   time.NewTicker(interval),
		ctx:      jobCtx,
		cancel:   cancel,
	}

	// Calculate next run
	nextRun := time.Now().Add(interval)
	job.nextRun = &nextRun

	s.jobs[jobName] = job

	// Start the job
	go s.runJob(job)

	s.logger.Info("job scheduled successfully", "name", jobName)

	return nil
}

// Stop stops a specific job.
func (s *jobScheduler) Stop(jobName string) error {
	s.logger.Info("stopping job", "name", jobName)

	s.mutex.Lock()
	defer s.mutex.Unlock()

	job, exists := s.jobs[jobName]
	if !exists {
		return fmt.Errorf("job %s not found", jobName)
	}

	s.stopJobLocked(job)
	delete(s.jobs, jobName)

	s.logger.Info("job stopped", "name", jobName)

	return nil
}

// StopAll stops all scheduled jobs.
func (s *jobScheduler) StopAll() error {
	s.logger.Info("stopping all jobs")

	s.mutex.Lock()
	defer s.mutex.Unlock()

	for name, job := range s.jobs {
		s.stopJobLocked(job)
		s.logger.Info("job stopped", "name", name)
	}

	s.jobs = make(map[string]*scheduledJob)
	s.logger.Info("all jobs stopped")

	return nil
}

// GetJobStatus returns the status of a job.
func (s *jobScheduler) GetJobStatus(jobName string) (JobStatus, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	job, exists := s.jobs[jobName]
	if !exists {
		return JobStatus{}, fmt.Errorf("job %s not found", jobName)
	}

	var lastRunStr, nextRunStr *string

	if job.lastRun != nil {
		str := job.lastRun.Format(time.RFC3339)
		lastRunStr = &str
	}

	if job.nextRun != nil {
		str := job.nextRun.Format(time.RFC3339)
		nextRunStr = &str
	}

	return JobStatus{
		Name:       job.name,
		IsRunning:  job.isRunning,
		LastRun:    lastRunStr,
		NextRun:    nextRunStr,
		ErrorCount: job.errorCount,
		LastError:  job.lastError,
	}, nil
}

// stopJobLocked stops a job (must be called with mutex locked).
func (s *jobScheduler) stopJobLocked(job *scheduledJob) {
	if job.ticker != nil {
		job.ticker.Stop()
	}

	if job.cancel != nil {
		job.cancel()
	}

	job.isRunning = false
}

// runJob runs a scheduled job.
func (s *jobScheduler) runJob(job *scheduledJob) {
	s.logger.Info("starting job execution loop", "name", job.name)
	defer s.logger.Info("job execution loop ended", "name", job.name)

	for {
		select {
		case <-job.ctx.Done():
			return
		case <-job.ticker.C:
			s.executeJob(job)
		}
	}
}

// executeJob executes a single job run.
func (s *jobScheduler) executeJob(job *scheduledJob) {
	s.logger.Info("executing job", "name", job.name)

	s.mutex.Lock()
	job.isRunning = true
	s.mutex.Unlock()

	start := time.Now()
	err := job.jobFunc()

	s.mutex.Lock()
	job.isRunning = false
	job.lastRun = &start
	nextRun := time.Now().Add(job.interval)
	job.nextRun = &nextRun

	if err != nil {
		job.errorCount++
		job.lastError = err
		s.logger.Error("job execution failed", "name", job.name, "error", err, "error_count", job.errorCount)
	} else {
		s.logger.Info("job execution completed", "name", job.name, "duration", time.Since(start))
	}
	s.mutex.Unlock()
}
