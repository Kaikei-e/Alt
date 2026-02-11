// Package server provides the HTTP server setup for the BFF service.
package server

import (
	"encoding/json"
	"io"
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
	TTSConnectURL    string
	TTSServiceSecret string

	// BFF Feature Configuration
	BFFConfig handler.BFFConfig
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

	// Determine which handler to use based on BFF features
	var mainHandler http.Handler
	var bffHandler *handler.BFFHandler

	if cfg.BFFConfig.EnableCache || cfg.BFFConfig.EnableCircuitBreaker ||
		cfg.BFFConfig.EnableDedup || cfg.BFFConfig.EnableErrorNormalization {
		// Use BFF handler with features enabled
		bffHandler = handler.NewBFFHandler(
			backendClient,
			cfg.Secret,
			cfg.Issuer,
			cfg.Audience,
			logger,
			cfg.BFFConfig,
		)
		mainHandler = bffHandler
	} else {
		// Use simple proxy handler (legacy behavior)
		mainHandler = handler.NewProxyHandler(
			backendClient,
			cfg.Secret,
			cfg.Issuer,
			cfg.Audience,
			logger,
		)
	}

	// Create aggregation handler
	aggregationHandler := handler.NewAggregationHandler(
		createQueryFetcher(backendClient),
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

	// Register stats endpoint for monitoring BFF features
	mux.HandleFunc("/v1/bff/stats", func(w http.ResponseWriter, r *http.Request) {
		stats := BFFStats{}
		if bffHandler != nil {
			if cacheStats := bffHandler.GetCacheStats(); cacheStats != nil {
				stats.Cache = &CacheStatsResponse{
					Hits:   cacheStats.Hits,
					Misses: cacheStats.Misses,
					Size:   cacheStats.Size,
				}
			}
			if cbStats := bffHandler.GetCircuitBreakerStats(); cbStats != nil {
				stats.CircuitBreaker = &CircuitBreakerStatsResponse{
					State:          cbStats.State.String(),
					TotalSuccesses: cbStats.TotalSuccesses,
					TotalFailures:  cbStats.TotalFailures,
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(stats)
	})

	// Register aggregation endpoint
	mux.Handle("/v1/aggregate", aggregationHandler)

	// TTS service routing (before catch-all)
	// Uses HTTP/1.1 transport because tts-speaker (uvicorn) does not support h2c.
	if cfg.TTSConnectURL != "" {
		ttsTransport := transport
		if ttsTransport == nil {
			ttsTransport = http.DefaultTransport
		}
		// Use streaming timeout for TTS since synthesis can be slow
		ttsClient := client.NewBackendClientWithTransport(
			cfg.TTSConnectURL,
			cfg.StreamingTimeout,
			cfg.StreamingTimeout,
			ttsTransport,
		)
		ttsProxy := handler.NewProxyHandler(
			ttsClient, cfg.Secret, cfg.Issuer, cfg.Audience, logger,
		)
		// Wrap to inject X-Service-Token for tts-speaker authentication
		ttsServiceSecret := cfg.TTSServiceSecret
		var ttsHandler http.Handler = ttsProxy
		if ttsServiceSecret != "" {
			ttsHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r.Header.Set("X-Service-Token", ttsServiceSecret)
				ttsProxy.ServeHTTP(w, r)
			})
		}
		mux.Handle("/alt.tts.v1.TTSService/", ttsHandler)
	}

	// Register proxy handler for all other paths
	// Connect-RPC uses paths like /alt.feeds.v2.FeedService/GetFeedStats
	mux.Handle("/", mainHandler)

	// Support HTTP/2 without TLS (h2c) for Connect-RPC streaming
	return h2c.NewHandler(mux, &http2.Server{})
}

// BFFStats holds statistics about BFF features.
type BFFStats struct {
	Cache          *CacheStatsResponse          `json:"cache,omitempty"`
	CircuitBreaker *CircuitBreakerStatsResponse `json:"circuit_breaker,omitempty"`
}

// CacheStatsResponse represents cache statistics in the API response.
type CacheStatsResponse struct {
	Hits   int64 `json:"hits"`
	Misses int64 `json:"misses"`
	Size   int   `json:"size"`
}

// CircuitBreakerStatsResponse represents circuit breaker statistics in the API response.
type CircuitBreakerStatsResponse struct {
	State          string `json:"state"`
	TotalSuccesses int64  `json:"total_successes"`
	TotalFailures  int64  `json:"total_failures"`
}

// createQueryFetcher creates a query fetcher function for the aggregation handler.
func createQueryFetcher(backendClient *client.BackendClient) handler.QueryFetcher {
	return func(path string, token string, body []byte) (*handler.AggregatedResult, error) {
		// Create a mock request to forward
		req, err := http.NewRequest(http.MethodPost, path, io.NopCloser(nil))
		if err != nil {
			return &handler.AggregatedResult{
				Error:      err.Error(),
				StatusCode: http.StatusInternalServerError,
			}, nil
		}
		req.URL.Path = path

		resp, err := backendClient.ForwardRequest(req, token)
		if err != nil {
			return &handler.AggregatedResult{
				Error:      err.Error(),
				StatusCode: http.StatusBadGateway,
			}, nil
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)

		return &handler.AggregatedResult{
			Data:       respBody,
			StatusCode: resp.StatusCode,
		}, nil
	}
}

