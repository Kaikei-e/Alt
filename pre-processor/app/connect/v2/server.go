// Package v2 provides Connect-RPC server setup and configuration for pre-processor.
package v2

import (
	"log/slog"
	"net/http"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"pre-processor/connect/v2/preprocessor"
	preprocessorv2connect "pre-processor/gen/proto/services/preprocessor/v2/preprocessorv2connect"
	"pre-processor/repository"
)

// CreateConnectServer creates the Connect-RPC server with HTTP/2 support.
func CreateConnectServer(
	apiRepo repository.ExternalAPIRepository,
	summaryRepo repository.SummaryRepository,
	articleRepo repository.ArticleRepository,
	jobRepo repository.SummarizeJobRepository,
	logger *slog.Logger,
) http.Handler {
	mux := http.NewServeMux()

	// Add health check endpoint for Connect-RPC server
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy","service":"connect-rpc"}`))
	})

	// Register PreProcessor service (no auth interceptor for internal service communication)
	handler := preprocessor.NewHandler(apiRepo, summaryRepo, articleRepo, jobRepo, logger)
	path, serviceHandler := preprocessorv2connect.NewPreProcessorServiceHandler(handler)
	mux.Handle(path, serviceHandler)
	logger.Info("Registered Connect-RPC PreProcessorService", "path", path)

	// Support HTTP/2 without TLS (h2c) for internal communication
	return h2c.NewHandler(mux, &http2.Server{})
}
