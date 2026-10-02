package bootstrap

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"search-indexer/config"
	connectv2 "search-indexer/connect/v2"
	"search-indexer/healthdeep"
	"search-indexer/middleware"
	"search-indexer/rest"
	"search-indexer/usecase"
	appOtel "search-indexer/utils/otel"
)

// newHTTPServer creates the plaintext health-only HTTP server on :9300.
// Per C01, business search endpoints are retired from plaintext and served
// exclusively over mTLS on :9443. Plaintext :9300 serves liveness and deep health only.
func newHTTPServer(searchByUserUsecase *usecase.SearchByUserUsecase, otelCfg appOtel.Config, rlCfg config.RateLimitConfig, meiliPing func(context.Context) error) *http.Server {
	mux := http.NewServeMux()

	healthHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	})

	deep := healthdeep.NewRunner(healthdeep.Config{
		Service: "search-indexer",
		Checks: []healthdeep.Check{{
			Name:     "meilisearch",
			Critical: true,
			Probe:    meiliPing,
		}},
	})
	deepHandler := deep.Handler()

	if otelCfg.Enabled {
		mux.Handle("/health", middleware.OTelStatusHandlerFunc(healthHandler, "GET /health"))
		mux.Handle("/health/deep", middleware.OTelStatusHandler(deepHandler, "GET /health/deep"))
	} else {
		mux.Handle("/health", healthHandler)
		mux.Handle("/health/deep", deepHandler)
	}

	return &http.Server{
		Addr:              config.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}

// newConnectServer creates the Connect-RPC server handler.
// Note: per C01, the standalone plaintext :9301 listener is retired.
func newConnectServer(searchByUserUsecase *usecase.SearchByUserUsecase, searchRecapsUsecase *usecase.SearchRecapsUsecase, authUsecase *usecase.AuthUsecase, rlCfg config.RateLimitConfig) *http.Server {
	handler := connectv2.CreateConnectServer(searchByUserUsecase, searchRecapsUsecase, authUsecase, rlCfg)

	return &http.Server{
		Addr:              config.ConnectAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}

// newMTLSMuxHandler builds the combined handler served on the :9443 mTLS
// listener: REST under /v1/* + Connect-RPC under /services.* + /health + /health/deep.
// All REST and Connect business endpoints are gated by peer_identity (TLS client-cert
// CN allowlist). Health endpoints stay reachable unauthenticated.
// Verified callers exercise deliberate service delegation on behalf of caller-specified user_id.
func newMTLSMuxHandler(
	authUsecase *usecase.AuthUsecase,
	searchByUserUsecase *usecase.SearchByUserUsecase,
	connectServerHandler http.Handler,
	otelCfg appOtel.Config,
	rlCfg config.RateLimitConfig,
	meiliPing ...func(context.Context) error,
) http.Handler {
	restHandler := rest.NewHandler(searchByUserUsecase, authUsecase)

	allowed := parseAllowedPeers(os.Getenv("MTLS_ALLOWED_PEERS"))
	peer := middleware.NewPeerIdentityMiddleware(allowed)
	rateLimiter := middleware.NewRateLimiter(rate.Limit(rlCfg.RequestsPerSecond), rlCfg.Burst)

	// REST /v1/search guarded by peer identity + rate limit.
	search := rateLimiter.Middleware(peer.Require(http.HandlerFunc(restHandler.SearchArticles)))
	// Connect-RPC is also gated by peer identity at the mux layer.
	connect := peer.Require(connectServerHandler)

	mux := http.NewServeMux()

	health := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	})

	var pingFn func(context.Context) error
	if len(meiliPing) > 0 && meiliPing[0] != nil {
		pingFn = meiliPing[0]
	}
	var checks []healthdeep.Check
	if pingFn != nil {
		checks = append(checks, healthdeep.Check{
			Name:     "meilisearch",
			Critical: true,
			Probe:    pingFn,
		})
	}
	deep := healthdeep.NewRunner(healthdeep.Config{
		Service: "search-indexer",
		Checks:  checks,
	})
	deepHandler := deep.Handler()

	if otelCfg.Enabled {
		mux.Handle("/v1/search", middleware.OTelStatusHandler(search, "GET /v1/search"))
		mux.Handle("/health", middleware.OTelStatusHandlerFunc(health, "GET /health"))
		mux.Handle("/health/deep", middleware.OTelStatusHandler(deepHandler, "GET /health/deep"))
	} else {
		mux.Handle("/v1/search", search)
		mux.Handle("/health", health)
		mux.Handle("/health/deep", deepHandler)
	}
	// Connect-RPC service paths: /services.search.v2.SearchService/*
	mux.Handle("/services.search.v2.SearchService/", connect)
	// Fallback for any other Connect-RPC-style prefix.
	mux.Handle("/", connect)

	return mux
}

func parseAllowedPeers(csv string) []string {
	trimmed := strings.TrimSpace(csv)
	if trimmed == "" {
		return []string{"alt-backend", "rag-orchestrator", "acolyte-orchestrator"}
	}
	parts := strings.Split(trimmed, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
