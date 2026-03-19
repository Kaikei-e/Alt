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
}

type KnowledgeProjectorRunner struct {
	pollInterval    time.Duration
	timeout         time.Duration
	process         func(ctx context.Context) error
	listenerFactory func(ctx context.Context) (KnowledgeProjectorListener, error)
}

func NewKnowledgeProjectorRunner(cfg KnowledgeProjectorRunnerConfig) *KnowledgeProjectorRunner {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 5 * time.Second
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 25 * time.Second
	}
	return &KnowledgeProjectorRunner{
		pollInterval:    cfg.PollInterval,
		timeout:         cfg.Timeout,
		process:         cfg.Process,
		listenerFactory: cfg.ListenerFactory,
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

	for {
		if ctx.Err() != nil {
			return nil
		}

		if listener == nil && r.listenerFactory != nil {
			created, err := r.listenerFactory(ctx)
			if err != nil {
				logger.WarnContext(ctx, "knowledge projector listener unavailable; falling back to polling", "error", err)
			} else {
				listener = created
			}
		}

		if listener == nil {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(r.pollInterval):
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
			if runErr := r.runOnce(ctx); runErr != nil && ctx.Err() == nil {
				logger.ErrorContext(ctx, "knowledge projector wake run failed", "error", runErr)
			}
			continue
		}

		if errors.Is(err, context.Canceled) && ctx.Err() != nil {
			return nil
		}

		logger.WarnContext(ctx, "knowledge projector listener failed; switching to poll fallback", "error", err)
		_ = listener.Close(context.Background())
		listener = nil

		if runErr := r.runOnce(ctx); runErr != nil && ctx.Err() == nil {
			logger.ErrorContext(ctx, "knowledge projector recovery run failed", "error", runErr)
		}
	}
}

func (r *KnowledgeProjectorRunner) runOnce(ctx context.Context) error {
	runCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	return r.process(runCtx)
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
