// ABOUTME: This file provides slog-based unified logger for rask-log-aggregator compatibility
// ABOUTME: Implements Alt-backend compatible JSON logging with context integration
package logger

import (
	"context"
	"log/slog"
	"os"
)

// UnifiedLogger provides slog-based logging compatible with Alt-backend format
type UnifiedLogger struct {
	logger      *slog.Logger
	serviceName string
}

// NewUnifiedLogger creates a new UnifiedLogger that outputs Alt-backend compatible JSON
func NewUnifiedLogger(serviceName string) *UnifiedLogger {
	// Configure Alt-backend compatible JSON handler

	options := &slog.HandlerOptions{
		Level:     slog.LevelDebug, // Allow all levels for compatibility
		AddSource: false,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Transform attributes to match Alt-backend format exactly
			switch a.Key {
			case slog.TimeKey:
				// Keep "time" field name like Alt-backend
				return slog.Attr{Key: "time", Value: a.Value}
			case slog.LevelKey:
				// Convert to "level" and keep original case for rask-log-forwarder compatibility
				if level, ok := a.Value.Any().(slog.Level); ok {
					return slog.Attr{Key: "level", Value: slog.StringValue(level.String())}
				}
				return a
			case slog.MessageKey:
				// Convert to "msg" field name like Alt-backend
				return slog.Attr{Key: "msg", Value: a.Value}
			default:
				return a
			}
		},
	}

	handler := slog.NewJSONHandler(os.Stdout, options)

	// Pre-configure with service name and version like Alt-backend
	logger := slog.New(handler).With("service", serviceName, "version", "1.0.0")

	return &UnifiedLogger{
		logger:      logger,
		serviceName: serviceName,
	}
}

// WithContext creates a context-aware logger with extracted context values
func (ul *UnifiedLogger) WithContext(ctx context.Context) *slog.Logger {
	var fields []any

	// Extract context values and add as fields (like Alt-backend)
	if requestID := ctx.Value(RequestIDKey); requestID != nil {
		fields = append(fields, "request_id", requestID)
	}

	if traceID := ctx.Value(TraceIDKey); traceID != nil {
		fields = append(fields, "trace_id", traceID)
	}

	if operation := ctx.Value(OperationKey); operation != nil {
		fields = append(fields, "operation", operation)
	}

	// Business context fields for Alt-specific observability
	// Keys use 'alt.' prefix following OTel semantic conventions
	if feedID := ctx.Value(FeedIDKey); feedID != nil {
		fields = append(fields, string(FeedIDKey), feedID)
	}

	if articleID := ctx.Value(ArticleIDKey); articleID != nil {
		fields = append(fields, string(ArticleIDKey), articleID)
	}

	if jobID := ctx.Value(JobIDKey); jobID != nil {
		fields = append(fields, string(JobIDKey), jobID)
	}

	if stage := ctx.Value(ProcessingStageKey); stage != nil {
		fields = append(fields, string(ProcessingStageKey), stage)
	}

	if pipeline := ctx.Value(AIPipelineKey); pipeline != nil {
		fields = append(fields, string(AIPipelineKey), pipeline)
	}

	// Return slog logger with context fields
	if len(fields) > 0 {
		return ul.logger.With(fields...)
	}

	return ul.logger
}

// Info logs an info message (convenience method)
func (ul *UnifiedLogger) Info(msg string, args ...any) {
	ul.logger.Info(msg, args...)
}

// Error logs an error message (convenience method)
func (ul *UnifiedLogger) Error(msg string, args ...any) {
	ul.logger.Error(msg, args...)
}

// Debug logs a debug message (convenience method)
func (ul *UnifiedLogger) Debug(msg string, args ...any) {
	ul.logger.Debug(msg, args...)
}

// Warn logs a warning message (convenience method)
func (ul *UnifiedLogger) Warn(msg string, args ...any) {
	ul.logger.Warn(msg, args...)
}

// With returns a logger with additional attributes (convenience method)
func (ul *UnifiedLogger) With(args ...any) *UnifiedLogger {
	newLogger := ul.logger.With(args...)
	return &UnifiedLogger{
		logger:      newLogger,
		serviceName: ul.serviceName,
	}
}
