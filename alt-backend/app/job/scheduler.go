package job

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Job defines a periodic background job.
type Job struct {
	Name     string
	Interval time.Duration
	Timeout  time.Duration
	Fn       func(ctx context.Context) error
}

// JobScheduler manages periodic background jobs with context-aware shutdown.
type JobScheduler struct {
	jobs []Job
	wg   sync.WaitGroup
}

// NewJobScheduler creates a new scheduler.
func NewJobScheduler() *JobScheduler {
	return &JobScheduler{}
}

// Add registers a job to be run when Start is called.
func (s *JobScheduler) Add(j Job) {
	s.jobs = append(s.jobs, j)
}

// Start launches all registered jobs as goroutines. Each job runs immediately
// on start, then repeats at its configured interval. All jobs stop when ctx
// is cancelled.
func (s *JobScheduler) Start(ctx context.Context) {
	for _, j := range s.jobs {
		s.wg.Add(1)
		go s.runJob(ctx, j)
	}
}

func (s *JobScheduler) runJob(ctx context.Context, j Job) {
	defer s.wg.Done()

	// Run immediately on start
	s.executeJob(ctx, j)

	ticker := time.NewTicker(j.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.InfoContext(ctx, "job stopping", "job", j.Name)
			return
		case <-ticker.C:
			s.executeJob(ctx, j)
		}
	}
}

func (s *JobScheduler) executeJob(ctx context.Context, j Job) {
	// Check if parent context is already cancelled
	if ctx.Err() != nil {
		return
	}

	jobCtx, cancel := context.WithTimeout(ctx, j.Timeout)
	defer cancel()

	if err := j.Fn(jobCtx); err != nil {
		slog.ErrorContext(ctx, "job failed", "job", j.Name, "error", err)
	}
}

// Shutdown blocks until all running jobs complete.
func (s *JobScheduler) Shutdown() {
	s.wg.Wait()
}
