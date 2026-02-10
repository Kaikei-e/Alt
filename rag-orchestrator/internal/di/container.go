package di

import (
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"rag-orchestrator/internal/adapter/altdb"
	"rag-orchestrator/internal/adapter/rag_augur"
	rag_http "rag-orchestrator/internal/adapter/rag_http"
	"rag-orchestrator/internal/adapter/repository"
	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/infra/config"
	"rag-orchestrator/internal/infra/httpclient"
	"rag-orchestrator/internal/usecase"
	"rag-orchestrator/internal/worker"
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
	ArticleClient domain.ArticleClient
	EmbeddingModel string
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
	answerUsecase := usecase.NewAnswerWithRAGUsecase(
		retrieveUsecase, promptBuilder, generator, usecase.NewOutputValidator(),
		cfg.RAG.MaxChunks, cfg.RAG.MaxTokens, cfg.RAG.MaxPromptTokens,
		cfg.RAG.PromptVersion, cfg.RAG.Locale, log,
		usecase.WithCacheConfig(cfg.Cache.Size, time.Duration(cfg.Cache.TTL)*time.Minute),
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
		ChunkRepo:           chunkRepo,
		DocRepo:             docRepo,
		JobRepo:             jobRepo,
		IndexUsecase:        indexUsecase,
		RetrieveUsecase:     retrieveUsecase,
		AnswerUsecase:       answerUsecase,
		MorningLetterUsecase: morningLetterUsecase,
		Worker:              jobWorker,
		EmbedderFactory:     embedderFactory,
		IndexUsecaseFactory: indexUsecaseFactory,
		ArticleClient:       articleClient,
		EmbeddingModel:      cfg.Embedder.Model,
		EmbedderTimeout:     cfg.Embedder.Timeout,
	}
}
