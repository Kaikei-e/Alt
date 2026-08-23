package orchestrator

import (
	"context"
	"errors"
	"fmt"
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
	// mu guards cancel/started/stopped so a concurrent Start and Stop (e.g.
	// Add racing StopAll in the owning JobGroup) do not data-race on the
	// cancel field or call wg.Add after wg.Wait. Once stopped, Start is a
	// no-op so a runner added after StopAll can never leak a goroutine that
	// nothing will ever stop.
	mu      sync.Mutex
	cancel  context.CancelFunc
	started bool
	stopped bool
	wg      sync.WaitGroup
}

// NewJobRunner creates a new job runner.
func NewJobRunner(config JobConfig, fn func(ctx context.Context) error, logger *slog.Logger) *JobRunner {
	return &JobRunner{
		config: config,
		fn:     fn,
		logger: logger,
	}
}

// Start starts the job runner in a goroutine. It is a no-op if the runner has
// already been started or if Stop has already run, so a Start that loses the
// race with Stop cannot leave an orphaned goroutine behind.
func (r *JobRunner) Start(ctx context.Context) {
	r.mu.Lock()
	if r.started || r.stopped {
		r.mu.Unlock()
		return
	}
	jobCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.started = true
	r.wg.Add(1)
	r.mu.Unlock()

	go func() {
		defer r.wg.Done()
		r.run(jobCtx)
	}()
}

// Stop stops the job runner and waits for it to finish. Marking stopped under
// the lock before reading cancel means a Start racing this call observes
// stopped and returns without spawning, so wg.Add can never run after the
// wg.Wait below.
func (r *JobRunner) Stop() {
	r.mu.Lock()
	r.stopped = true
	cancel := r.cancel
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	r.wg.Wait()
}

// run is the main loop of the job runner. Each iteration recovers its own
// panic via invoke — a top-level recover here would only catch the first
// panic and then return, killing the ticker loop and silently stopping the
// periodic job for good.
func (r *JobRunner) run(ctx context.Context) {
	// Run immediately if configured
	if r.config.RunImmediately {
		if err := r.invoke(ctx); err != nil {
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
			if err := r.invoke(ctx); err != nil {
				if r.shouldBackoff(err) {
					backoff = r.nextBackoff(backoff)
					r.logger.WarnContext(ctx, "job backing off",
						"job", r.config.Name, "backoff", backoff, "error", err)
					ticker.Reset(backoff)
					continue
				}
				r.logger.ErrorContext(ctx, "job failed", "job", r.config.Name, "error", err)
				// Non-backoff failure must not leave the ticker stuck on a
				// prior backoff interval — re-evaluate to the configured cadence.
				if backoff > 0 {
					r.logger.InfoContext(ctx, "backoff cleared after non-backoff error, resuming normal interval",
						"job", r.config.Name)
					backoff = 0
					ticker.Reset(r.config.Interval)
				}
			} else if backoff > 0 {
				// Success: reset backoff if active
				r.logger.InfoContext(ctx, "backoff cleared, resuming normal interval",
					"job", r.config.Name)
				backoff = 0
				ticker.Reset(r.config.Interval)
			}
		}
	}
}

// invoke calls r.fn, recovering any panic so a single bad iteration reports
// as a failed run instead of unwinding the whole job loop.
func (r *JobRunner) invoke(ctx context.Context) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			r.logger.ErrorContext(ctx, "panic in job runner", "job", r.config.Name, "panic", rec)
			err = fmt.Errorf("panic recovered: %v", rec)
		}
	}()
	return r.fn(ctx)
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

// JobGroup manages a collection of job runners. Add may be called
// concurrently — e.g. once a health-gated job's background wait completes —
// so runners and mu guard against a concurrent StopAll or another Add.
type JobGroup struct {
	mu      sync.Mutex
	runners []*JobRunner
	stopped bool
	ctx     context.Context
	logger  *slog.Logger
}

// NewJobGroup creates a new job group. The provided context is used for all
// runners added via Add.
func NewJobGroup(ctx context.Context, logger *slog.Logger) *JobGroup {
	return &JobGroup{ctx: ctx, logger: logger}
}

// Add adds a job runner to the group and starts it immediately. Registration
// and Start happen together under the lock, and both are skipped once StopAll
// has run: otherwise a runner added concurrently with StopAll could be started
// after StopAll had already snapshotted and stopped the group, leaving a
// goroutine that nothing will ever stop.
func (g *JobGroup) Add(runner *JobRunner) {
	g.mu.Lock()
	if g.stopped {
		g.mu.Unlock()
		g.logger.InfoContext(g.ctx, "job group already stopped, not starting job", "job", runner.config.Name)
		return
	}
	g.runners = append(g.runners, runner)
	g.logger.InfoContext(g.ctx, "starting job", "job", runner.config.Name)
	runner.Start(g.ctx)
	g.mu.Unlock()
}

// StopAll stops all jobs in the group and waits for them to finish. Setting
// stopped under the lock before snapshotting means any Add still to come is a
// no-op, so no runner escapes being stopped.
func (g *JobGroup) StopAll() {
	g.mu.Lock()
	g.stopped = true
	runners := make([]*JobRunner, len(g.runners))
	copy(runners, g.runners)
	g.mu.Unlock()
	for _, r := range runners {
		r.Stop()
	}
}
