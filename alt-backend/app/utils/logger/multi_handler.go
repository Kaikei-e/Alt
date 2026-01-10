package logger

import (
	"context"
	"log/slog"
	"os"
)

// MultiHandler sends logs to multiple handlers
type MultiHandler struct {
	handlers []slog.Handler
}

// NewMultiHandler creates a handler that writes to both stdout and OTel
func NewMultiHandler(level slog.Level) *MultiHandler {
	return &MultiHandler{
		handlers: []slog.Handler{
			// JSON output to stdout (for Docker log driver / rask-log-forwarder)
			slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
				Level:     level,
				AddSource: false,
			}),
			// OTel export
			NewOTelHandler(WithOTelLevel(level)),
		},
	}
}

// NewMultiHandlerStdoutOnly creates a handler that writes only to stdout (OTel disabled)
func NewMultiHandlerStdoutOnly(level slog.Level) *MultiHandler {
	return &MultiHandler{
		handlers: []slog.Handler{
			slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
				Level:     level,
				AddSource: false,
			}),
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
