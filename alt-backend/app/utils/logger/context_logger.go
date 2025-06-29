package logger

import (
	"context"
	"log/slog"
	"time"
)

type ContextKey string

const (
	RequestIDKey ContextKey = "request_id"
	UserIDKey    ContextKey = "user_id"
	OperationKey ContextKey = "operation"
)

type ContextLogger struct {
	logger *slog.Logger
}

func NewContextLogger(logger *slog.Logger) *ContextLogger {
	return &ContextLogger{logger: logger}
}

// WithContext adds context values to log entries
func (cl *ContextLogger) WithContext(ctx context.Context) *slog.Logger {
	args := make([]any, 0)

	if requestID := ctx.Value(RequestIDKey); requestID != nil {
		args = append(args, "request_id", requestID.(string))
	}

	if userID := ctx.Value(UserIDKey); userID != nil {
		args = append(args, "user_id", userID.(string))
	}

	if operation := ctx.Value(OperationKey); operation != nil {
		args = append(args, "operation", operation.(string))
	}

	return cl.logger.With(args...)
}

// Performance logging helpers
func (cl *ContextLogger) LogDuration(ctx context.Context, operation string, duration time.Duration) {
	cl.WithContext(ctx).Info("operation completed",
		"operation", operation,
		"duration_ms", duration.Milliseconds(),
	)
}

func (cl *ContextLogger) LogError(ctx context.Context, operation string, err error) {
	cl.WithContext(ctx).Error("operation failed",
		"operation", operation,
		"error", err,
	)
}
