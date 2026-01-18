package logger

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel/log/global"
)

// MultiHandler sends logs to multiple handlers
type MultiHandler struct {
	handlers []slog.Handler
}

// NewMultiHandler creates a handler that writes to both stdout and OTel
// Uses the official otelslog bridge for proper trace context propagation
func NewMultiHandler(level slog.Level) *MultiHandler {
	// Wrap JSONHandler with TraceContextHandler to add trace_id/span_id to stdout logs
	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     level,
		AddSource: false,
	})

	// Use official otelslog bridge for OTel export
	// This properly propagates trace context from the Go context
	otelHandler := otelslog.NewHandler(
		"alt-backend",
		otelslog.WithLoggerProvider(global.GetLoggerProvider()),
	)

	return &MultiHandler{
		handlers: []slog.Handler{
			// JSON output to stdout with trace context (for Docker log driver / rask-log-forwarder)
			NewTraceContextHandler(jsonHandler),
			// OTel export via official bridge (properly handles trace context)
			otelHandler,
		},
	}
}

// NewMultiHandlerStdoutOnly creates a handler that writes only to stdout (OTel disabled)
func NewMultiHandlerStdoutOnly(level slog.Level) *MultiHandler {
	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     level,
		AddSource: false,
	})

	return &MultiHandler{
		handlers: []slog.Handler{
			// JSON output to stdout with trace context
			NewTraceContextHandler(jsonHandler),
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
