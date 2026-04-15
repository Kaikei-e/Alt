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
	BackendRESTURL   string
	Secret           []byte
	Issuer           string
	Audience         string
	RequestTimeout   time.Duration
	StreamingTimeout time.Duration
	TTSConnectURL      string
	AcolyteConnectURL  string

	// BFF Feature Configuration
	BFFConfig handler.BFFConfig
}

// NewServer creates a new HTTP server with the proxy handler.
func NewServer(cfg Config, logger *slog.Logger) http.Handler {
	return NewServerWithTransport(cfg, logger, nil)
}

// NewServerWithTransport creates a new HTTP server with a custom transport.
// If transport is nil, uses HTTP/2 h2c transport for production.
// REST proxies reuse the same transport.
func NewServerWithTransport(cfg Config, logger *slog.Logger, transport http.RoundTripper) http.Handler {
	return NewServerWithTransports(cfg, logger, transport, transport)
}

// NewServerWithTransports is like NewServerWithTransport but allows callers to
// supply a separate transport for REST proxy routes. This is the seam used by
// the mTLS rollout: Connect-RPC targets the mTLS listener on alt-backend while
// REST endpoints continue talking plaintext to the Echo listener.
func NewServerWithTransports(
	cfg Config,
	logger *slog.Logger,
	connectTransport http.RoundTripper,
	restTransport http.RoundTripper,
) http.Handler {
	mux := http.NewServeMux()

	// Create backend client (Connect-RPC over configured transport)
	backendClient := client.NewBackendClientWithTransport(
		cfg.BackendURL,
		cfg.RequestTimeout,
		cfg.StreamingTimeout,
		connectTransport,
	)
	// Shadow the original parameter name so downstream constructors that
	// previously read `transport` continue to work for Connect-RPC callers.
	transport := connectTransport
	_ = transport

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
			cfg.RequestTimeout,
			cfg.StreamingTimeout,
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

	// REST API proxy routing (before catch-all). Uses HTTP/1.1 transport
	// because alt-backend REST API does not use h2c. The REST transport is
	// kept separate from the Connect-RPC transport so mTLS rollouts can
	// target only the Connect-RPC path without breaking plaintext REST
	// proxies (see ADR-000727 / ADR-000729).
	//
	// Only allowlisted paths (OPML, dashboard, image proxy, admin scraping,
	// csrf, health) reach the upstream Echo listener. Every other /v1/*
	// request returns 404 so accidental reintroduction of REST endpoints
	// for user-facing features surfaces immediately.
	if cfg.BackendRESTURL != "" {
		effectiveRESTTransport := restTransport
		if effectiveRESTTransport == nil {
			effectiveRESTTransport = http.DefaultTransport
		}
		restClient := client.NewBackendClientWithTransport(
			cfg.BackendRESTURL,
			cfg.RequestTimeout,
			cfg.StreamingTimeout,
			effectiveRESTTransport,
		)
		restProxy := handler.NewRESTProxyHandler(
			restClient, cfg.Secret, cfg.Issuer, cfg.Audience, logger, cfg.RequestTimeout,
		)
		// The BFF still forwards every /v1/* path, but logs a warning when
		// a caller hits a prefix that is not on the architectural allowlist.
		// This keeps user-facing traffic alive (numerous SSR helpers still
		// speak REST) while surfacing every migration candidate for future
		// Connect-RPC conversion — see ADR-000729.
		mux.Handle("/v1/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !allowRESTPath(r.URL.Path) && logger != nil {
				logger.WarnContext(r.Context(), "BFF plaintext REST path outside allowlist",
					"path", r.URL.Path,
					"hint", "migrate to Connect-RPC via /api/v2 or add to restAllowlistPrefixes",
				)
			}
			restProxy.ServeHTTP(w, r)
		}))
	}

	// TTS service routing (before catch-all).
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
			ttsClient, cfg.Secret, cfg.Issuer, cfg.Audience, logger, cfg.StreamingTimeout, cfg.StreamingTimeout,
		)
		// Auth to tts-speaker is established at the TLS transport layer (mTLS).
		mux.Handle("/alt.tts.v1.TTSService/", ttsProxy)
	}

	// Knowledge Home admin routing (before catch-all).
	// Requests are authenticated as admin users at the BFF boundary; service
	// auth is established at the TLS transport layer (mTLS).
	{
		adminProxy := handler.NewAdminProxyHandler(
			backendClient,
			cfg.Secret,
			cfg.Issuer,
			cfg.Audience,
			"",
			logger,
			cfg.RequestTimeout,
		)
		mux.Handle("/alt.knowledge_home.v1.KnowledgeHomeAdminService/", adminProxy)

		// Admin observability (Prometheus-backed). Uses a streaming-aware
		// proxy that does not apply a short request timeout: Watch is
		// a long-lived server stream.
		adminMonitorProxy := handler.NewAdminMonitorProxyHandler(
			backendClient,
			cfg.Secret,
			cfg.Issuer,
			cfg.Audience,
			"",
			logger,
		)
		mux.Handle("/alt.admin_monitor.v1.AdminMonitorService/", adminMonitorProxy)
	}

	// Acolyte orchestrator routing (Connect protocol, HTTP/1.1)
	// Uses streaming timeout since report generation can be long-running.
	if cfg.AcolyteConnectURL != "" {
		acolyteTransport := transport
		if acolyteTransport == nil {
			acolyteTransport = http.DefaultTransport
		}
		acolyteClient := client.NewBackendClientWithTransport(
			cfg.AcolyteConnectURL,
			cfg.StreamingTimeout,
			cfg.StreamingTimeout,
			acolyteTransport,
		)
		acolyteProxy := handler.NewProxyHandler(
			acolyteClient,
			cfg.Secret,
			cfg.Issuer,
			cfg.Audience,
			logger,
			cfg.StreamingTimeout,
			cfg.StreamingTimeout,
		)
		// Auth to acolyte is established at the TLS transport layer (mTLS).
		mux.Handle("/alt.acolyte.v1.AcolyteService/", acolyteProxy)
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
