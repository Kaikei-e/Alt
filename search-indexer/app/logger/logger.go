package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/log/global"
)

var Logger *slog.Logger
var otelEnabled bool

// Init initializes the logger (legacy mode - stdout only)
func Init() {
	InitWithOTel(false)
}

// InitWithOTel initializes the logger with optional OTel support
func InitWithOTel(enableOTel bool) {
	otelEnabled = enableOTel
	level := parseLevel(os.Getenv("LOG_LEVEL"))

	var handler slog.Handler
	if enableOTel {
		handler = NewMultiHandler(level)
	} else {
		jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})
		// Wrap with TraceContextHandler even without OTel for trace_id/span_id in stdout
		handler = NewTraceContextHandler(jsonHandler)
	}

	Logger = slog.New(handler)

	// Initialize ContextLogger for ADR 98/99 business context support
	GlobalContext = NewContextLogger(Logger)

	Logger.Info("Logger initialized", "otel_enabled", enableOTel)
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

// MultiHandler sends logs to multiple handlers
type MultiHandler struct {
	handlers []slog.Handler
}

// NewMultiHandler creates a handler that writes to both stdout and OTel
// Uses the official otelslog bridge for proper trace context propagation
func NewMultiHandler(level slog.Level) *MultiHandler {
	// Wrap JSONHandler with TraceContextHandler to add trace_id/span_id to stdout logs
	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})

	// Use official otelslog bridge for OTel export
	// This properly propagates trace context from the Go context
	otelHandler := otelslog.NewHandler(
		"search-indexer",
		otelslog.WithLoggerProvider(global.GetLoggerProvider()),
	)

	return &MultiHandler{
		handlers: []slog.Handler{
			NewTraceContextHandler(jsonHandler),
			otelHandler,
		},
	}
}

func (h *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *MultiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, r.Level) {
			_ = handler.Handle(ctx, r)
		}
	}
	return nil
}

func (h *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithAttrs(attrs)
	}
	return &MultiHandler{handlers: newHandlers}
}

func (h *MultiHandler) WithGroup(name string) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithGroup(name)
	}
	return &MultiHandler{handlers: newHandlers}
}
