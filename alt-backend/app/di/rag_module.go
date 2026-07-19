package di

import (
	"alt/adapter/augur_adapter"
	"alt/orchestrator/gateway/morning_gateway"
	"alt/orchestrator/gateway/morning_letter_connect_gateway"
	"alt/orchestrator/gateway/rag_connect_gateway"
	"alt/orchestrator/gateway/rag_gateway"
	"alt/orchestrator/gateway/user_feed_gateway"
	"alt/orchestrator/port/morning_letter_port"
	"alt/orchestrator/port/rag_integration_port"
	"alt/orchestrator/usecase/answer_chat_usecase"
	"alt/orchestrator/usecase/morning_usecase"
	"alt/orchestrator/usecase/retrieve_context_usecase"
	"alt/tlsutil"
	"log/slog"
	"net/http"
	"os"
	"strings"
)

// RAGModule holds all RAG-domain components.
type RAGModule struct {
	// Adapter (shared across article and RAG modules)
	RagAdapter rag_integration_port.RagIntegrationPort

	// Usecases
	RetrieveContextUsecase retrieve_context_usecase.RetrieveContextUsecase
	AnswerChatUsecase      answer_chat_usecase.AnswerChatUsecase
	MorningUsecase         morning_letter_port.MorningUsecase
	MorningLetterUsecase   morning_letter_port.MorningLetterUsecase

	// Clients / Ports
	RagConnectClient *rag_connect_gateway.Client
	StreamChatPort   morning_letter_port.StreamChatPort
}

// newRagConnectHTTPClient builds the HTTP client for the rag-orchestrator
// Connect-RPC hop. https scheme => mTLS with the leaf cert from MTLS_CERT_FILE
// / MTLS_KEY_FILE / MTLS_CA_FILE (same files the :9443 listener uses); any
// failure panics so a missing cert can never degrade to plaintext. http
// scheme keeps the historical plaintext client. Both branches log loudly.
func newRagConnectHTTPClient(connectURL string) *http.Client {
	if strings.HasPrefix(connectURL, "https://") {
		client, err := tlsutil.NewMTLSClient(
			os.Getenv("MTLS_CERT_FILE"),
			os.Getenv("MTLS_KEY_FILE"),
			os.Getenv("MTLS_CA_FILE"),
		)
		if err != nil {
			slog.Default().Error("rag_connect_mtls_client_failed", "error", err, "url", connectURL)
			panic("rag-orchestrator Connect URL is https but mTLS client construction failed: " + err.Error())
		}
		slog.Default().Info("rag_connect_mtls_enabled", "url", connectURL)
		return client
	}
	slog.Default().Warn("rag_connect_plaintext", "url", connectURL,
		"reason", "RAG_ORCHESTRATOR_CONNECT_URL is http; X-Alt-User-Id hop is protected by network policy only")
	return &http.Client{}
}

func newRAGModule(infra *InfraModule, feed *FeedModule) *RAGModule {
	cfg := infra.Config

	// RAG Integration (REST client)
	ragClient, err := rag_gateway.NewClientWithResponses(cfg.Rag.OrchestratorURL)
	if err != nil {
		panic("Failed to create RAG client: " + err.Error())
	}
	ragAdapter := augur_adapter.NewAugurAdapter(ragClient)

	ragRetrieveContextUC := retrieve_context_usecase.NewRetrieveContextUsecase(feed.SearchFeedMeilisearchGateway, ragAdapter)
	answerChatUC := answer_chat_usecase.NewAnswerChatUsecase(ragAdapter)

	// RAG Connect-RPC transport. An https:// RAG_ORCHESTRATOR_CONNECT_URL
	// means rag-orchestrator's listener runs PEER_IDENTITY_MODE=mtls and this
	// service must present its pki-agent leaf cert; failing to build that
	// transport is a startup error, never a plaintext fallback.
	ragConnectHTTPClient := newRagConnectHTTPClient(cfg.Rag.OrchestratorConnectURL)

	// RAG Connect-RPC client (for direct Connect-RPC communication with rag-orchestrator)
	ragConnectClient := rag_connect_gateway.NewClient(ragConnectHTTPClient, cfg.Rag.OrchestratorConnectURL, slog.Default())

	// MorningLetter Connect-RPC gateway (calls rag-orchestrator)
	morningLetterConnectGw := morning_letter_connect_gateway.NewGateway(ragConnectHTTPClient, cfg.Rag.OrchestratorConnectURL, slog.Default())

	// Morning letter usecase
	userFeedGw := user_feed_gateway.NewGateway(infra.AltDBRepository)
	morningGw := morning_gateway.NewMorningGateway(infra.Pool)
	morningUC := morning_usecase.NewMorningUsecase(morningGw, userFeedGw)

	// Morning Letter v2 read usecase + v3 enrichment ports.
	// ArticleRepository is embedded on AltDBRepository so FetchArticlesByIDs
	// is method-promoted; same for the newly added FetchFeedTitlesByIDs.
	// SearchIndexerDriver provides the related-articles fan-out.
	morningLetterGw := morning_gateway.NewMorningLetterGateway(infra.Pool)
	morningLetterUC := morning_usecase.NewMorningLetterUsecaseWithEnrichment(
		morningLetterGw,
		userFeedGw,
		infra.AltDBRepository,
		infra.AltDBRepository,
		infra.SearchIndexerDriver,
	)

	return &RAGModule{
		RagAdapter:             ragAdapter,
		RetrieveContextUsecase: ragRetrieveContextUC,
		AnswerChatUsecase:      answerChatUC,
		MorningUsecase:         morningUC,
		MorningLetterUsecase:   morningLetterUC,
		RagConnectClient:       ragConnectClient,
		StreamChatPort:         morningLetterConnectGw,
	}
}
