// Package server provides the HTTP server setup for the BFF service.
package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"alt-butterfly-facade/internal/client"
	"alt-butterfly-facade/internal/handler"
)

// HealthResponse represents the health check response.
type HealthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

// Config holds server configuration.
type Config struct {
	BackendURL       string
	Secret           []byte
	Issuer           string
	Audience         string
	RequestTimeout   time.Duration
	StreamingTimeout time.Duration
}

// NewServer creates a new HTTP server with the proxy handler.
func NewServer(cfg Config, logger *slog.Logger) http.Handler {
	return NewServerWithTransport(cfg, logger, nil)
}

// NewServerWithTransport creates a new HTTP server with a custom transport.
// If transport is nil, uses HTTP/2 h2c transport for production.
func NewServerWithTransport(cfg Config, logger *slog.Logger, transport http.RoundTripper) http.Handler {
	mux := http.NewServeMux()

	// Create backend client
	backendClient := client.NewBackendClientWithTransport(
		cfg.BackendURL,
		cfg.RequestTimeout,
		cfg.StreamingTimeout,
		transport,
	)

	// Create proxy handler
	proxyHandler := handler.NewProxyHandler(
		backendClient,
		cfg.Secret,
		cfg.Issuer,
		cfg.Audience,
		logger,
	)

	// Register health endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(HealthResponse{
			Status:  "healthy",
			Service: "alt-butterfly-facade",
		})
	})

	// Register proxy handler for all other paths
	// Connect-RPC uses paths like /alt.feeds.v2.FeedService/GetFeedStats
	mux.Handle("/", proxyHandler)

	// Support HTTP/2 without TLS (h2c) for Connect-RPC streaming
	return h2c.NewHandler(mux, &http2.Server{})
}
