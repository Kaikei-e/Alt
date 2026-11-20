package middleware

import (
	"log/slog"
	"net/http"

	"search-indexer/logger"
)

func init() {
	// tests import the package directly without calling logger.Init() — wire a
	// discard slog so middleware that logs via logger.Logger never panics.
	if logger.Logger == nil {
		logger.Logger = slog.New(slog.DiscardHandler)
	}
}

func newTestHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}
