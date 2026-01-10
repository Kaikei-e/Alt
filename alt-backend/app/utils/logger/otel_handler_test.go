package logger

import (
	"context"
	"log/slog"
	"testing"
)

func TestOTelHandler_Enabled(t *testing.T) {
	tests := []struct {
		name          string
		handlerLevel  slog.Level
		checkLevel    slog.Level
		expectEnabled bool
	}{
		{"info handler, debug level", slog.LevelInfo, slog.LevelDebug, false},
		{"info handler, info level", slog.LevelInfo, slog.LevelInfo, true},
		{"info handler, warn level", slog.LevelInfo, slog.LevelWarn, true},
		{"info handler, error level", slog.LevelInfo, slog.LevelError, true},
		{"warn handler, info level", slog.LevelWarn, slog.LevelInfo, false},
		{"warn handler, warn level", slog.LevelWarn, slog.LevelWarn, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewOTelHandler(WithOTelLevel(tt.handlerLevel))
			if got := h.Enabled(context.Background(), tt.checkLevel); got != tt.expectEnabled {
				t.Errorf("Enabled() = %v, want %v", got, tt.expectEnabled)
			}
		})
	}
}

func TestSlogLevelToOTel(t *testing.T) {
	tests := []struct {
		name     string
		level    slog.Level
		expected string
	}{
		{"debug", slog.LevelDebug, "DEBUG"},
		{"info", slog.LevelInfo, "INFO"},
		{"warn", slog.LevelWarn, "WARN"},
		{"error", slog.LevelError, "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			severity := slogLevelToOTel(tt.level)
			if severity.String() != tt.expected {
				t.Errorf("slogLevelToOTel(%v) = %s, want %s", tt.level, severity.String(), tt.expected)
			}
		})
	}
}

func TestOTelHandler_WithAttrs(t *testing.T) {
	h := NewOTelHandler()
	attrs := []slog.Attr{slog.String("key", "value")}

	newHandler := h.WithAttrs(attrs)

	// Verify it returns a new handler (not the same instance)
	if newHandler == h {
		t.Error("WithAttrs should return a new handler instance")
	}

	// Verify it's still an OTelHandler
	if _, ok := newHandler.(*OTelHandler); !ok {
		t.Error("WithAttrs should return an OTelHandler")
	}
}

func TestOTelHandler_WithGroup(t *testing.T) {
	h := NewOTelHandler()

	newHandler := h.WithGroup("testgroup")

	if newHandler == h {
		t.Error("WithGroup should return a new handler instance")
	}

	if _, ok := newHandler.(*OTelHandler); !ok {
		t.Error("WithGroup should return an OTelHandler")
	}
}

func TestMultiHandler_Enabled(t *testing.T) {
	h := NewMultiHandlerStdoutOnly(slog.LevelInfo)

	if !h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Expected INFO level to be enabled")
	}

	if h.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Expected DEBUG level to be disabled")
	}
}
