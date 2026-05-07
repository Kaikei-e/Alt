package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
)

// dynamicHandler delegates to a swappable inner slog.Handler. The inner
// handler is held in an atomic.Pointer so swaps are race-free with concurrent
// readers — and, critically, the package-level *slog.Logger that wraps this
// handler can stay stable for the lifetime of the process. That stability is
// what allows hundreds of `logger.Logger.X(...)` call sites to remain safe
// without an accessor migration.
//
// Pattern reference: log/slog itself stores its default logger in an
// atomic.Pointer[Logger] (see go.dev/src/log/slog/logger.go). We use the same
// approach one level down (the handler instead of the logger) so that the
// public *slog.Logger value is never reassigned after package init.
type dynamicHandler struct {
	inner atomic.Pointer[slog.Handler]
}

func (d *dynamicHandler) currentHandler() slog.Handler {
	if h := d.inner.Load(); h != nil {
		return *h
	}
	return nil
}

func (d *dynamicHandler) Enabled(ctx context.Context, lvl slog.Level) bool {
	if h := d.currentHandler(); h != nil {
		return h.Enabled(ctx, lvl)
	}
	return false
}

func (d *dynamicHandler) Handle(ctx context.Context, r slog.Record) error {
	if h := d.currentHandler(); h != nil {
		return h.Handle(ctx, r)
	}
	return nil
}

func (d *dynamicHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if h := d.currentHandler(); h != nil {
		return h.WithAttrs(attrs)
	}
	return d
}

func (d *dynamicHandler) WithGroup(name string) slog.Handler {
	if h := d.currentHandler(); h != nil {
		return h.WithGroup(name)
	}
	return d
}

// swap atomically replaces the underlying handler. Safe to call concurrently
// with any number of Handle/Enabled/WithAttrs/WithGroup readers.
func (d *dynamicHandler) swap(h slog.Handler) {
	d.inner.Store(&h)
}

var (
	rootHandler   = &dynamicHandler{}
	Logger        *slog.Logger
	GlobalContext *ContextLogger
	GlobalPerf    *PerformanceLogger
	otelEnabledV  atomic.Bool
)

type LogConfig struct {
	Level       slog.Level
	Format      string // "text" or "json"
	OTelEnabled bool
}

func init() {
	// Pre-initialize the package globals before any goroutine can observe
	// them. Subsequent InitLoggerWithOTel calls atomically swap the
	// underlying handler without reassigning Logger / GlobalContext /
	// GlobalPerf, eliminating the data race that occurs when one test calls
	// InitLogger while a previous test's fire-and-forget goroutine is still
	// reading the globals.
	rootHandler.swap(buildHandler(false))
	Logger = slog.New(rootHandler)
	slog.SetDefault(Logger)
	GlobalContext = NewContextLogger(Logger)
	GlobalPerf = NewPerformanceLogger(Logger)
}

// InitLogger initializes the logger (legacy mode - stdout only)
func InitLogger() *slog.Logger {
	return InitLoggerWithOTel(false)
}

// InitLoggerWithOTel atomically swaps the active handler. The package globals
// (Logger, GlobalContext, GlobalPerf) keep the same pointer values they had
// after package init, so any goroutine already holding a reference continues
// to see a valid Logger and observes the new handler on its next call.
//
// Production calls this exactly once from main.go after the OTel provider is
// initialized (or has been confirmed unavailable). Tests may call it for
// setup; it is now safe to do so concurrently with background goroutines
// from prior tests.
func InitLoggerWithOTel(enableOTel bool) *slog.Logger {
	otelEnabledV.Store(enableOTel)
	rootHandler.swap(buildHandler(enableOTel))
	config := getLogConfig()
	Logger.Info("Logger initialized",
		"level", config.Level.String(),
		"format", config.Format,
		"otel_enabled", enableOTel,
	)
	return Logger
}

func buildHandler(enableOTel bool) slog.Handler {
	config := getLogConfig()
	config.OTelEnabled = enableOTel

	if enableOTel && strings.ToLower(config.Format) == "json" {
		// Use MultiHandler for JSON + OTel
		return NewMultiHandler(config.Level)
	}

	options := &slog.HandlerOptions{Level: config.Level}
	switch strings.ToLower(config.Format) {
	case "json":
		// Always wrap with TraceContextHandler to add trace_id/span_id to stdout logs
		return NewTraceContextHandler(slog.NewJSONHandler(os.Stdout, options))
	default:
		return slog.NewTextHandler(os.Stdout, options)
	}
}

// IsOTelEnabled returns whether OTel is enabled
func IsOTelEnabled() bool {
	return otelEnabledV.Load()
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
