package connect

import (
	"log/slog"
	"net/http"

	"alt/gen/proto/alt/morning_letter/v2/morningletterv2connect"

	"rag-orchestrator/internal/adapter/connect/morning_letter"
	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// ServerConfig holds configuration for the Connect-RPC server
type ServerConfig struct {
	Port string
}

// SetupConnectHandlers registers all Connect-RPC handlers on the given mux
func SetupConnectHandlers(
	mux *http.ServeMux,
	articleClient domain.ArticleClient,
	answerUsecase usecase.AnswerWithRAGUsecase,
	logger *slog.Logger,
) {
	// Register MorningLetterService
	morningLetterHandler := morning_letter.NewHandler(
		articleClient,
		answerUsecase,
		logger,
	)
	path, handler := morningletterv2connect.NewMorningLetterServiceHandler(morningLetterHandler)
	mux.Handle(path, handler)
	logger.Info("Registered Connect-RPC MorningLetterService", slog.String("path", path))
}

// CreateConnectServer creates an HTTP server with Connect-RPC handlers
func CreateConnectServer(
	articleClient domain.ArticleClient,
	answerUsecase usecase.AnswerWithRAGUsecase,
	logger *slog.Logger,
) http.Handler {
	mux := http.NewServeMux()

	// Health check for Connect-RPC server
	mux.HandleFunc("/connect/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy","service":"connect-rpc"}`))
	})

	SetupConnectHandlers(mux, articleClient, answerUsecase, logger)

	// Support HTTP/2 without TLS (h2c) for Connect-RPC streaming
	return h2c.NewHandler(mux, &http2.Server{})
}
