package logger

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/log/global"
)

var Logger *slog.Logger

// New creates a basic JSON logger (legacy mode - stdout only)
func New() *slog.Logger {
	return NewWithOTel(false)
}

// NewWithOTel creates a logger with optional OTel support
func NewWithOTel(enableOTel bool) *slog.Logger {
	level := parseLevel(os.Getenv("LOG_LEVEL"))

	var handler slog.Handler
	if enableOTel {
		handler = NewMultiHandler(level)
	} else {
		jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})
		handler = NewTraceContextHandler(jsonHandler)
	}

	Logger = slog.New(handler)
	Logger.Info("Logger initialized", "otel_enabled", enableOTel)
	return Logger
}

// MultiHandler sends logs to multiple handlers
type MultiHandler struct {
	handlers []slog.Handler
}

// NewMultiHandler creates a handler that writes to both stdout and OTel
// Uses the official otelslog bridge for proper trace context propagation
func NewMultiHandler(level slog.Level) *MultiHandler {
	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	// Wrap jsonHandler with TraceContextHandler to include trace_id/span_id in stdout logs
	stdoutHandler := NewTraceContextHandler(jsonHandler)

	// Use official otelslog bridge for OTel export
	// This properly propagates trace context from the Go context
	otelHandler := otelslog.NewHandler(
		"rag-orchestrator",
		otelslog.WithLoggerProvider(global.GetLoggerProvider()),
	)

	return &MultiHandler{
		handlers: []slog.Handler{
			stdoutHandler,
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
