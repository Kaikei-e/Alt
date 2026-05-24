// Package v2 provides Connect-RPC server setup and configuration for search-indexer.
package v2

import (
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
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
// Authentication is established at the TLS transport layer (mTLS peer-identity
// on :9443); the plaintext h2c mux here carries only a shared rate limiter and
// the OTel server-side interceptor so Connect-RPC procedures emit spans into
// the rask_logs otel_traces table.
func CreateConnectServer(searchByUserUsecase *usecase.SearchByUserUsecase, searchRecapsUsecase *usecase.SearchRecapsUsecase, rlCfg config.RateLimitConfig) http.Handler {
	mux := http.NewServeMux()

	// Health check endpoint stays open so orchestrators can probe liveness.
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy","service":"connect-rpc"}`))
	})

	rateLimit := interceptor.NewRateLimitInterceptor(rate.Limit(rlCfg.RequestsPerSecond), rlCfg.Burst)

	// otelInterceptor is the outermost layer so spans wrap rate-limit rejections too.
	// Failure to construct it must not block service start: rate limiting still works.
	interceptors := []connect.Interceptor{}
	if otelInt, err := otelconnect.NewInterceptor(); err == nil {
		interceptors = append(interceptors, otelInt)
	} else {
		logger.Logger.Warn("Failed to create OTel Connect interceptor, proceeding without tracing", "error", err)
	}
	interceptors = append(interceptors, rateLimit)

	searchHandler := search.NewHandler(searchByUserUsecase, searchRecapsUsecase)
	searchPath, searchServiceHandler := searchv2connect.NewSearchServiceHandler(
		searchHandler,
		connect.WithInterceptors(interceptors...),
	)
	mux.Handle(searchPath, searchServiceHandler)
	logger.Logger.Info("Registered Connect-RPC SearchService", "path", searchPath)

	// HTTP/2 without TLS (h2c) for internal communication within the compose network.
	return h2c.NewHandler(mux, &http2.Server{})
}
