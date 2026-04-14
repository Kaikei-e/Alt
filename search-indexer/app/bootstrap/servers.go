package bootstrap

import (
	"io"
	"net/http"
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
func newHTTPServer(searchByUserUsecase *usecase.SearchByUserUsecase, searchArticlesUsecase *usecase.SearchArticlesUsecase, otelCfg appOtel.Config, serviceToken string, rlCfg config.RateLimitConfig) *http.Server {
	restHandler := rest.NewHandler(searchByUserUsecase, searchArticlesUsecase)

	mux := http.NewServeMux()

	healthHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	})

	// Gate /v1/search behind X-Service-Token (ADR-000717 parity) and a global
	// rate limiter. /health stays open and unmetered for orchestrator probes.
	serviceAuth := middleware.NewServiceAuthMiddleware(serviceToken)
	rateLimiter := middleware.NewRateLimiter(rate.Limit(rlCfg.RequestsPerSecond), rlCfg.Burst)
	searchHandler := rateLimiter.Middleware(
		serviceAuth.RequireServiceAuth(http.HandlerFunc(restHandler.SearchArticles)),
	)

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
func newConnectServer(searchByUserUsecase *usecase.SearchByUserUsecase, searchRecapsUsecase *usecase.SearchRecapsUsecase, serviceToken string, rlCfg config.RateLimitConfig) *http.Server {
	handler := connectv2.CreateConnectServer(searchByUserUsecase, searchRecapsUsecase, serviceToken, rlCfg)

	return &http.Server{
		Addr:              config.ConnectAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
}
