// ABOUTME: This file provides context-aware structured logging for ADR 98 compliance
// ABOUTME: Supports job ID, article ID, and processing stage propagation with JSON output format
package logger

import (
	"context"
	"log/slog"
	"os"
)

type ContextKey string

const (
	// Business context keys for Alt-specific observability
	// These follow OpenTelemetry semantic conventions with 'alt.' prefix
	JobIDKey           ContextKey = "alt.job.id"
	ArticleIDKey       ContextKey = "alt.article.id"
	ProcessingStageKey ContextKey = "alt.processing.stage"
	AIPipelineKey      ContextKey = "alt.ai.pipeline"
)

// ContextLogger provides context-aware logging with ADR 98 business context
type ContextLogger struct {
	logger      *slog.Logger
	serviceName string
}

// NewContextLogger creates a new context-aware logger
func NewContextLogger(serviceName string) *ContextLogger {
	opts := &slog.HandlerOptions{
		Level: parseLevel(os.Getenv("LOG_LEVEL")),
	}
	handler := slog.NewJSONHandler(os.Stdout, opts)

	return &ContextLogger{
		logger:      slog.New(handler),
		serviceName: serviceName,
	}
}

// WithContext returns a logger with context values extracted and added as fields
func (cl *ContextLogger) WithContext(ctx context.Context) *slog.Logger {
	logger := cl.logger.With("service", cl.serviceName)

	var fields []any

	if jobID := ctx.Value(JobIDKey); jobID != nil {
		fields = append(fields, string(JobIDKey), jobID)
	}
	if articleID := ctx.Value(ArticleIDKey); articleID != nil {
		fields = append(fields, string(ArticleIDKey), articleID)
	}
	if stage := ctx.Value(ProcessingStageKey); stage != nil {
		fields = append(fields, string(ProcessingStageKey), stage)
	}
	if pipeline := ctx.Value(AIPipelineKey); pipeline != nil {
		fields = append(fields, string(AIPipelineKey), pipeline)
	}

	if len(fields) > 0 {
		logger = logger.With(fields...)
	}

	return logger
}

// Context helper functions

// WithJobID adds job ID to context for observability
func WithJobID(ctx context.Context, jobID string) context.Context {
	return context.WithValue(ctx, JobIDKey, jobID)
}

// WithArticleID adds article ID to context for observability
func WithArticleID(ctx context.Context, articleID string) context.Context {
	return context.WithValue(ctx, ArticleIDKey, articleID)
}

// WithProcessingStage adds processing stage to context for observability
func WithProcessingStage(ctx context.Context, stage string) context.Context {
	return context.WithValue(ctx, ProcessingStageKey, stage)
}

// WithAIPipeline adds AI pipeline name to context for observability
func WithAIPipeline(ctx context.Context, pipeline string) context.Context {
	return context.WithValue(ctx, AIPipelineKey, pipeline)
}

func parseLevel(level string) slog.Level {
	switch level {
	case "debug", "DEBUG":
		return slog.LevelDebug
	case "warn", "WARN", "warning", "WARNING":
		return slog.LevelWarn
	case "error", "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
