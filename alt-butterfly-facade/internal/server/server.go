// Package server provides the HTTP server setup for the BFF service.
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"alt-butterfly-facade/internal/client"
	"alt-butterfly-facade/internal/handler"
	"alt-butterfly-facade/internal/middleware"
)

// HealthResponse represents the health check response.
type HealthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

// Config holds server configuration.
type Config struct {
	BackendURL string
	// BackendInternalURL is alt-backend's internal listener, which carries the
	// admin Connect-RPC services. It is a separate field from BackendURL so
	// the admin proxies cannot silently degrade to the browser-facing port.
	BackendInternalURL string
	BackendRESTURL     string
	Secret             []byte
	Issuer             string
	Audience           string
	RequestTimeout     time.Duration
	StreamingTimeout   time.Duration
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
		createQueryFetcher(backendClient, cfg.RequestTimeout),
		cfg.Secret,
		cfg.Issuer,
		cfg.Audience,
		logger,
	)

	// Auth for internal-info endpoints (e.g. /v1/bff/stats) that are not
	// backend proxies and so don't go through a *ProxyHandler's authInterceptor.
	statsAuth := middleware.NewAuthInterceptor(logger, cfg.Secret, cfg.Issuer, cfg.Audience)

	// Register health endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(HealthResponse{
			Status:  "healthy",
			Service: "alt-butterfly-facade",
		})
	})

	// Register stats endpoint for monitoring BFF features. Exposes cache /
	// circuit-breaker internals, so it requires an authenticated admin token
	// like the other admin-only routes below.
	mux.HandleFunc("/v1/bff/stats", func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get(middleware.BackendTokenHeader)
		userCtx, err := statsAuth.ValidateToken(token)
		if err != nil || userCtx.Role != "admin" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		stats := BFFStats{}
		if bffHandler != nil {
			if cacheStats := bffHandler.GetCacheStats(); cacheStats != nil {
				stats.Cache = &CacheStatsResponse{
					Hits:   cacheStats.Hits,
					Misses: cacheStats.Misses,
					Size:   cacheStats.Size,
				}
			}
			if classStats := bffHandler.GetCircuitBreakerClassStats(); classStats != nil {
				stats.CircuitBreaker = &CircuitBreakerStatsResponse{}
				if rollup := bffHandler.GetCircuitBreakerStats(); rollup != nil {
					stats.CircuitBreaker.State = rollup.State.String()
					stats.CircuitBreaker.TotalSuccesses = rollup.TotalSuccesses
					stats.CircuitBreaker.TotalFailures = rollup.TotalFailures
					stats.CircuitBreaker.TotalRejections = rollup.TotalRejections
				}
				if classStats.Mutation != nil {
					stats.CircuitBreaker.Mutation = &CircuitBreakerClassSlice{
						State:           classStats.Mutation.State.String(),
						TotalSuccesses:  classStats.Mutation.TotalSuccesses,
						TotalFailures:   classStats.Mutation.TotalFailures,
						TotalRejections: classStats.Mutation.TotalRejections,
					}
				}
				if classStats.Projection != nil {
					stats.CircuitBreaker.Projection = &CircuitBreakerClassSlice{
						State:           classStats.Projection.State.String(),
						TotalSuccesses:  classStats.Projection.TotalSuccesses,
						TotalFailures:   classStats.Projection.TotalFailures,
						TotalRejections: classStats.Projection.TotalRejections,
					}
				}
				if classStats.NonCritical != nil {
					stats.CircuitBreaker.NonCritical = &CircuitBreakerClassSlice{
						State:           classStats.NonCritical.State.String(),
						TotalSuccesses:  classStats.NonCritical.TotalSuccesses,
						TotalFailures:   classStats.NonCritical.TotalFailures,
						TotalRejections: classStats.NonCritical.TotalRejections,
					}
				}
				if classStats.ExternalContent != nil {
					stats.CircuitBreaker.ExternalContent = &CircuitBreakerClassSlice{
						State:           classStats.ExternalContent.State.String(),
						TotalSuccesses:  classStats.ExternalContent.TotalSuccesses,
						TotalFailures:   classStats.ExternalContent.TotalFailures,
						TotalRejections: classStats.ExternalContent.TotalRejections,
					}
				}
				if classStats.Telemetry != nil {
					stats.CircuitBreaker.Telemetry = &TelemetryStatsSlice{
						TotalSuccesses: classStats.Telemetry.TotalSuccesses,
						TotalFailures:  classStats.Telemetry.TotalFailures,
					}
				}
			} else if cbStats := bffHandler.GetCircuitBreakerStats(); cbStats != nil {
				stats.CircuitBreaker = &CircuitBreakerStatsResponse{
					State:           cbStats.State.String(),
					TotalSuccesses:  cbStats.TotalSuccesses,
					TotalFailures:   cbStats.TotalFailures,
					TotalRejections: cbStats.TotalRejections,
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
	// The plaintext REST surface is closed (ADR-000729 Phase 3): only the
	// paths on the architectural allowlist reach the upstream Echo listener,
	// everything else is refused here.
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
		// Only allowlisted /v1/* paths reach the plaintext Echo listener. The
		// allowlist is the record of what deliberately stayed on REST after
		// the Connect-RPC migration, so anything outside it is either already
		// migrated or was never browser-facing — both are 404 here rather than
		// a proxied request the BFF has no architectural reason to make.
		mux.Handle("/v1/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !allowRESTPath(r.URL.Path) {
				if logger != nil {
					logger.WarnContext(r.Context(), "BFF rejected plaintext REST path outside allowlist",
						"path", r.URL.Path,
						"hint", "migrate to Connect-RPC via /api/v2, or add to restAllowlistPrefixes / restAllowlistPatterns",
					)
				}
				http.NotFound(w, r)
				return
			}
			restProxy.ServeHTTP(w, r)
		}))
	}

	// Knowledge Home admin routing (before catch-all).
	// The admin-role check on the caller's JWT happens here, at the BFF
	// boundary — alt-backend's admin services do not repeat it. They live on
	// its internal listener, so these proxies get their own client pointed at
	// BackendInternalURL rather than the browser-facing Connect port.
	{
		adminBackendClient := backendClient
		if cfg.BackendInternalURL != "" {
			adminBackendClient = client.NewBackendClientWithTransport(
				cfg.BackendInternalURL,
				cfg.RequestTimeout,
				cfg.StreamingTimeout,
				connectTransport,
			)
		}
		adminProxy := handler.NewAdminProxyHandler(
			adminBackendClient,
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
			adminBackendClient,
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
		if acolyteTransport == nil || strings.HasPrefix(cfg.AcolyteConnectURL, "http://") {
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

	// Register proxy handler for all other paths.
	// Connect-RPC uses paths like /alt.feeds.v2.FeedService/GetFeedStats.
	//
	// The BFF is a legitimate mTLS peer of its upstreams, and the east-west
	// RPC surface has no user-JWT interceptor on the far side. Forwarding a
	// caller-chosen path here would therefore make the BFF a confused deputy
	// for any account that can obtain a session. The invariant is unchanged —
	// a caller-chosen path is never forwarded — but it is now enforced
	// positively: only services present in the generated allowlist
	// (allowlist_gen.go, derived by protovis from the (alt.api.v1.visibility)
	// option on each proto service) get through. Absence denies by
	// construction, so a new east-west service, or a public service that was
	// never published, needs no hand-maintained deny prefix to be refused.
	//
	// The dedicated routes above (KnowledgeHomeAdminService,
	// AdminMonitorService, Acolyte, /v1/…) are longer mux patterns and resolve
	// before this one; they are unaffected.
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowConnectPath(r.URL.Path) {
			if logger != nil {
				logger.WarnContext(r.Context(), "BFF rejected Connect-RPC path outside the generated allowlist",
					"path", r.URL.Path,
				)
			}
			http.NotFound(w, r)
			return
		}
		mainHandler.ServeHTTP(w, r)
	}))

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
	State           string                    `json:"state"`
	TotalSuccesses  int64                     `json:"total_successes"`
	TotalFailures   int64                     `json:"total_failures"`
	TotalRejections int64                     `json:"total_rejections"`
	Mutation        *CircuitBreakerClassSlice `json:"mutation,omitempty"`
	Projection      *CircuitBreakerClassSlice `json:"projection,omitempty"`
	NonCritical     *CircuitBreakerClassSlice `json:"non_critical,omitempty"`
	ExternalContent *CircuitBreakerClassSlice `json:"external_content,omitempty"`
	// Telemetry is ClassTelemetry's outcome counters. Those endpoints are
	// exempt from breaker gating (fire-and-forget writes), so there is no
	// state/rejections to report, only whether they are succeeding.
	Telemetry *TelemetryStatsSlice `json:"telemetry,omitempty"`
}

// CircuitBreakerClassSlice is per-dependency-class CB stats.
type CircuitBreakerClassSlice struct {
	State          string `json:"state"`
	TotalSuccesses int64  `json:"total_successes"`
	TotalFailures  int64  `json:"total_failures"`
	// TotalRejections is the requests this class refused while open — the
	// user-visible 503 volume that the transition log deliberately does not
	// emit a line for.
	TotalRejections int64 `json:"total_rejections"`
}

// TelemetryStatsSlice is ClassTelemetry's outcome counters in the API
// response.
type TelemetryStatsSlice struct {
	TotalSuccesses int64 `json:"total_successes"`
	TotalFailures  int64 `json:"total_failures"`
}

// createQueryFetcher creates a query fetcher function for the aggregation handler.
//
// requestTimeout bounds each fan-out call. AggregationHandler blocks on a
// WaitGroup until every query goroutine returns, and the backend client runs
// with http.Client.Timeout unset, so a request built on a deadline-less
// context strands N goroutines per aggregation whenever alt-backend stops
// answering. QueryFetcher takes no context, so the inbound request's
// cancellation cannot reach here yet — see the note on the deadline below.
func createQueryFetcher(backendClient *client.BackendClient, requestTimeout time.Duration) handler.QueryFetcher {
	return func(path string, token string, body []byte) (*handler.AggregatedResult, error) {
		// Deadline only, not inbound cancellation: widening QueryFetcher to
		// carry the caller's context is the remaining half of this fix.
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()

		// Create a mock request to forward
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, path, bytes.NewReader(body))
		if err != nil {
			return &handler.AggregatedResult{
				Error:      err.Error(),
				StatusCode: http.StatusInternalServerError,
			}, nil
		}
		req.URL.Path = path
		req.Header.Set("Content-Type", "application/json")

		resp, err := backendClient.ForwardRequest(req, token)
		if err != nil {
			return &handler.AggregatedResult{
				Error:      err.Error(),
				StatusCode: http.StatusBadGateway,
			}, nil
		}
		defer resp.Body.Close()

		// A body that ends mid-stream is a failed fetch whatever its status
		// line said, and the deadline above is now the ordinary way that
		// happens: a backend that answers headers promptly and then stalls has
		// its read cut off here, not at ForwardRequest. Swallowing the error
		// would report the upstream's 200 with a truncated JSON prefix in Data
		// — the same defect already fixed in BFFHandler.forwardToBackend.
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return &handler.AggregatedResult{
				Error:      err.Error(),
				StatusCode: http.StatusBadGateway,
			}, nil
		}

		return &handler.AggregatedResult{
			Data:       respBody,
			StatusCode: resp.StatusCode,
		}, nil
	}
}
