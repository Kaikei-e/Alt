// Package v2 provides Connect-RPC server setup and configuration for search-indexer.
package v2

import (
	"net/http"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/meilisearch/meilisearch-go"

	"search-indexer/connect/v2/search"
	searchv2connect "search-indexer/gen/proto/services/search/v2/searchv2connect"
	"search-indexer/logger"
)

// CreateConnectServer creates the Connect-RPC server with HTTP/2 support.
func CreateConnectServer(idx meilisearch.IndexManager) http.Handler {
	mux := http.NewServeMux()

	// Add health check endpoint for Connect-RPC server
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy","service":"connect-rpc"}`))
	})

	// Register Search service (no auth interceptor for internal service communication)
	searchHandler := search.NewHandler(idx)
	searchPath, searchServiceHandler := searchv2connect.NewSearchServiceHandler(searchHandler)
	mux.Handle(searchPath, searchServiceHandler)
	logger.Logger.Info("Registered Connect-RPC SearchService", "path", searchPath)

	// Support HTTP/2 without TLS (h2c) for internal communication
	return h2c.NewHandler(mux, &http2.Server{})
}
