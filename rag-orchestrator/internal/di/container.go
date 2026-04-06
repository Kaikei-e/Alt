package di

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"connectrpc.com/connect"

	"github.com/jackc/pgx/v5/pgxpool"

	"rag-orchestrator/internal/adapter/altdb"
	"rag-orchestrator/internal/adapter/rag_augur"
	rag_http "rag-orchestrator/internal/adapter/rag_http"
	"rag-orchestrator/internal/adapter/repository"
	"rag-orchestrator/internal/adapter/tools"
	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/infra/config"
	"rag-orchestrator/internal/infra/httpclient"
	"rag-orchestrator/internal/usecase"
	"rag-orchestrator/internal/worker"

	"alt/gen/proto/services/backend/v1/backendv1connect"
)

// ApplicationComponents holds all wired dependencies for the application.
type ApplicationComponents struct {
	// Repositories
	ChunkRepo domain.RagChunkRepository
	DocRepo   domain.RagDocumentRepository
	JobRepo   domain.RagJobRepository

	// Usecases
	IndexUsecase         usecase.IndexArticleUsecase
	RetrieveUsecase      usecase.RetrieveContextUsecase
	AnswerUsecase        usecase.AnswerWithRAGUsecase
	MorningLetterUsecase usecase.MorningLetterUsecase

	// Worker
	Worker *worker.JobWorker

	// Factories (for hyper-boost support)
	EmbedderFactory     rag_http.EmbedderFactory
	IndexUsecaseFactory rag_http.IndexUsecaseFactory

	// Adapters exposed for handler wiring
	ArticleClient   domain.ArticleClient
	EmbeddingModel  string
	EmbedderTimeout int
}

// NewApplicationComponents wires all dependencies from config and database pool.
func NewApplicationComponents(cfg *config.Config, pool *pgxpool.Pool, log *slog.Logger) *ApplicationComponents {
	// Repositories
	chunkRepo := repository.NewRagChunkRepository(pool)
	docRepo := repository.NewRagDocumentRepository(pool)
	jobRepo := repository.NewRagJobRepository(pool)
	txManager := repository.NewPostgresTransactionManager(pool)

	// Shared HTTP clients with connection pooling
	embedderHTTP := httpclient.NewPooledClient(time.Duration(cfg.Embedder.Timeout) * time.Second)
	augurHTTP := httpclient.NewPooledClient(time.Duration(cfg.Augur.Timeout) * time.Second)
	queryExpanderHTTP := httpclient.NewPooledClient(time.Duration(cfg.QueryExpansion.Timeout) * time.Second)
	rerankHTTP := httpclient.NewPooledClient(time.Duration(cfg.Rerank.Timeout) * time.Second)

	// External clients
	embedder := rag_augur.NewOllamaEmbedder(cfg.Embedder.URL, cfg.Embedder.Model, cfg.Embedder.Timeout, embedderHTTP)
	generator := rag_augur.NewOllamaGenerator(cfg.Augur.URL, cfg.Augur.Model, cfg.Augur.Timeout, log, augurHTTP)
	searchClient := rag_http.NewSearchIndexerClient(cfg.Search.IndexerURL, cfg.Search.Timeout)
	queryExpander := rag_augur.NewQueryExpanderClient(cfg.QueryExpansion.URL, cfg.QueryExpansion.Timeout, log, queryExpanderHTTP)

	// Domain services
	hasher := domain.NewSourceHashPolicy()
	chunker := domain.NewChunker()

	// Index usecase
	indexUsecase := usecase.NewIndexArticleUsecase(docRepo, chunkRepo, txManager, hasher, chunker, embedder)

	// Retrieval config
	retrievalConfig := usecase.RetrievalConfig{
		SearchLimit:   cfg.RAG.SearchLimit,
		QuotaOriginal: cfg.RAG.QuotaOriginal,
		QuotaExpanded: cfg.RAG.QuotaExpanded,
		RRFK:          cfg.RAG.RRFK,
		Reranking: usecase.RerankingConfig{
			Enabled: cfg.Rerank.Enabled,
			TopK:    cfg.Rerank.TopK,
			Timeout: time.Duration(cfg.Rerank.Timeout) * time.Second,
		},
		HybridSearch: usecase.HybridSearchConfig{
			Enabled:   cfg.Hybrid.Enabled,
			Alpha:     cfg.Hybrid.Alpha,
			BM25Limit: cfg.Hybrid.BM25Limit,
		},
		LanguageAllocation: usecase.LanguageAllocationConfig{
			Enabled: cfg.RAG.DynamicLanguageAllocationEnabled,
		},
	}

	// Optional components
	var opts []usecase.RetrieveContextOption
	if cfg.Rerank.Enabled {
		rerankerClient := rag_augur.NewRerankerClient(
			cfg.Rerank.URL,
			cfg.Rerank.Model,
			time.Duration(cfg.Rerank.Timeout)*time.Second,
			log,
			rerankHTTP,
		)
		opts = append(opts, usecase.WithReranker(rerankerClient))
		log.Info("reranker_enabled",
			slog.String("url", cfg.Rerank.URL),
			slog.String("model", cfg.Rerank.Model))
	}
	if cfg.Hybrid.Enabled {
		opts = append(opts, usecase.WithBM25Searcher(searchClient))
		log.Info("hybrid_search_enabled",
			slog.Float64("alpha", cfg.Hybrid.Alpha),
			slog.Int("bm25_limit", cfg.Hybrid.BM25Limit))
	}

	// Retrieve usecase
	retrieveUsecase := usecase.NewRetrieveContextUsecase(
		chunkRepo, docRepo, embedder, generator, searchClient, queryExpander,
		retrievalConfig, log, opts...,
	)

	// Answer usecase
	promptBuilder := usecase.NewXMLPromptBuilder("Answer in Japanese.")
	articleScopedStrategy := usecase.NewArticleScopedStrategy(docRepo, chunkRepo, log, queryExpander)

	// Agentic RAG options (ADR-000568)
	answerOpts := []usecase.AnswerUsecaseOption{
		usecase.WithCacheConfig(cfg.Cache.Size, time.Duration(cfg.Cache.TTL)*time.Minute),
		usecase.WithStrategy(usecase.IntentArticleScoped, articleScopedStrategy),
		usecase.WithStrategy(usecase.IntentTemporal, usecase.NewTemporalStrategy(retrieveUsecase, log)),
		usecase.WithStrategy(usecase.IntentComparison, usecase.NewComparisonStrategy(retrieveUsecase, log)),
		usecase.WithStrategy(usecase.IntentTopicDeepDive, usecase.NewTopicDeepDiveStrategy(retrieveUsecase, log)),
		usecase.WithStrategy(usecase.IntentFactCheck, usecase.NewFactCheckStrategy(retrieveUsecase, log)),
		usecase.WithQueryClassifier(usecase.NewQueryClassifier(nil, 0)),
	}
	// Tool dispatcher (Agentic RAG: subintent-driven tool selection + synthesis tools)
	relatedArticlesTool := tools.NewRelatedArticlesTool(searchClient)

	// Connect-RPC clients for alt-backend and recap service (ADR-000617: Tool-Use Agentic RAG)
	var connectOpts []connect.ClientOption
	if cfg.Backend.ServiceToken != "" {
		connectOpts = append(connectOpts, connect.WithInterceptors(
			newServiceTokenInterceptor(cfg.Backend.ServiceToken),
		))
		log.Info("connect_rpc_service_token_configured")
	}
	backendInternalClient := backendv1connect.NewBackendInternalServiceClient(
		http.DefaultClient, cfg.Backend.ConnectURL, connectOpts...,
	)
	tagCloudClient := altdb.NewInternalTagCloudClient(backendInternalClient, log)
	articlesByTagClient := altdb.NewInternalArticlesByTagClient(backendInternalClient, log)

	toolMap := map[string]domain.Tool{
		"related_articles":  relatedArticlesTool,
		"tag_cloud_explore": tools.NewTagCloudExploreTool(tagCloudClient),
		"articles_by_tag":   tools.NewArticlesByTagTool(articlesByTagClient),
		// search_recaps: Connect-RPC to recap service pending (requires recap-worker internal API)
		// "search_recaps":     tools.NewSearchRecapsTool(recapSearchClient),
		"tag_search":        tools.NewTagSearchTool(searchClient),
		"date_range_filter": tools.NewDateRangeFilterTool(searchClient),
	}
	toolDispatcher := usecase.NewToolDispatcher(toolMap, log)
	answerOpts = append(answerOpts, usecase.WithToolDispatcher(toolDispatcher))
	log.Info("tool_dispatcher_enabled", slog.Int("tools", len(toolMap)))

	// Synthesis strategy (ADR-000617: Tool-Use Agentic RAG)
	toolDescriptors := make([]domain.ToolDescriptor, 0, len(toolMap))
	for _, t := range toolMap {
		toolDescriptors = append(toolDescriptors, domain.ToolDescriptor{
			Name:        t.Name(),
			Description: t.Description(),
		})
	}
	toolPlanner := usecase.NewToolPlanner(generator, toolDescriptors, log)
	synthStrategy := usecase.NewSynthesisStrategy(toolPlanner, toolDispatcher, retrieveUsecase, log)
	answerOpts = append(answerOpts, usecase.WithStrategy(usecase.IntentSynthesis, synthStrategy))
	log.Info("synthesis_strategy_enabled")

	if cfg.QualityGate.Enabled {
		assessor := usecase.NewRetrievalQualityAssessor(
			cfg.QualityGate.GoodThreshold,
			cfg.QualityGate.MarginalThreshold,
			cfg.QualityGate.MinContexts,
		)
		answerOpts = append(answerOpts, usecase.WithQualityAssessor(assessor, queryExpander))
		log.Info("quality_gate_enabled",
			slog.Float64("good_threshold", float64(cfg.QualityGate.GoodThreshold)),
			slog.Float64("marginal_threshold", float64(cfg.QualityGate.MarginalThreshold)))
	}

	// Conversation planner + state store (ADR-000604)
	classifier := usecase.NewQueryClassifier(nil, 0)
	planner := usecase.NewConversationPlanner(classifier)
	conversationStore := usecase.NewConversationStore(1024, 30*time.Minute)
	answerOpts = append(answerOpts, usecase.WithConversationPlanner(planner, conversationStore))
	log.Info("conversation_planner_enabled")

	// LLM-based query planner via news-creator (ADR-000630)
	queryPlannerClient := rag_augur.NewQueryPlannerClient(
		cfg.QueryExpansion.URL, cfg.QueryExpansion.Timeout, log,
	)
	answerOpts = append(answerOpts, usecase.WithQueryPlanner(queryPlannerClient))
	log.Info("query_planner_enabled", slog.String("url", cfg.QueryExpansion.URL))

	// Cross-encoder relevance gate (ADR-000630)
	relevanceGate := usecase.NewRelevanceGate(0.5, 0.25)
	answerOpts = append(answerOpts, usecase.WithRelevanceGate(relevanceGate))
	log.Info("relevance_gate_enabled")

	answerUsecase := usecase.NewAnswerWithRAGUsecase(
		retrieveUsecase, promptBuilder, generator, usecase.NewOutputValidator(cfg.RAG.MinAnswerLength),
		cfg.RAG.MaxChunks, cfg.RAG.MaxTokens, cfg.RAG.MaxPromptTokens,
		cfg.RAG.PromptVersion, cfg.RAG.Locale, log,
		answerOpts...,
	)

	// Morning letter usecase
	articleClient := altdb.NewHTTPArticleClient(
		cfg.Backend.URL,
		time.Duration(cfg.Backend.Timeout)*time.Second,
		log,
	)
	morningLetterPromptBuilder := usecase.NewMorningLetterPromptBuilder()
	temporalBoostConfig := usecase.TemporalBoostConfig{
		Boost6h:  cfg.Temporal.Boost6h,
		Boost12h: cfg.Temporal.Boost12h,
		Boost18h: cfg.Temporal.Boost18h,
	}
	morningLetterUsecase := usecase.NewMorningLetterUsecase(
		articleClient, retrieveUsecase, morningLetterPromptBuilder,
		generator, cfg.RAG.MorningLetterMaxTokens, cfg.RAG.MaxPromptTokens, temporalBoostConfig, log,
	)

	// Factories for hyper-boost
	embedderFactory := func(url string, model string, timeout int) domain.VectorEncoder {
		return rag_augur.NewOllamaEmbedder(url, model, timeout, httpclient.NewPooledClient(time.Duration(timeout)*time.Second))
	}
	indexUsecaseFactory := func(encoder domain.VectorEncoder) usecase.IndexArticleUsecase {
		return usecase.NewIndexArticleUsecase(docRepo, chunkRepo, txManager, hasher, chunker, encoder)
	}

	// Worker
	jobWorker := worker.NewJobWorker(jobRepo, indexUsecase, log)

	return &ApplicationComponents{
		ChunkRepo:            chunkRepo,
		DocRepo:              docRepo,
		JobRepo:              jobRepo,
		IndexUsecase:         indexUsecase,
		RetrieveUsecase:      retrieveUsecase,
		AnswerUsecase:        answerUsecase,
		MorningLetterUsecase: morningLetterUsecase,
		Worker:               jobWorker,
		EmbedderFactory:      embedderFactory,
		IndexUsecaseFactory:  indexUsecaseFactory,
		ArticleClient:        articleClient,
		EmbeddingModel:       cfg.Embedder.Model,
		EmbedderTimeout:      cfg.Embedder.Timeout,
	}
}

// serviceTokenInterceptor adds X-Service-Token header to all Connect-RPC requests.
type serviceTokenInterceptor struct {
	token string
}

func newServiceTokenInterceptor(token string) *serviceTokenInterceptor {
	return &serviceTokenInterceptor{token: token}
}

func (i *serviceTokenInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		req.Header().Set("X-Service-Token", i.token)
		return next(ctx, req)
	}
}

func (i *serviceTokenInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		conn.RequestHeader().Set("X-Service-Token", i.token)
		return conn
	}
}

func (i *serviceTokenInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next // No-op for handler side
}
