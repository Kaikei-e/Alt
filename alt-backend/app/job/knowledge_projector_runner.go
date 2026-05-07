package job

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"alt/utils/logger"

	"github.com/jackc/pgx/v5"
)

type KnowledgeProjectorListener interface {
	WaitForNotification(ctx context.Context) error
	Close(ctx context.Context) error
}

type KnowledgeProjectorRunnerConfig struct {
	PollInterval    time.Duration
	Timeout         time.Duration
	Process         func(ctx context.Context) error
	ListenerFactory func(ctx context.Context) (KnowledgeProjectorListener, error)

	// Bounded backoff and circuit breaker for systemic listener failures.
	// Defaults are applied when a field is zero.
	InitialBackoff   time.Duration // first delay after a listener failure (default 1s)
	MaxBackoff       time.Duration // cap on the exponential backoff (default 30s)
	BreakerThreshold int           // consecutive failures to open the breaker (default 5)
	BreakerCooldown  time.Duration // sleep between attempts while breaker is open (default 60s)
}

type KnowledgeProjectorRunner struct {
	pollInterval     time.Duration
	timeout          time.Duration
	initialBackoff   time.Duration
	maxBackoff       time.Duration
	breakerThreshold int
	breakerCooldown  time.Duration
	process          func(ctx context.Context) error
	listenerFactory  func(ctx context.Context) (KnowledgeProjectorListener, error)
}

func NewKnowledgeProjectorRunner(cfg KnowledgeProjectorRunnerConfig) *KnowledgeProjectorRunner {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 5 * time.Second
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 25 * time.Second
	}
	if cfg.InitialBackoff <= 0 {
		cfg.InitialBackoff = 1 * time.Second
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 30 * time.Second
	}
	if cfg.BreakerThreshold <= 0 {
		cfg.BreakerThreshold = 5
	}
	if cfg.BreakerCooldown <= 0 {
		cfg.BreakerCooldown = 60 * time.Second
	}
	return &KnowledgeProjectorRunner{
		pollInterval:     cfg.PollInterval,
		timeout:          cfg.Timeout,
		initialBackoff:   cfg.InitialBackoff,
		maxBackoff:       cfg.MaxBackoff,
		breakerThreshold: cfg.BreakerThreshold,
		breakerCooldown:  cfg.BreakerCooldown,
		process:          cfg.Process,
		listenerFactory:  cfg.ListenerFactory,
	}
}

func (r *KnowledgeProjectorRunner) Run(ctx context.Context) error {
	if r.process == nil {
		return fmt.Errorf("knowledge projector runner: process func is required")
	}

	if err := r.runOnce(ctx); err != nil && ctx.Err() == nil {
		logger.ErrorContext(ctx, "knowledge projector initial drain failed", "error", err)
	}

	var listener KnowledgeProjectorListener
	defer func() {
		if listener != nil {
			_ = listener.Close(context.Background())
		}
	}()

	consecutiveFailures := 0
	breakerOpen := false

	recordFailure := func(err error, msg string) {
		consecutiveFailures++
		switch {
		case consecutiveFailures == r.breakerThreshold:
			breakerOpen = true
			logger.ErrorContext(ctx, "knowledge projector circuit breaker opened",
				"consecutive_failures", consecutiveFailures,
				"cooldown", r.breakerCooldown,
				"reason", msg,
				"error", err)
		case !breakerOpen:
			logger.WarnContext(ctx, msg,
				"consecutive_failures", consecutiveFailures,
				"error", err)
		}
		// While the breaker is open and the threshold has been crossed,
		// per-iteration logs are suppressed so a sustained upstream
		// misconfiguration cannot flood the container log driver.
	}

	recordSuccess := func() {
		if consecutiveFailures > 0 {
			if breakerOpen {
				logger.InfoContext(ctx, "knowledge projector circuit breaker closed after recovery")
			}
			consecutiveFailures = 0
			breakerOpen = false
		}
	}

	failureBackoff := func() time.Duration {
		if breakerOpen {
			return r.breakerCooldown
		}
		delay := r.initialBackoff
		for i := 1; i < consecutiveFailures; i++ {
			delay *= 2
			if delay > r.maxBackoff {
				return r.maxBackoff
			}
		}
		return delay
	}

	for {
		if ctx.Err() != nil {
			return nil
		}

		if listener == nil && r.listenerFactory != nil {
			created, err := r.listenerFactory(ctx)
			if err != nil {
				recordFailure(err, "knowledge projector listener unavailable; falling back to polling")
			} else {
				// A factory call that returns a listener is not yet a
				// "success" — the upstream may still reject the first
				// WaitForNotification immediately. Only confirmed wait
				// outcomes (notification or deadline) reset the breaker.
				listener = created
			}
		}

		if listener == nil {
			delay := r.pollInterval
			if consecutiveFailures > 0 {
				delay = failureBackoff()
			}
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(delay):
				if err := r.runOnce(ctx); err != nil && ctx.Err() == nil {
					logger.ErrorContext(ctx, "knowledge projector poll run failed", "error", err)
				}
			}
			continue
		}

		waitCtx, cancel := context.WithTimeout(ctx, r.pollInterval)
		err := listener.WaitForNotification(waitCtx)
		cancel()

		if err == nil || errors.Is(err, context.DeadlineExceeded) {
			recordSuccess()
			if runErr := r.runOnce(ctx); runErr != nil && ctx.Err() == nil {
				logger.ErrorContext(ctx, "knowledge projector wake run failed", "error", runErr)
			}
			continue
		}

		if errors.Is(err, context.Canceled) && ctx.Err() != nil {
			return nil
		}

		recordFailure(err, "knowledge projector listener failed; switching to poll fallback")
		_ = listener.Close(context.Background())
		listener = nil

		if runErr := r.runOnce(ctx); runErr != nil && ctx.Err() == nil {
			logger.ErrorContext(ctx, "knowledge projector recovery run failed", "error", runErr)
		}

		// Bounded backoff before the next factory attempt prevents tight
		// ms-scale loops when the upstream is systemically broken (e.g. the
		// staging slice misroute that triggered PM-2026-042).
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(failureBackoff()):
		}
	}
}

func (r *KnowledgeProjectorRunner) runOnce(ctx context.Context) error {
	runCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	return r.process(runCtx)
}

// NewSovereignProjectorListenerFactory creates a listener factory from a
// function that connects to sovereign's WatchProjectorEvents streaming RPC.
// The projector name is bound at factory-creation time so callers can share a
// single connect closure across multiple projectors (e.g. the
// Knowledge Home and Knowledge Loop projectors both call this with their
// own name constant).
func NewSovereignProjectorListenerFactory(
	connect func(ctx context.Context, projectorName string) (KnowledgeProjectorListener, error),
	name string,
) func(ctx context.Context) (KnowledgeProjectorListener, error) {
	if name == "" {
		name = projectorName
	}
	return func(ctx context.Context) (KnowledgeProjectorListener, error) {
		return connect(ctx, name)
	}
}

type pgKnowledgeProjectorListener struct {
	conn    *pgx.Conn
	channel string
}

func NewPGKnowledgeProjectorListenerFactory(connString string, channel string) func(ctx context.Context) (KnowledgeProjectorListener, error) {
	return func(ctx context.Context) (KnowledgeProjectorListener, error) {
		conn, err := pgx.Connect(ctx, connString)
		if err != nil {
			return nil, fmt.Errorf("connect projector listener: %w", err)
		}
		safeChannel, err := sanitizeProjectorChannel(channel)
		if err != nil {
			_ = conn.Close(ctx)
			return nil, err
		}
		if _, err := conn.Exec(ctx, "LISTEN "+safeChannel); err != nil {
			_ = conn.Close(ctx)
			return nil, fmt.Errorf("listen on %s: %w", safeChannel, err)
		}
		return &pgKnowledgeProjectorListener{conn: conn, channel: channel}, nil
	}
}

func (l *pgKnowledgeProjectorListener) WaitForNotification(ctx context.Context) error {
	_, err := l.conn.WaitForNotification(ctx)
	return err
}

func (l *pgKnowledgeProjectorListener) Close(ctx context.Context) error {
	if l.conn == nil {
		return nil
	}
	return l.conn.Close(ctx)
}

var projectorChannelPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func sanitizeProjectorChannel(channel string) (string, error) {
	if !projectorChannelPattern.MatchString(channel) {
		return "", fmt.Errorf("invalid projector notify channel %q", channel)
	}
	return channel, nil
}
