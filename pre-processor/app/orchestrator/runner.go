package orchestrator

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// JobConfig configures a job runner.
type JobConfig struct {
	Name            string
	Interval        time.Duration
	InitialBackoff  time.Duration
	MaxBackoff      time.Duration
	BackoffOnErrors []error // Errors that trigger backoff instead of logging
	RunImmediately  bool    // Run once immediately before starting the ticker
}

// JobRunner manages the lifecycle of a single background job.
type JobRunner struct {
	config JobConfig
	fn     func(ctx context.Context) error
	logger *slog.Logger
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewJobRunner creates a new job runner.
func NewJobRunner(config JobConfig, fn func(ctx context.Context) error, logger *slog.Logger) *JobRunner {
	return &JobRunner{
		config: config,
		fn:     fn,
		logger: logger,
	}
}

// Start starts the job runner in a goroutine.
func (r *JobRunner) Start(ctx context.Context) {
	jobCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.run(jobCtx)
	}()
}

// Stop stops the job runner and waits for it to finish.
func (r *JobRunner) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()
}

// run is the main loop of the job runner.
func (r *JobRunner) run(ctx context.Context) {
	defer func() {
		if rec := recover(); rec != nil {
			r.logger.ErrorContext(ctx, "panic in job runner", "job", r.config.Name, "panic", rec)
		}
	}()

	// Run immediately if configured
	if r.config.RunImmediately {
		if err := r.fn(ctx); err != nil {
			r.logger.ErrorContext(ctx, "initial job run failed", "job", r.config.Name, "error", err)
		}
	}

	ticker := time.NewTicker(r.config.Interval)
	defer ticker.Stop()

	backoff := time.Duration(0)

	for {
		select {
		case <-ctx.Done():
			r.logger.InfoContext(ctx, "job stopped", "job", r.config.Name)
			return
		case <-ticker.C:
			if err := r.fn(ctx); err != nil {
				if r.shouldBackoff(err) {
					backoff = r.nextBackoff(backoff)
					r.logger.WarnContext(ctx, "job backing off",
						"job", r.config.Name, "backoff", backoff, "error", err)
					ticker.Reset(backoff)
					continue
				}
				r.logger.ErrorContext(ctx, "job failed", "job", r.config.Name, "error", err)
			} else {
				// Success: reset backoff if active
				if backoff > 0 {
					r.logger.InfoContext(ctx, "backoff cleared, resuming normal interval",
						"job", r.config.Name)
					backoff = 0
					ticker.Reset(r.config.Interval)
				}
			}
		}
	}
}

// shouldBackoff checks if the error should trigger a backoff.
func (r *JobRunner) shouldBackoff(err error) bool {
	for _, backoffErr := range r.config.BackoffOnErrors {
		if errors.Is(err, backoffErr) {
			return true
		}
	}
	return false
}

// nextBackoff calculates the next backoff duration.
func (r *JobRunner) nextBackoff(current time.Duration) time.Duration {
	initial := r.config.InitialBackoff
	if initial == 0 {
		initial = 30 * time.Second
	}
	maxB := r.config.MaxBackoff
	if maxB == 0 {
		maxB = 5 * time.Minute
	}

	if current == 0 {
		return initial
	}
	next := current * 2
	if next > maxB {
		return maxB
	}
	return next
}

// JobGroup manages a collection of job runners.
type JobGroup struct {
	runners []*JobRunner
	ctx     context.Context
	logger  *slog.Logger
}

// NewJobGroup creates a new job group. The provided context is used for all
// runners added via Add.
func NewJobGroup(ctx context.Context, logger *slog.Logger) *JobGroup {
	return &JobGroup{ctx: ctx, logger: logger}
}

// Add adds a job runner to the group and starts it immediately.
func (g *JobGroup) Add(runner *JobRunner) {
	g.runners = append(g.runners, runner)
	g.logger.InfoContext(g.ctx, "starting job", "job", runner.config.Name)
	runner.Start(g.ctx)
}

// StopAll stops all jobs in the group and waits for them to finish.
func (g *JobGroup) StopAll() {
	for _, r := range g.runners {
		r.Stop()
	}
}
