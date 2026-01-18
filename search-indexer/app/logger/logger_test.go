package logger

import (
	"context"
	"log/slog"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo},
		{"", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseLevel(tt.input)
			if got != tt.expected {
				t.Errorf("parseLevel(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestMultiHandler_Enabled(t *testing.T) {
	h := NewMultiHandler(slog.LevelInfo)

	if h.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("expected DEBUG to be disabled")
	}
	if !h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("expected INFO to be enabled")
	}
}

func TestMultiHandler_WithAttrs(t *testing.T) {
	h := NewMultiHandler(slog.LevelInfo)
	attrs := []slog.Attr{slog.String("key", "value")}

	newHandler := h.WithAttrs(attrs)

	if newHandler == h {
		t.Error("WithAttrs should return a new handler instance")
	}

	if _, ok := newHandler.(*MultiHandler); !ok {
		t.Error("WithAttrs should return a MultiHandler")
	}
}

func TestMultiHandler_WithGroup(t *testing.T) {
	h := NewMultiHandler(slog.LevelInfo)

	newHandler := h.WithGroup("testgroup")

	if newHandler == h {
		t.Error("WithGroup should return a new handler instance")
	}

	if _, ok := newHandler.(*MultiHandler); !ok {
		t.Error("WithGroup should return a MultiHandler")
	}
}
