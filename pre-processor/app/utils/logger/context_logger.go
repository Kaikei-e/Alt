// ABOUTME: This file provides context-aware structured logging for rask integration
// ABOUTME: Supports request ID and trace ID propagation with JSON output format
package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

type ContextKey string

const (
	RequestIDKey ContextKey = "request_id"
	TraceIDKey   ContextKey = "trace_id"
	OperationKey ContextKey = "operation"
	ServiceKey   ContextKey = "service"
)

type ContextLogger struct {
	logger      *slog.Logger
	serviceName string
}

type LoggerConfig struct {
	Level       string
	Format      string
	ServiceName string
}

func LoadLoggerConfigFromEnv() *LoggerConfig {
	return &LoggerConfig{
		Level:       getEnvOrDefault("LOG_LEVEL", "info"),
		Format:      getEnvOrDefault("LOG_FORMAT", "json"),
		ServiceName: getEnvOrDefault("SERVICE_NAME", "pre-processor"),
	}
}

func NewContextLogger(output io.Writer, format, level string) *ContextLogger {
	var handler slog.Handler

	// Configure log level
	var logLevel slog.Level
	switch strings.ToUpper(level) {
	case "DEBUG":
		logLevel = slog.LevelDebug
	case "INFO":
		logLevel = slog.LevelInfo
	case "WARN":
		logLevel = slog.LevelWarn
	case "ERROR":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	options := &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: logLevel == slog.LevelDebug,
		// Use ReplaceAttr to customize field names for rask compatibility
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case slog.TimeKey:
				return slog.Attr{Key: "time", Value: a.Value}
			case slog.LevelKey:
				// Convert level to lowercase for rask compatibility
				if level, ok := a.Value.Any().(slog.Level); ok {
					return slog.Attr{Key: "level", Value: slog.StringValue(strings.ToLower(level.String()))}
				}
				return a
			case slog.MessageKey:
				return slog.Attr{Key: "msg", Value: a.Value}
			default:
				return a
			}
		},
	}

	// Configure output format for rask integration
	switch strings.ToLower(format) {
	case "json":
		handler = slog.NewJSONHandler(output, options)
	default:
		handler = slog.NewTextHandler(output, options)
	}

	logger := slog.New(handler)

	return &ContextLogger{
		logger:      logger,
		serviceName: "pre-processor",
	}
}

func (cl *ContextLogger) WithContext(ctx context.Context) *slog.Logger {
	// Add rask-compatible fields
	logger := cl.logger.With(
		"service", cl.serviceName,
		"version", "1.0.0",
	)

	// Add context values directly (not nested)
	var fields []any

	if requestID := ctx.Value(RequestIDKey); requestID != nil {
		fields = append(fields, "request_id", requestID)
	}

	if traceID := ctx.Value(TraceIDKey); traceID != nil {
		fields = append(fields, "trace_id", traceID)
	}

	if operation := ctx.Value(OperationKey); operation != nil {
		fields = append(fields, "operation", operation)
	}

	if len(fields) > 0 {
		logger = logger.With(fields...)
	}

	return logger
}

// Context helper functions
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDKey, traceID)
}

func WithOperation(ctx context.Context, operation string) context.Context {
	return context.WithValue(ctx, OperationKey, operation)
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
