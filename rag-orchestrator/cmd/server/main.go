package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"rag-orchestrator/internal/adapter/altdb"
	connectserver "rag-orchestrator/internal/adapter/connect"
	"rag-orchestrator/internal/adapter/rag_augur"
	rag_http "rag-orchestrator/internal/adapter/rag_http"
	"rag-orchestrator/internal/adapter/rag_http/openapi"
	"rag-orchestrator/internal/adapter/repository"
	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/infra"
	"rag-orchestrator/internal/infra/config"
	"rag-orchestrator/internal/infra/logger"
	"rag-orchestrator/internal/infra/otel"
	"rag-orchestrator/internal/usecase"
	"rag-orchestrator/internal/worker"
)

func main() {
	ctx := context.Background()

	// 1. Load Config
	cfg := config.Load()

	// 2. Initialize OpenTelemetry
	otelCfg := otel.ConfigFromEnv()
	shutdown, err := otel.InitProvider(ctx, otelCfg)
	if err != nil {
		slog.Error("failed to initialize OTel provider", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := shutdown(ctx); err != nil {
			slog.Error("failed to shutdown OTel provider", "error", err)
		}
	}()

	// 3. Initialize Logger with OTel support
	log := logger.NewWithOTel(otelCfg.Enabled)
	slog.SetDefault(log)

	// 4. Initialize DB
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName)
	dbPool, err := infra.NewPostgresDB(context.Background(), dsn)
	if err != nil {
		log.Error("failed to connect to db", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()

	// 4. Initialize Adapters
	chunkRepo := repository.NewRagChunkRepository(dbPool)
	docRepo := repository.NewRagDocumentRepository(dbPool)
	jobRepo := repository.NewRagJobRepository(dbPool)
	txManager := repository.NewPostgresTransactionManager(dbPool)
	embedder := rag_augur.NewOllamaEmbedder(cfg.OllamaURL, cfg.EmbeddingModel, cfg.OllamaTimeout)

	// 5. Initialize Usecases
	hasher := domain.NewSourceHashPolicy()
	chunker := domain.NewChunker()

	indexUsecase := usecase.NewIndexArticleUsecase(
		docRepo,
		chunkRepo,
		txManager,
		hasher,
		chunker,
		embedder,
	)

	generator := rag_augur.NewOllamaGenerator(cfg.KnowledgeAugurURL, cfg.KnowledgeAugurModel, cfg.OllamaTimeout, log)
	searchClient := rag_http.NewSearchIndexerClient(cfg.SearchIndexerURL, cfg.SearchIndexerTimeout)
	queryExpander := rag_augur.NewQueryExpanderClient(cfg.QueryExpansionURL, cfg.QueryExpansionTimeout, log)

	// Initialize optional Reranker (cross-encoder via news-creator)
	var rerankerOpt usecase.RetrieveContextOption
	if cfg.RerankEnabled {
		rerankerClient := rag_augur.NewRerankerClient(
			cfg.RerankURL,
			cfg.RerankModel,
			time.Duration(cfg.RerankTimeout)*time.Second,
			log,
		)
		rerankerOpt = usecase.WithReranker(rerankerClient)
		log.Info("reranker_enabled",
			slog.String("url", cfg.RerankURL),
			slog.String("model", cfg.RerankModel))
	}

	// Initialize optional BM25Searcher for hybrid search (via search-indexer/Meilisearch)
	var bm25SearcherOpt usecase.RetrieveContextOption
	if cfg.HybridSearchEnabled {
		// SearchIndexerClient implements both SearchClient and BM25Searcher
		bm25SearcherOpt = usecase.WithBM25Searcher(searchClient)
		log.Info("hybrid_search_enabled",
			slog.Float64("alpha", cfg.HybridAlpha),
			slog.Int("bm25_limit", cfg.HybridBM25Limit))
	}

	// Build RetrievalConfig from environment variables (research-backed defaults)
	retrievalConfig := usecase.RetrievalConfig{
		SearchLimit:   cfg.RAGSearchLimit,
		QuotaOriginal: cfg.RAGQuotaOriginal,
		QuotaExpanded: cfg.RAGQuotaExpanded,
		RRFK:          cfg.RAGRRFK,
		Reranking: usecase.RerankingConfig{
			Enabled: cfg.RerankEnabled,
			TopK:    cfg.RerankTopK,
			Timeout: time.Duration(cfg.RerankTimeout) * time.Second,
		},
		HybridSearch: usecase.HybridSearchConfig{
			Enabled:   cfg.HybridSearchEnabled,
			Alpha:     cfg.HybridAlpha,
			BM25Limit: cfg.HybridBM25Limit,
		},
		LanguageAllocation: usecase.LanguageAllocationConfig{
			Enabled: cfg.DynamicLanguageAllocationEnabled,
		},
	}

	// Build retrieve usecase with optional reranker and BM25 searcher
	var opts []usecase.RetrieveContextOption
	if rerankerOpt != nil {
		opts = append(opts, rerankerOpt)
	}
	if bm25SearcherOpt != nil {
		opts = append(opts, bm25SearcherOpt)
	}
	retrieveUsecase := usecase.NewRetrieveContextUsecase(chunkRepo, docRepo, embedder, generator, searchClient, queryExpander, retrievalConfig, log, opts...)
	promptBuilder := usecase.NewXMLPromptBuilder("Answer in Japanese.")
	answerUsecase := usecase.NewAnswerWithRAGUsecase(
		retrieveUsecase,
		promptBuilder,
		generator,
		usecase.NewOutputValidator(),
		cfg.AnswerMaxChunks,
		cfg.AnswerMaxTokens,
		cfg.PromptVersion,
		cfg.DefaultLocale,
		log,
	)

	// Initialize ArticleClient for morning letter
	articleClient := altdb.NewHTTPArticleClient(
		cfg.AltBackendURL,
		time.Duration(cfg.AltBackendTimeout)*time.Second,
		log,
	)
	morningLetterPromptBuilder := usecase.NewMorningLetterPromptBuilder()

	// Build TemporalBoostConfig from environment variables
	temporalBoostConfig := usecase.TemporalBoostConfig{
		Boost6h:  cfg.TemporalBoost6h,
		Boost12h: cfg.TemporalBoost12h,
		Boost18h: cfg.TemporalBoost18h,
	}
	morningLetterUsecase := usecase.NewMorningLetterUsecase(
		articleClient,
		retrieveUsecase,
		morningLetterPromptBuilder,
		generator,
		temporalBoostConfig,
		log,
	)

	// 6. Initialize & Start Worker
	jobWorker := worker.NewJobWorker(jobRepo, indexUsecase, log)
	jobWorker.Start()
	// Ensure worker stops on shutdown
	defer func() {
		log.Info("Stopping worker...")
		jobWorker.Stop()
	}()

	// 7. Initialize Echo
	e := echo.New()
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())

	// 8. Initialize Handlers
	// Create factories for hyper-boost support
	embedderFactory := func(url string, model string, timeout int) domain.VectorEncoder {
		return rag_augur.NewOllamaEmbedder(url, model, timeout)
	}
	indexUsecaseFactory := func(encoder domain.VectorEncoder) usecase.IndexArticleUsecase {
		return usecase.NewIndexArticleUsecase(
			docRepo,
			chunkRepo,
			txManager,
			hasher,
			chunker,
			encoder,
		)
	}
	handler := rag_http.NewHandler(
		retrieveUsecase,
		answerUsecase,
		indexUsecase,
		jobRepo,
		morningLetterUsecase,
		rag_http.WithEmbedderOverride(embedderFactory, indexUsecaseFactory, cfg.EmbeddingModel, cfg.OllamaTimeout),
	)

	// 9. Register OpenAPI Handlers
	openapi.RegisterHandlers(e, handler)

	// 10. Manual Registration for Backfill and Morning Letter
	e.POST("/internal/rag/backfill", handler.Backfill)
	e.POST("/v1/rag/morning-letter", handler.MorningLetter)

	// 11. Health Checks
	e.GET("/healthz", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})
	e.GET("/readyz", func(c echo.Context) error {
		if err := dbPool.Ping(c.Request().Context()); err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "db down", "error": err.Error()})
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
	})

	// 12. Start Echo Server (REST/OpenAPI)
	go func() {
		addr := fmt.Sprintf(":%s", cfg.Port)
		log.Info("Starting Echo server", "addr", addr)
		if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
			e.Logger.Fatal("shutting down the server")
		}
	}()

	// 13. Start Connect-RPC Server
	connectHandler := connectserver.CreateConnectServer(articleClient, answerUsecase, retrieveUsecase, log)
	connectServer := &http.Server{
		Addr:              fmt.Sprintf(":%s", cfg.ConnectPort),
		Handler:           connectHandler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Info("Starting Connect-RPC server", "addr", connectServer.Addr)
		if err := connectServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("Connect-RPC server error", "error", err)
		}
	}()

	// 14. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Shutdown both servers
	if err := connectServer.Shutdown(ctx); err != nil {
		log.Error("Connect-RPC server shutdown error", "error", err)
	}
	if err := e.Shutdown(ctx); err != nil {
		e.Logger.Fatal(err)
	}
}
