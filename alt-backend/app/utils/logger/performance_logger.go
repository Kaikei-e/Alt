package logger

import (
	"context"
	"log/slog"
	"time"
)

type PerformanceLogger struct {
	contextLogger *ContextLogger
}

type Timer struct {
	ctx        context.Context
	operation  string
	startTime  time.Time
	perfLogger *PerformanceLogger
}

func NewPerformanceLogger(logger *slog.Logger) *PerformanceLogger {
	return &PerformanceLogger{
		contextLogger: NewContextLogger(logger),
	}
}

// StartTimer creates a new timer for measuring operation performance
func (pl *PerformanceLogger) StartTimer(ctx context.Context, operation string) *Timer {
	return &Timer{
		ctx:        ctx,
		operation:  operation,
		startTime:  time.Now(),
		perfLogger: pl,
	}
}

// End completes the timer and logs the duration
func (t *Timer) End() {
	duration := time.Since(t.startTime)
	t.perfLogger.contextLogger.LogDuration(t.ctx, t.operation, duration)
}

// EndWithError completes the timer and logs an error along with the duration
func (t *Timer) EndWithError(err error) {
	duration := time.Since(t.startTime)

	// Log the error first
	t.perfLogger.contextLogger.LogError(t.ctx, t.operation, err)

	// Then log the duration
	t.perfLogger.contextLogger.WithContext(t.ctx).Info("operation failed after duration",
		"operation", t.operation,
		"duration_ms", duration.Milliseconds(),
		"error", err,
	)
}

// LogSlowOperation logs a warning when an operation exceeds a threshold
func (pl *PerformanceLogger) LogSlowOperation(ctx context.Context, operation string, duration, threshold time.Duration) {
	if duration > threshold {
		pl.contextLogger.WithContext(ctx).Warn("slow operation detected",
			"operation", operation,
			"duration_ms", duration.Milliseconds(),
			"threshold_ms", threshold.Milliseconds(),
		)
	}
}

// LogOperationMetrics logs detailed metrics for an operation
func (pl *PerformanceLogger) LogOperationMetrics(ctx context.Context, operation string, duration time.Duration, extraFields map[string]any) {
	args := []any{
		"operation", operation,
		"duration_ms", duration.Milliseconds(),
	}

	for key, value := range extraFields {
		args = append(args, key, value)
	}

	pl.contextLogger.WithContext(ctx).Info("operation metrics", args...)
}
