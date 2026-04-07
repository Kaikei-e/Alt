package di

import (
	"alt/adapter/augur_adapter"
	"alt/gateway/morning_gateway"
	"alt/gateway/morning_letter_connect_gateway"
	"alt/gateway/rag_connect_gateway"
	"alt/gateway/rag_gateway"
	"alt/gateway/user_feed_gateway"
	"alt/port/morning_letter_port"
	"alt/port/rag_integration_port"
	"alt/usecase/answer_chat_usecase"
	"alt/usecase/morning_usecase"
	"alt/usecase/retrieve_context_usecase"
	"log/slog"
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

	// RAG Connect-RPC client (for direct Connect-RPC communication with rag-orchestrator)
	ragConnectClient := rag_connect_gateway.NewClient(cfg.Rag.OrchestratorConnectURL, slog.Default())

	// MorningLetter Connect-RPC gateway (calls rag-orchestrator)
	morningLetterConnectGw := morning_letter_connect_gateway.NewGateway(cfg.Rag.OrchestratorConnectURL, slog.Default())

	// Morning letter usecase
	userFeedGw := user_feed_gateway.NewGateway(infra.AltDBRepository)
	morningGw := morning_gateway.NewMorningGateway(infra.Pool)
	morningUC := morning_usecase.NewMorningUsecase(morningGw, userFeedGw)

	// Morning Letter v2 read usecase
	morningLetterGw := morning_gateway.NewMorningLetterGateway(infra.Pool)
	morningLetterUC := morning_usecase.NewMorningLetterUsecase(morningLetterGw, userFeedGw)

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
