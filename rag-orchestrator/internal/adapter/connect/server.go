package connect

import (
	"log/slog"
	"net/http"

	"alt/gen/proto/alt/augur/v2/augurv2connect"
	"alt/gen/proto/alt/morning_letter/v2/morningletterv2connect"

	"rag-orchestrator/internal/adapter/connect/augur"
	"rag-orchestrator/internal/adapter/connect/morning_letter"
	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/middleware"
	"rag-orchestrator/internal/usecase"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// ServerConfig holds configuration for the Connect-RPC server
type ServerConfig struct {
	Port string
}

// SetupConnectHandlers registers all Connect-RPC handlers on the given mux.
// eventEmitter publishes augur.conversation_linked.v1 into knowledge-sovereign;
// pass usecase.NoopKnowledgeEventEmitter{} when emit is intentionally
// disabled (tests, or production until alt-deploy registers
// rag-orchestrator as a knowledge-sovereign pacticipant).
func SetupConnectHandlers(
	mux *http.ServeMux,
	articleClient domain.ArticleClient,
	answerUsecase usecase.AnswerWithRAGUsecase,
	retrieveUsecase usecase.RetrieveContextUsecase,
	conversationUsecase usecase.AugurConversationUsecase,
	eventEmitter usecase.KnowledgeEventEmitter,
	letterFetcher domain.MorningLetterFetcher,
	logger *slog.Logger,
) {
	// Register MorningLetterService
	morningLetterHandler := morning_letter.NewHandler(
		articleClient,
		answerUsecase,
		letterFetcher,
		logger,
	)
	mlPath, mlHandler := morningletterv2connect.NewMorningLetterServiceHandler(morningLetterHandler)
	mux.Handle(mlPath, mlHandler)
	logger.Info("Registered Connect-RPC MorningLetterService", slog.String("path", mlPath))

	// Register AugurService
	augurHandler := augur.NewHandler(
		answerUsecase,
		retrieveUsecase,
		conversationUsecase,
		eventEmitter,
		logger,
	)
	augurPath, augurHTTPHandler := augurv2connect.NewAugurServiceHandler(augurHandler)
	mux.Handle(augurPath, augurHTTPHandler)
	logger.Info("Registered Connect-RPC AugurService", slog.String("path", augurPath))
}

func newConnectMux(
	articleClient domain.ArticleClient,
	answerUsecase usecase.AnswerWithRAGUsecase,
	retrieveUsecase usecase.RetrieveContextUsecase,
	conversationUsecase usecase.AugurConversationUsecase,
	eventEmitter usecase.KnowledgeEventEmitter,
	letterFetcher domain.MorningLetterFetcher,
	logger *slog.Logger,
) *http.ServeMux {
	mux := http.NewServeMux()

	// Health check for Connect-RPC server
	mux.HandleFunc("/connect/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy","service":"connect-rpc"}`))
	})

	SetupConnectHandlers(mux, articleClient, answerUsecase, retrieveUsecase, conversationUsecase, eventEmitter, letterFetcher, logger)
	return mux
}

// CreateConnectServer creates the plaintext (h2c) Connect-RPC handler chain.
// Only valid when PEER_IDENTITY_MODE=disabled: X-Alt-User-Id is unverified on
// this listener, so exposure must be limited by network policy.
func CreateConnectServer(
	articleClient domain.ArticleClient,
	answerUsecase usecase.AnswerWithRAGUsecase,
	retrieveUsecase usecase.RetrieveContextUsecase,
	conversationUsecase usecase.AugurConversationUsecase,
	eventEmitter usecase.KnowledgeEventEmitter,
	letterFetcher domain.MorningLetterFetcher,
	logger *slog.Logger,
) http.Handler {
	mux := newConnectMux(articleClient, answerUsecase, retrieveUsecase, conversationUsecase, eventEmitter, letterFetcher, logger)

	// Support HTTP/2 without TLS (h2c) for Connect-RPC streaming
	return h2c.NewHandler(mux, &http2.Server{})
}

// CreateMTLSConnectServer creates the Connect-RPC handler chain for an
// mTLS-terminating listener: every request must carry a verified client cert
// whose CN passes peerMW's allowlist before any RPC handler (and therefore
// extractUserID's X-Alt-User-Id trust) is reached. No h2c wrapper — the TLS
// listener negotiates HTTP/2 via ALPN. peerMW is mandatory: a nil middleware
// here means the composition root forgot to wire peer identity, which must
// fail loudly rather than serve unguarded (CLAUDE.md rule 8).
func CreateMTLSConnectServer(
	peerMW *middleware.PeerIdentityMiddleware,
	articleClient domain.ArticleClient,
	answerUsecase usecase.AnswerWithRAGUsecase,
	retrieveUsecase usecase.RetrieveContextUsecase,
	conversationUsecase usecase.AugurConversationUsecase,
	eventEmitter usecase.KnowledgeEventEmitter,
	letterFetcher domain.MorningLetterFetcher,
	logger *slog.Logger,
) http.Handler {
	if peerMW == nil {
		panic("connect: CreateMTLSConnectServer called without peer-identity middleware")
	}
	mux := newConnectMux(articleClient, answerUsecase, retrieveUsecase, conversationUsecase, eventEmitter, letterFetcher, logger)
	return peerMW.Require(mux)
}
