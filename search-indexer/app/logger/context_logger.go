package logger

import (
	"context"
	"log/slog"
	"time"
)

// ContextKey is the type for context keys used in logging
type ContextKey string

const (
	RequestIDKey ContextKey = "request_id"
	UserIDKey    ContextKey = "user_id"
	OperationKey ContextKey = "operation"

	// Business context keys for Alt-specific observability (ADR 98/99)
	// These follow OpenTelemetry semantic conventions with 'alt.' prefix
	FeedIDKey          ContextKey = "alt.feed.id"
	ArticleIDKey       ContextKey = "alt.article.id"
	JobIDKey           ContextKey = "alt.job.id"
	ProcessingStageKey ContextKey = "alt.processing.stage"
	AIPipelineKey      ContextKey = "alt.ai.pipeline"
)

// GlobalContext is the global ContextLogger instance
var GlobalContext *ContextLogger

// ContextLogger wraps a slog.Logger to add context-aware logging
type ContextLogger struct {
	logger *slog.Logger
}

// NewContextLogger creates a new ContextLogger wrapping the provided logger
func NewContextLogger(logger *slog.Logger) *ContextLogger {
	return &ContextLogger{logger: logger}
}

// WithContext adds context values to log entries and returns a new logger
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

	// Business context fields for Alt-specific observability (ADR 98/99)
	if feedID := ctx.Value(FeedIDKey); feedID != nil {
		args = append(args, string(FeedIDKey), feedID.(string))
	}

	if articleID := ctx.Value(ArticleIDKey); articleID != nil {
		args = append(args, string(ArticleIDKey), articleID.(string))
	}

	if jobID := ctx.Value(JobIDKey); jobID != nil {
		args = append(args, string(JobIDKey), jobID.(string))
	}

	if stage := ctx.Value(ProcessingStageKey); stage != nil {
		args = append(args, string(ProcessingStageKey), stage.(string))
	}

	if pipeline := ctx.Value(AIPipelineKey); pipeline != nil {
		args = append(args, string(AIPipelineKey), pipeline.(string))
	}

	return cl.logger.With(args...)
}

// LogDuration logs an operation completion with duration in milliseconds
func (cl *ContextLogger) LogDuration(ctx context.Context, operation string, durationMs int64) {
	cl.WithContext(ctx).Info("operation completed",
		"operation", operation,
		"duration_ms", durationMs,
	)
}

// LogError logs an operation failure with error details
func (cl *ContextLogger) LogError(ctx context.Context, operation string, err error) {
	cl.WithContext(ctx).Error("operation failed",
		"operation", operation,
		"error", err,
	)
}

// Context helper functions for business context

// WithFeedID adds feed ID to context for observability
func WithFeedID(ctx context.Context, feedID string) context.Context {
	return context.WithValue(ctx, FeedIDKey, feedID)
}

// WithArticleID adds article ID to context for observability
func WithArticleID(ctx context.Context, articleID string) context.Context {
	return context.WithValue(ctx, ArticleIDKey, articleID)
}

// WithJobID adds job ID to context for observability
func WithJobID(ctx context.Context, jobID string) context.Context {
	return context.WithValue(ctx, JobIDKey, jobID)
}

// WithProcessingStage adds processing stage to context for observability
func WithProcessingStage(ctx context.Context, stage string) context.Context {
	return context.WithValue(ctx, ProcessingStageKey, stage)
}

// WithAIPipeline adds AI pipeline name to context for observability
func WithAIPipeline(ctx context.Context, pipeline string) context.Context {
	return context.WithValue(ctx, AIPipelineKey, pipeline)
}

// LogDurationTime is a convenience function that takes time.Duration
func (cl *ContextLogger) LogDurationTime(ctx context.Context, operation string, duration time.Duration) {
	cl.LogDuration(ctx, operation, duration.Milliseconds())
}
