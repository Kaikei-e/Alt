// Package logger provides structured logging for mq-hub service with ADR 98/99 compliance.
package logger

import (
	"log/slog"
	"os"
	"strings"
)

// Logger is the global logger instance
var Logger *slog.Logger

// GlobalContext is the global ContextLogger instance for ADR 98/99 business context support
var GlobalContext *ContextLogger

// Init initializes a JSON logger
func Init() *slog.Logger {
	level := parseLevel(os.Getenv("LOG_LEVEL"))

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})

	Logger = slog.New(handler)
	slog.SetDefault(Logger)

	// Initialize ContextLogger for ADR 98/99 business context support
	GlobalContext = NewContextLogger(Logger)

	Logger.Info("Logger initialized", "level", level.String())

	return Logger
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
