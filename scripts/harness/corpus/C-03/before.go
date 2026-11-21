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
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	handleUpstreamFailure(logger)

	if !strings.Contains(buf.String(), "upstream failed") {
		t.Fatalf("expected error log entry, got %q", buf.String())
	}
}
