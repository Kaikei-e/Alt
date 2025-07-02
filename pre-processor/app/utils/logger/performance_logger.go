// ABOUTME: This file provides performance monitoring and timing for operations
// ABOUTME: Detects slow operations and logs detailed timing information
package logger

import (
	"context"
	"log/slog"
	"time"
)

type PerformanceLogger struct {
	contextLogger *ContextLogger
	threshold     time.Duration
}

type Timer struct {
	logger    *slog.Logger
	operation string
	startTime time.Time
	threshold time.Duration
}

func NewPerformanceLogger(slowThreshold time.Duration) *PerformanceLogger {
	contextLogger := NewContextLogger("json", "debug")

	return &PerformanceLogger{
		contextLogger: contextLogger,
		threshold:     slowThreshold,
	}
}

func (pl *PerformanceLogger) StartTimer(ctx context.Context, operation string) *Timer {
	logger := pl.contextLogger.WithContext(ctx)

	logger.Debug("operation started", "operation", operation)

	return &Timer{
		logger:    logger,
		operation: operation,
		startTime: time.Now(),
		threshold: pl.threshold,
	}
}

func (t *Timer) End() {
	duration := time.Since(t.startTime)

	t.logger.Info("operation completed",
		"operation", t.operation,
		"duration_ms", duration.Milliseconds(),
	)

	// Warn about slow operations
	if duration > t.threshold {
		t.logger.Warn("slow operation detected",
			"operation", t.operation,
			"duration_ms", duration.Milliseconds(),
			"threshold_ms", t.threshold.Milliseconds(),
		)
	}
}
