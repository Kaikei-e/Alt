package bootstrap

import (
	"io"
	"log/slog"
	"testing"

	"pre-processor/handler"
)

// newTestHTTPServer returns a minimal Dependencies bundle sufficient for
// ingress-layer tests (body limit, CORS). The unused secret argument is
// retained so older test call sites compile unchanged; authentication is
// now transport-layer only.
func newTestHTTPServer(t *testing.T, _ string) *Dependencies {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &Dependencies{
		SummarizeHandler: handler.NewSummarizeHandler(nil, nil, nil, nil, logger),
		Logger:           logger,
	}
}
