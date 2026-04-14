package bootstrap

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	"search-indexer/logger"
)

// captureLogs swaps logger.Logger with one that writes JSON to buf for the
// duration of the test. It restores the previous logger on cleanup.
func captureLogs(t *testing.T, level slog.Level) *bytes.Buffer {
	t.Helper()
	prev := logger.Logger
	t.Cleanup(func() { logger.Logger = prev })

	buf := &bytes.Buffer{}
	h := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: level})
	logger.Logger = slog.New(h)
	return buf
}

func TestLogPanic_DoesNotLeakPanicValueAtInfoLevel(t *testing.T) {
	t.Parallel()
	buf := captureLogs(t, slog.LevelInfo)

	// Panic value contains what looks like a secret to simulate accidental
	// inclusion via wrapped errors.
	logPanic(context.Background(), "index loop panic", "leaked-service-token-abc123")

	out := buf.String()
	if !strings.Contains(out, "index loop panic") {
		t.Fatalf("expected error log to mention the panic site, got: %s", out)
	}
	if strings.Contains(out, "leaked-service-token-abc123") {
		t.Fatalf("panic value leaked into info-level log: %s", out)
	}
	if !strings.Contains(out, `"err_type"`) {
		t.Fatalf("expected err_type field in log, got: %s", out)
	}
}

func TestLogPanic_IncludesDetailAtDebugLevel(t *testing.T) {
	t.Parallel()
	buf := captureLogs(t, slog.LevelDebug)

	logPanic(context.Background(), "index loop panic", "debug-detail-marker")

	out := buf.String()
	if !strings.Contains(out, "debug-detail-marker") {
		t.Fatalf("expected panic detail at debug level, got: %s", out)
	}
	if !strings.Contains(out, `"stack"`) {
		t.Fatalf("expected stack field at debug level, got: %s", out)
	}
}

func TestMain(m *testing.M) {
	logger.Init()
	os.Exit(m.Run())
}
