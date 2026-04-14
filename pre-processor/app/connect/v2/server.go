// Package v2 provides Connect-RPC server setup and configuration for pre-processor.
package v2

import (
	"log/slog"
	"net/http"
	"os"
	"strings"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"pre-processor/connect/v2/interceptor"
	"pre-processor/connect/v2/preprocessor"
	preprocessorv2connect "pre-processor/gen/proto/services/preprocessor/v2/preprocessorv2connect"
	"pre-processor/repository"
)

// loadServiceSecret mirrors the REST middleware contract: SERVICE_SECRET_FILE
// (Docker secrets) takes precedence over SERVICE_SECRET, and whitespace is
// trimmed.
func loadServiceSecret(logger *slog.Logger) string {
	secret := os.Getenv("SERVICE_SECRET")
	if secretFile := os.Getenv("SERVICE_SECRET_FILE"); secretFile != "" {
		content, err := os.ReadFile(secretFile) // #nosec G304 -- path is env-configured Docker Secrets mount
		if err == nil {
			secret = strings.TrimSpace(string(content))
		} else if logger != nil {
			logger.Error("failed to read SERVICE_SECRET_FILE for Connect-RPC", "error", err)
		}
	}
	if secret == "" && logger != nil {
		logger.Warn("SERVICE_SECRET not set, Connect-RPC will deny all requests")
	}
	return secret
}

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

	// Service-to-service authentication: require X-Service-Token on every RPC.
	serviceAuth := interceptor.NewServiceAuthInterceptor(logger, loadServiceSecret(logger))

	handler := preprocessor.NewHandler(apiRepo, summaryRepo, articleRepo, jobRepo, logger)
	path, serviceHandler := preprocessorv2connect.NewPreProcessorServiceHandler(
		handler,
		connect.WithInterceptors(serviceAuth),
	)
	mux.Handle(path, serviceHandler)
	logger.Info("Registered Connect-RPC PreProcessorService", "path", path)

	// Support HTTP/2 without TLS (h2c) for internal communication
	return h2c.NewHandler(mux, &http2.Server{})
}
