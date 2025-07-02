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

var (
	Logger *slog.Logger
)

const (
	RequestIDKey ContextKey = "request_id"
	TraceIDKey   ContextKey = "trace_id"
	OperationKey ContextKey = "operation"
	ServiceKey   ContextKey = "service"
)

type ContextLogger struct {
	logger      *slog.Logger
	serviceName string
	raskLogger  *RaskLogger
	useRask     bool
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
		useRask:     false,
	}
}

// NewContextLoggerWithConfig creates a ContextLogger based on configuration
func NewContextLoggerWithConfig(config *LoggerConfig, output io.Writer) *ContextLogger {
	if config.UseRask {
		return NewRaskContextLogger(output, config.ServiceName)
	}
	return NewContextLogger(output, config.Format, config.Level)
}

// NewRaskContextLogger creates a ContextLogger that uses RaskLogger internally
func NewRaskContextLogger(output io.Writer, serviceName string) *ContextLogger {
	raskLogger := NewRaskLogger(output, serviceName)
	
	return &ContextLogger{
		logger:      nil, // Not used when useRask is true
		serviceName: serviceName,
		raskLogger:  raskLogger,
		useRask:     true,
	}
}

func (cl *ContextLogger) WithContext(ctx context.Context) *slog.Logger {
	if cl.useRask {
		// When using Rask logger, we need to create a custom handler that ensures
		// all logs go through the RaskLogger's formatting
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
		
		// Create a wrapper handler that delegates to RaskLogger
		raskWithFields := cl.raskLogger.With(fields...)
		wrapper := &raskHandlerWrapper{raskLogger: raskWithFields}
		
		return slog.New(wrapper)
	}

	// Existing behavior for non-Rask mode
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

// raskHandlerWrapper wraps RaskLogger to implement slog.Handler interface
type raskHandlerWrapper struct {
	raskLogger *RaskLogger
	attrs      []slog.Attr
	groups     []string
}

func (w *raskHandlerWrapper) Enabled(_ context.Context, level slog.Level) bool {
	return true // Let RaskLogger decide
}

func (w *raskHandlerWrapper) Handle(_ context.Context, record slog.Record) error {
	// Convert record to RaskLogger call
	args := make([]any, 0, record.NumAttrs()*2+len(w.attrs)*2)
	
	// Add existing attributes
	for _, attr := range w.attrs {
		args = append(args, attr.Key, attr.Value.Any())
	}
	
	// Add record attributes
	record.Attrs(func(attr slog.Attr) bool {
		args = append(args, attr.Key, attr.Value.Any())
		return true
	})
	
	w.raskLogger.Log(record.Level, record.Message, args...)
	return nil
}

func (w *raskHandlerWrapper) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(w.attrs)+len(attrs))
	copy(newAttrs, w.attrs)
	copy(newAttrs[len(w.attrs):], attrs)
	
	return &raskHandlerWrapper{
		raskLogger: w.raskLogger,
		attrs:      newAttrs,
		groups:     w.groups,
	}
}

func (w *raskHandlerWrapper) WithGroup(name string) slog.Handler {
	newGroups := make([]string, len(w.groups)+1)
	copy(newGroups, w.groups)
	newGroups[len(w.groups)] = name
	
	return &raskHandlerWrapper{
		raskLogger: w.raskLogger,
		attrs:      w.attrs,
		groups:     newGroups,
	}
}
