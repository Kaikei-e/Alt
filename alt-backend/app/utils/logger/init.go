package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

var (
	Logger        *slog.Logger
	GlobalContext *ContextLogger
	GlobalPerf    *PerformanceLogger
	otelEnabled   bool
)

type LogConfig struct {
	Level       slog.Level
	Format      string // "text" or "json"
	OTelEnabled bool
}

// InitLogger initializes the logger (legacy mode - stdout only)
func InitLogger() *slog.Logger {
	return InitLoggerWithOTel(false)
}

// InitLoggerWithOTel initializes the logger with optional OTel support
func InitLoggerWithOTel(enableOTel bool) *slog.Logger {
	config := getLogConfig()
	config.OTelEnabled = enableOTel
	otelEnabled = enableOTel

	var handler slog.Handler

	if enableOTel && strings.ToLower(config.Format) == "json" {
		// Use MultiHandler for JSON + OTel
		handler = NewMultiHandler(config.Level)
	} else {
		// Fallback to single handler
		options := &slog.HandlerOptions{
			Level: config.Level,
		}
		switch strings.ToLower(config.Format) {
		case "json":
			jsonHandler := slog.NewJSONHandler(os.Stdout, options)
			// Always wrap with TraceContextHandler to add trace_id/span_id to stdout logs
			handler = NewTraceContextHandler(jsonHandler)
		default:
			handler = slog.NewTextHandler(os.Stdout, options)
		}
	}

	Logger = slog.New(handler)
	slog.SetDefault(Logger)

	// Initialize context and performance loggers
	GlobalContext = NewContextLogger(Logger)
	GlobalPerf = NewPerformanceLogger(Logger)

	Logger.Info("Logger initialized",
		"level", config.Level.String(),
		"format", config.Format,
		"otel_enabled", enableOTel,
	)

	return Logger
}

// IsOTelEnabled returns whether OTel is enabled
func IsOTelEnabled() bool {
	return otelEnabled
}

func getLogConfig() LogConfig {
	config := LogConfig{
		Level:  slog.LevelInfo,
		Format: "json",
	}

	// Read log level from environment
	if levelStr := os.Getenv("LOG_LEVEL"); levelStr != "" {
		switch strings.ToUpper(levelStr) {
		case "DEBUG":
			config.Level = slog.LevelDebug
		case "INFO":
			config.Level = slog.LevelInfo
		case "WARN", "WARNING":
			config.Level = slog.LevelWarn
		case "ERROR":
			config.Level = slog.LevelError
		}
	}

	// Read log format from environment
	if formatStr := os.Getenv("LOG_FORMAT"); formatStr != "" {
		config.Format = strings.ToLower(formatStr)
	}

	return config
}

// SafeInfo logs an info message if the logger is initialized, otherwise does nothing
func SafeInfo(msg string, args ...any) {
	if Logger != nil {
		Logger.Info(msg, args...)
	}
}

// SafeError logs an error message if the logger is initialized, otherwise does nothing
func SafeError(msg string, args ...any) {
	if Logger != nil {
		Logger.Error(msg, args...)
	}
}

// SafeWarn logs a warning message if the logger is initialized, otherwise does nothing
func SafeWarn(msg string, args ...any) {
	if Logger != nil {
		Logger.Warn(msg, args...)
	}
}

// InfoContext logs at INFO level with trace context
func InfoContext(ctx context.Context, msg string, args ...any) {
	if Logger != nil {
		Logger.InfoContext(ctx, msg, args...)
	}
}

// ErrorContext logs at ERROR level with trace context
func ErrorContext(ctx context.Context, msg string, args ...any) {
	if Logger != nil {
		Logger.ErrorContext(ctx, msg, args...)
	}
}

// WarnContext logs at WARN level with trace context
func WarnContext(ctx context.Context, msg string, args ...any) {
	if Logger != nil {
		Logger.WarnContext(ctx, msg, args...)
	}
}

// DebugContext logs at DEBUG level with trace context
func DebugContext(ctx context.Context, msg string, args ...any) {
	if Logger != nil {
		Logger.DebugContext(ctx, msg, args...)
	}
}

// SafeInfoContext logs at INFO level with trace context (nil-safe)
func SafeInfoContext(ctx context.Context, msg string, args ...any) {
	if Logger != nil {
		Logger.InfoContext(ctx, msg, args...)
	}
}

// SafeErrorContext logs at ERROR level with trace context (nil-safe)
func SafeErrorContext(ctx context.Context, msg string, args ...any) {
	if Logger != nil {
		Logger.ErrorContext(ctx, msg, args...)
	}
}

// SafeWarnContext logs at WARN level with trace context (nil-safe)
func SafeWarnContext(ctx context.Context, msg string, args ...any) {
	if Logger != nil {
		Logger.WarnContext(ctx, msg, args...)
	}
}

// SafeDebugContext logs at DEBUG level with trace context (nil-safe)
func SafeDebugContext(ctx context.Context, msg string, args ...any) {
	if Logger != nil {
		Logger.DebugContext(ctx, msg, args...)
	}
}
