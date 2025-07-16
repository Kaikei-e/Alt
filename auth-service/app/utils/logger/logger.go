package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"
)

// LogLevel represents the logging level
type LogLevel string

const (
	LevelDebug LogLevel = "debug"
	LevelInfo  LogLevel = "info"
	LevelWarn  LogLevel = "warn"
	LevelError LogLevel = "error"
)

// New creates a new structured logger with the specified level
func New(level string) (*slog.Logger, error) {
	logLevel, err := parseLogLevel(level)
	if err != nil {
		return nil, fmt.Errorf("invalid log level: %w", err)
	}

	// Create handler options
	opts := &slog.HandlerOptions{
		Level: logLevel,
		AddSource: logLevel == slog.LevelDebug,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Format time in RFC3339 format
			if a.Key == slog.TimeKey {
				if t, ok := a.Value.Any().(time.Time); ok {
					a.Value = slog.StringValue(t.Format(time.RFC3339))
				}
			}
			return a
		},
	}

	// Create handler based on environment
	var handler slog.Handler
	if isProduction() {
		// JSON format for production
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		// Text format for development
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	// Create logger with service context
	logger := slog.New(handler).With(
		"service", "auth-service",
		"component", "main",
	)

	return logger, nil
}

// NewWithWriter creates a logger with a custom writer (useful for testing)
func NewWithWriter(level string, writer io.Writer) (*slog.Logger, error) {
	logLevel, err := parseLogLevel(level)
	if err != nil {
		return nil, fmt.Errorf("invalid log level: %w", err)
	}

	opts := &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: logLevel == slog.LevelDebug,
	}

	var handler slog.Handler
	if isProduction() {
		handler = slog.NewJSONHandler(writer, opts)
	} else {
		handler = slog.NewTextHandler(writer, opts)
	}

	logger := slog.New(handler).With(
		"service", "auth-service",
	)

	return logger, nil
}

// WithComponent creates a logger with component context
func WithComponent(logger *slog.Logger, component string) *slog.Logger {
	return logger.With("component", component)
}

// WithUser creates a logger with user context
func WithUser(logger *slog.Logger, userID string) *slog.Logger {
	return logger.With("user_id", userID)
}

// WithTenant creates a logger with tenant context
func WithTenant(logger *slog.Logger, tenantID string) *slog.Logger {
	return logger.With("tenant_id", tenantID)
}

// WithRequest creates a logger with request context
func WithRequest(logger *slog.Logger, requestID, method, path string) *slog.Logger {
	return logger.With(
		"request_id", requestID,
		"method", method,
		"path", path,
	)
}

// LogError logs an error with additional context
func LogError(logger *slog.Logger, err error, msg string, keysAndValues ...interface{}) {
	args := []interface{}{"error", err}
	args = append(args, keysAndValues...)
	logger.Error(msg, args...)
}

// LogDuration logs the duration of an operation
func LogDuration(logger *slog.Logger, start time.Time, operation string, keysAndValues ...interface{}) {
	duration := time.Since(start)
	args := []interface{}{
		"operation", operation,
		"duration_ms", duration.Milliseconds(),
	}
	args = append(args, keysAndValues...)
	logger.Info("Operation completed", args...)
}

// Helper functions

func parseLogLevel(level string) (slog.Level, error) {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown log level: %s", level)
	}
}

func isProduction() bool {
	env := strings.ToLower(os.Getenv("GO_ENV"))
	return env == "production" || env == "prod"
}

// Middleware helpers for request logging

// RequestLogger returns a middleware function that logs HTTP requests
func RequestLogger(logger *slog.Logger) func(next func()) func() {
	return func(next func()) func() {
		return func() {
			start := time.Now()
			next()
			LogDuration(logger, start, "http_request")
		}
	}
}

// DatabaseLogger creates a logger specifically for database operations
func DatabaseLogger(logger *slog.Logger) *slog.Logger {
	return WithComponent(logger, "database")
}

// KratosLogger creates a logger specifically for Kratos operations
func KratosLogger(logger *slog.Logger) *slog.Logger {
	return WithComponent(logger, "kratos")
}