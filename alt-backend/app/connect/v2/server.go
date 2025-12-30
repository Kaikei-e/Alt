// Package v2 provides Connect-RPC server setup and configuration.
package v2

import (
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"alt/gen/proto/alt/feeds/v2/feedsv2connect"

	"alt/config"
	"alt/connect/v2/feeds"
	"alt/connect/v2/middleware"
	"alt/di"
)

// SetupConnectHandlers registers all Connect-RPC handlers with the HTTP mux.
func SetupConnectHandlers(mux *http.ServeMux, container *di.ApplicationComponents, cfg *config.Config, logger *slog.Logger) {
	// Create auth interceptor
	authInterceptor := middleware.NewAuthInterceptor(logger, cfg)

	// Handler options with auth interceptor
	opts := connect.WithInterceptors(authInterceptor.Interceptor())

	// Register Feed service
	feedHandler := feeds.NewHandler(container, logger)
	path, handler := feedsv2connect.NewFeedServiceHandler(feedHandler, opts)
	mux.Handle(path, handler)

	logger.Info("Registered Connect-RPC FeedService", "path", path)
}

// CreateConnectServer creates the Connect-RPC server with HTTP/2 support.
func CreateConnectServer(container *di.ApplicationComponents, cfg *config.Config, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	// Add health check endpoint for Connect-RPC server
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy","service":"connect-rpc"}`))
	})

	SetupConnectHandlers(mux, container, cfg, logger)

	// Support HTTP/2 without TLS (h2c) for local development and internal communication
	return h2c.NewHandler(mux, &http2.Server{})
}
