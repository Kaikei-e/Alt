package logger

import (
	"context"
	"log/slog"
	"testing"
)

func TestMultiHandler_Enabled(t *testing.T) {
	h := NewMultiHandlerStdoutOnly(slog.LevelInfo)

	if !h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Expected INFO level to be enabled")
	}

	if h.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Expected DEBUG level to be disabled")
	}
}

func TestMultiHandler_WithAttrs(t *testing.T) {
	h := NewMultiHandlerStdoutOnly(slog.LevelInfo)
	attrs := []slog.Attr{slog.String("key", "value")}

	newHandler := h.WithAttrs(attrs)

	// Verify it returns a new handler (not the same instance)
	if newHandler == h {
		t.Error("WithAttrs should return a new handler instance")
	}

	// Verify it's still a MultiHandler
	if _, ok := newHandler.(*MultiHandler); !ok {
		t.Error("WithAttrs should return a MultiHandler")
	}
}

func TestMultiHandler_WithGroup(t *testing.T) {
	h := NewMultiHandlerStdoutOnly(slog.LevelInfo)

	newHandler := h.WithGroup("testgroup")

	if newHandler == h {
		t.Error("WithGroup should return a new handler instance")
	}

	if _, ok := newHandler.(*MultiHandler); !ok {
		t.Error("WithGroup should return a MultiHandler")
	}
}

func TestNewMultiHandler_WithOTel(t *testing.T) {
	// Test that NewMultiHandler creates a handler with 2 handlers (stdout + otelslog)
	h := NewMultiHandler(slog.LevelInfo)

	if len(h.handlers) != 2 {
		t.Errorf("Expected 2 handlers, got %d", len(h.handlers))
	}

	// Test that it's enabled for INFO level
	if !h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Expected INFO level to be enabled")
	}
}
