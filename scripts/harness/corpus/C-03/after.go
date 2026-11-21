package middleware

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func handleUpstreamFailure(logger *slog.Logger) {
	logger.Error("upstream failed", "status", 502)
}

func TestHandleUpstreamFailure_LogsError(t *testing.T) {
	var buf bytes.Buffer
	// BUG: sloglint autofix replaced capturing test logger with DiscardHandler.
	logger := slog.New(slog.DiscardHandler)

	handleUpstreamFailure(logger)

	if !strings.Contains(buf.String(), "upstream failed") {
		t.Fatalf("expected error log entry, got %q", buf.String())
	}
}
