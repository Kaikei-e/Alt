package logger

import (
	"log/slog"
	"os"
)

// Logger wraps slog.Logger with additional functionality
type Logger struct {
	*slog.Logger
}

// NewLogger creates a new logger with structured logging
func NewLogger() *Logger {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	handler := slog.NewTextHandler(os.Stdout, opts)
	logger := slog.New(handler)

	return &Logger{Logger: logger}
}

// NewLoggerWithLevel creates a new logger with specified log level
func NewLoggerWithLevel(level slog.Level) *Logger {
	opts := &slog.HandlerOptions{
		Level: level,
	}

	handler := slog.NewTextHandler(os.Stdout, opts)
	logger := slog.New(handler)

	return &Logger{Logger: logger}
}

// Common log levels
const (
	DebugLevel = slog.LevelDebug
	InfoLevel  = slog.LevelInfo
	WarnLevel  = slog.LevelWarn
	ErrorLevel = slog.LevelError
)

// WithContext adds context to the logger
func (l *Logger) WithContext(key string, value interface{}) *Logger {
	newLogger := l.Logger.With(slog.Any(key, value))
	return &Logger{Logger: newLogger}
}

// InfoWithContext logs info message with context
func (l *Logger) InfoWithContext(msg string, args ...interface{}) {
	l.Logger.Info(msg, args...)
}

// ErrorWithContext logs error message with context
func (l *Logger) ErrorWithContext(msg string, args ...interface{}) {
	l.Logger.Error(msg, args...)
}

// WarnWithContext logs warning message with context
func (l *Logger) WarnWithContext(msg string, args ...interface{}) {
	l.Logger.Warn(msg, args...)
}

// DebugWithContext logs debug message with context
func (l *Logger) DebugWithContext(msg string, args ...interface{}) {
	l.Logger.Debug(msg, args...)
}
