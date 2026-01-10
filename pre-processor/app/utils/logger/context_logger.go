// ABOUTME: This file provides context-aware structured logging for rask integration
// ABOUTME: Supports request ID and trace ID propagation with JSON output format
package logger

import (
	"context"
	"log/slog"
	"os"
)

type ContextKey string

// Logger is imported from global_logger.go

const (
	RequestIDKey ContextKey = "request_id"
	TraceIDKey   ContextKey = "trace_id"
	OperationKey ContextKey = "operation"
	ServiceKey   ContextKey = "service"
)

type ContextLogger struct {
	logger        *slog.Logger
	serviceName   string
	unifiedLogger *UnifiedLogger // New unified logger implementation
}

type LoggerConfig struct {
	Level       string
	Format      string
	ServiceName string
	UseRask     bool
}

func LoadLoggerConfigFromEnv() *LoggerConfig {
	useRask := getEnvOrDefault("USE_RASK_LOGGER", "false") == "true"
	return &LoggerConfig{
		Level:       getEnvOrDefault("LOG_LEVEL", "info"),
		Format:      getEnvOrDefault("LOG_FORMAT", "json"),
		ServiceName: getEnvOrDefault("SERVICE_NAME", "pre-processor"),
		UseRask:     useRask,
	}
}

func NewContextLogger(format, level string) *ContextLogger {
	// Use UnifiedLogger internally for consistent Alt-backend compatible output
	unifiedLogger := NewUnifiedLoggerWithLevel("pre-processor", level)

	return &ContextLogger{
		logger:        unifiedLogger.logger,
		serviceName:   "pre-processor",
		unifiedLogger: unifiedLogger,
	}
}

// NewContextLoggerWithConfig creates a ContextLogger based on configuration
func NewContextLoggerWithConfig(config *LoggerConfig) *ContextLogger {
	// Always use UnifiedLogger now - UseRask flag is deprecated
	unifiedLogger := NewUnifiedLoggerWithLevel(config.ServiceName, config.Level)

	return &ContextLogger{
		logger:        unifiedLogger.logger,
		serviceName:   config.ServiceName,
		unifiedLogger: unifiedLogger,
	}
}

// NewContextLoggerWithOTel creates a ContextLogger with OTel support
func NewContextLoggerWithOTel(config *LoggerConfig, enableOTel bool) *ContextLogger {
	unifiedLogger := NewUnifiedLoggerWithOTel(config.ServiceName, config.Level, enableOTel)

	return &ContextLogger{
		logger:        unifiedLogger.logger,
		serviceName:   config.ServiceName,
		unifiedLogger: unifiedLogger,
	}
}

// NewRaskContextLogger creates a ContextLogger that uses UnifiedLogger internally
// Deprecated: USE_RASK_LOGGER flag is deprecated, always uses UnifiedLogger now
func NewRaskContextLogger(serviceName string) *ContextLogger {
	// Use UnifiedLogger instead of RaskLogger for consistency
	unifiedLogger := NewUnifiedLogger(serviceName)

	return &ContextLogger{
		logger:        unifiedLogger.logger,
		serviceName:   serviceName,
		unifiedLogger: unifiedLogger,
	}
}

func (cl *ContextLogger) WithContext(ctx context.Context) *slog.Logger {
	// Always use UnifiedLogger for consistent Alt-backend compatible output
	if cl.unifiedLogger != nil {
		return cl.unifiedLogger.WithContext(ctx)
	}

	// Fallback for backward compatibility (should not happen with new implementation)
	logger := cl.logger.With("service", cl.serviceName, "version", "1.0.0")

	// Add context values directly
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

// Deprecated RaskLogger wrapper code removed - using UnifiedLogger consistently
