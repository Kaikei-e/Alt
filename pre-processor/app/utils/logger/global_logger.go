package logger

import (
	"log/slog"
	"os"
)

// init sets up a no-op logger for tests to avoid nil-pointer panics when
// the application code uses logger.Logger before the main package configures
// it. Production code still overrides this value in main.go.
func init() {
	if Logger == nil {
		// Minimal text handler that writes to stderr; level=INFO by default.
		Logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{}))
	}
}
