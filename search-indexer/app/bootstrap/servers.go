package bootstrap

import (
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/time/rate"

	connectv2 "search-indexer/connect/v2"
	"search-indexer/config"
	"search-indexer/middleware"
	"search-indexer/rest"
	"search-indexer/usecase"
	appOtel "search-indexer/utils/otel"
)

// newHTTPServer creates the REST HTTP server.
func newHTTPServer(searchByUserUsecase *usecase.SearchByUserUsecase, searchArticlesUsecase *usecase.SearchArticlesUsecase, otelCfg appOtel.Config, rlCfg config.RateLimitConfig) *http.Server {
	restHandler := rest.NewHandler(searchByUserUsecase, searchArticlesUsecase)

	mux := http.NewServeMux()

	healthHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	})

	// /v1/search is gated at the transport layer (mTLS peer-identity on the
	// :9443 listener, see newMTLSMuxHandler). The plaintext :9300 path here
	// serves only rate-limited handlers; auth has been removed pending
	// retirement of the listener itself.
	rateLimiter := middleware.NewRateLimiter(rate.Limit(rlCfg.RequestsPerSecond), rlCfg.Burst)
	searchHandler := rateLimiter.Middleware(http.HandlerFunc(restHandler.SearchArticles))

	if otelCfg.Enabled {
		mux.Handle("/v1/search", middleware.OTelStatusHandler(searchHandler, "GET /v1/search"))
		mux.Handle("/health", middleware.OTelStatusHandlerFunc(healthHandler, "GET /health"))
	} else {
		mux.Handle("/v1/search", searchHandler)
		mux.Handle("/health", healthHandler)
	}

	return &http.Server{
		Addr:              config.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

// newConnectServer creates the Connect-RPC server.
func newConnectServer(searchByUserUsecase *usecase.SearchByUserUsecase, searchRecapsUsecase *usecase.SearchRecapsUsecase, rlCfg config.RateLimitConfig) *http.Server {
	handler := connectv2.CreateConnectServer(searchByUserUsecase, searchRecapsUsecase, rlCfg)

	return &http.Server{
		Addr:              config.ConnectAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

// newMTLSMuxHandler builds the combined handler served on the :9443 mTLS
// listener: REST under /v1/* + Connect-RPC under /services.* + /health.
// All REST endpoints are gated by peer_identity (TLS client-cert CN
// allowlist). Connect-RPC already runs through the Connect interceptor stack
// with its own auth chain; the same peer_identity check is applied at the
// outer mux layer for belt-and-suspenders.
//
// this path will replace the plaintext :9300/:9301 listeners once all
// callers have moved; until then the mTLS mux runs in parallel.
func newMTLSMuxHandler(
	searchByUserUsecase *usecase.SearchByUserUsecase,
	searchArticlesUsecase *usecase.SearchArticlesUsecase,
	searchRecapsUsecase *usecase.SearchRecapsUsecase,
	connectServerHandler http.Handler,
	otelCfg appOtel.Config,
	rlCfg config.RateLimitConfig,
) http.Handler {
	restHandler := rest.NewHandler(searchByUserUsecase, searchArticlesUsecase)

	allowed := parseAllowedPeers(os.Getenv("MTLS_ALLOWED_PEERS"))
	peer := middleware.NewPeerIdentityMiddleware(allowed)
	rateLimiter := middleware.NewRateLimiter(rate.Limit(rlCfg.RequestsPerSecond), rlCfg.Burst)

	// REST /v1/search guarded by peer identity + rate limit.
	search := rateLimiter.Middleware(peer.Require(http.HandlerFunc(restHandler.SearchArticles)))
	// Connect-RPC is also gated by peer identity at the mux layer — inside,
	// the existing ServiceAuthInterceptor remains during the migration window.
	connect := peer.Require(connectServerHandler)

	mux := http.NewServeMux()

	health := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	})

	if otelCfg.Enabled {
		mux.Handle("/v1/search", middleware.OTelStatusHandler(search, "GET /v1/search"))
		mux.Handle("/health", middleware.OTelStatusHandlerFunc(health, "GET /health"))
	} else {
		mux.Handle("/v1/search", search)
		mux.Handle("/health", health)
	}
	// Connect-RPC service paths: /services.search.v2.SearchService/*
	mux.Handle("/services.search.v2.SearchService/", connect)
	// Fallback for any other Connect-RPC-style prefix.
	mux.Handle("/", connect)

	_ = searchRecapsUsecase // kept in signature so callers can pass both usecases; recap search is served via Connect-RPC inside `connect`.
	return mux
}

func parseAllowedPeers(csv string) []string {
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
