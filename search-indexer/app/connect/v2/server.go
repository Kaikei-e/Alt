// Package v2 provides Connect-RPC server setup and configuration for search-indexer.
package v2

import (
	"net/http"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/time/rate"

	"search-indexer/config"
	"search-indexer/connect/v2/interceptor"
	"search-indexer/connect/v2/search"
	searchv2connect "search-indexer/gen/proto/services/search/v2/searchv2connect"
	"search-indexer/logger"
	"search-indexer/usecase"
)

// CreateConnectServer creates the Connect-RPC server with HTTP/2 support.
// All SearchService endpoints require X-Service-Token and share a global rate
// limiter; see ADR-000717 for the Public/Internal authentication boundary.
func CreateConnectServer(searchByUserUsecase *usecase.SearchByUserUsecase, searchRecapsUsecase *usecase.SearchRecapsUsecase, serviceToken string, rlCfg config.RateLimitConfig) http.Handler {
	mux := http.NewServeMux()

	// Health check endpoint stays open so orchestrators can probe liveness.
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy","service":"connect-rpc"}`))
	})

	serviceAuth := interceptor.NewServiceAuthInterceptor(serviceToken)
	rateLimit := interceptor.NewRateLimitInterceptor(rate.Limit(rlCfg.RequestsPerSecond), rlCfg.Burst)
	searchHandler := search.NewHandler(searchByUserUsecase, searchRecapsUsecase)
	searchPath, searchServiceHandler := searchv2connect.NewSearchServiceHandler(
		searchHandler,
		connect.WithInterceptors(rateLimit, serviceAuth),
	)
	mux.Handle(searchPath, searchServiceHandler)
	logger.Logger.Info("Registered Connect-RPC SearchService", "path", searchPath)

	// HTTP/2 without TLS (h2c) for internal communication within the compose network.
	return h2c.NewHandler(mux, &http2.Server{})
}
