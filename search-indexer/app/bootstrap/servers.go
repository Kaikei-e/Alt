package bootstrap

import (
	"io"
	"net/http"
	"time"

	connectv2 "search-indexer/connect/v2"
	"search-indexer/config"
	"search-indexer/middleware"
	"search-indexer/rest"
	"search-indexer/usecase"
	appOtel "search-indexer/utils/otel"
)

// newHTTPServer creates the REST HTTP server.
func newHTTPServer(searchByUserUsecase *usecase.SearchByUserUsecase, otelCfg appOtel.Config) *http.Server {
	restHandler := rest.NewHandler(searchByUserUsecase)

	mux := http.NewServeMux()

	healthHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	})

	if otelCfg.Enabled {
		mux.Handle("/v1/search", middleware.OTelStatusHandlerFunc(restHandler.SearchArticles, "GET /v1/search"))
		mux.Handle("/health", middleware.OTelStatusHandlerFunc(healthHandler, "GET /health"))
	} else {
		mux.HandleFunc("/v1/search", restHandler.SearchArticles)
		mux.Handle("/health", healthHandler)
	}

	return &http.Server{
		Addr:              config.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

// newConnectServer creates the Connect-RPC server.
func newConnectServer(searchByUserUsecase *usecase.SearchByUserUsecase) *http.Server {
	handler := connectv2.CreateConnectServer(searchByUserUsecase)

	return &http.Server{
		Addr:              config.ConnectAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
}
