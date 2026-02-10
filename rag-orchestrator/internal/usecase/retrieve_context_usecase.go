package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase/retrieval"

	"github.com/google/uuid"
)

// RetrieveContextInput defines the input parameters for RetrieveContext.
type RetrieveContextInput struct {
	Query               string
	CandidateArticleIDs []string
}

// RetrieveContextOutput defines the output for RetrieveContext.
type RetrieveContextOutput struct {
	Contexts        []ContextItem
	ExpandedQueries []string
}

// ContextItem represents a single retrieved chunk with metadata.
type ContextItem struct {
	ChunkText       string
	URL             string
	Title           string
	PublishedAt     string // ISO8601 string
	Score           float32
	DocumentVersion int
	ChunkID         uuid.UUID
}

// RetrieveContextUsecase defines the interface for retrieving context.
type RetrieveContextUsecase interface {
	Execute(ctx context.Context, input RetrieveContextInput) (*RetrieveContextOutput, error)
}

type retrieveContextUsecase struct {
	chunkRepo     domain.RagChunkRepository
	docRepo       domain.RagDocumentRepository
	encoder       domain.VectorEncoder
	llmClient     domain.LLMClient
	searchClient  domain.SearchClient
	queryExpander domain.QueryExpander
	reranker      domain.Reranker     // Optional: cross-encoder reranking
	bm25Searcher  domain.BM25Searcher // Optional: BM25 search for hybrid fusion
	config        RetrievalConfig
	logger        *slog.Logger
}

// RetrieveContextOption is a functional option for configuring the usecase.
type RetrieveContextOption func(*retrieveContextUsecase)

// WithReranker sets an optional cross-encoder reranker.
// If not set or nil, reranking is skipped.
func WithReranker(r domain.Reranker) RetrieveContextOption {
	return func(u *retrieveContextUsecase) {
		u.reranker = r
	}
}

// WithBM25Searcher sets an optional BM25 searcher for hybrid search fusion.
// If not set or nil, pure vector search is used.
func WithBM25Searcher(s domain.BM25Searcher) RetrieveContextOption {
	return func(u *retrieveContextUsecase) {
		u.bm25Searcher = s
	}
}

// NewRetrieveContextUsecase creates a new RetrieveContextUsecase.
// If config is zero-valued, defaults are used (research-backed values).
func NewRetrieveContextUsecase(
	chunkRepo domain.RagChunkRepository,
	docRepo domain.RagDocumentRepository,
	encoder domain.VectorEncoder,
	llmClient domain.LLMClient,
	searchClient domain.SearchClient,
	queryExpander domain.QueryExpander,
	config RetrievalConfig,
	logger *slog.Logger,
	opts ...RetrieveContextOption,
) RetrieveContextUsecase {
	// Apply defaults if config is zero-valued
	if config.SearchLimit == 0 {
		config = DefaultRetrievalConfig()
	}
	u := &retrieveContextUsecase{
		chunkRepo:     chunkRepo,
		docRepo:       docRepo,
		encoder:       encoder,
		llmClient:     llmClient,
		searchClient:  searchClient,
		queryExpander: queryExpander,
		config:        config,
		logger:        logger,
	}
	for _, opt := range opts {
		opt(u)
	}
	return u
}

func (u *retrieveContextUsecase) Execute(ctx context.Context, input RetrieveContextInput) (*RetrieveContextOutput, error) {
	if input.Query == "" {
		return nil, fmt.Errorf("query is empty")
	}

	retrievalStart := time.Now()
	retrievalID := uuid.NewString()
	u.logger.Info("retrieval_started",
		slog.String("retrieval_id", retrievalID),
		slog.String("query", input.Query),
		slog.Int("candidate_articles", len(input.CandidateArticleIDs)))

	// Initialize StageContext
	sc := &retrieval.StageContext{
		RetrievalID:         retrievalID,
		Query:               input.Query,
		CandidateArticleIDs: input.CandidateArticleIDs,
		SearchLimit:         u.config.SearchLimit,
		RRFK:                u.config.RRFK,
		QuotaOriginal:       u.config.QuotaOriginal,
		QuotaExpanded:       u.config.QuotaExpanded,
	}

	// Stage 1: Query expansion + tag search + embedding (parallel)
	if err := retrieval.ExpandQueries(ctx, sc, u.queryExpander, u.llmClient, u.searchClient, u.encoder, u.logger); err != nil {
		return nil, err
	}

	stage1Duration := time.Since(retrievalStart)
	u.logger.Info("retrieval_stage1_completed",
		slog.String("retrieval_id", retrievalID),
		slog.Int("expanded_queries", len(sc.ExpandedQueries)),
		slog.Int("tag_queries", len(sc.TagQueries)),
		slog.Int64("duration_ms", stage1Duration.Milliseconds()))

	// Stage 2: BM25 search + original vector search + expanded embedding (parallel)
	if err := retrieval.EmbedAndSearch(ctx, sc, u.encoder, u.bm25Searcher, u.chunkRepo,
		u.config.HybridSearch.Enabled, u.config.HybridSearch.BM25Limit, u.logger); err != nil {
		return nil, err
	}

	stage2Duration := time.Since(retrievalStart) - stage1Duration
	u.logger.Info("retrieval_stage2_completed",
		slog.String("retrieval_id", retrievalID),
		slog.Int("original_results", len(sc.OriginalResults)),
		slog.Int("additional_embeddings", len(sc.AdditionalEmbeddings)),
		slog.Int("bm25_results", len(sc.BM25Results)),
		slog.Int64("duration_ms", stage2Duration.Milliseconds()))

	// Stage 3: Parallel vector search for expanded queries + RRF fusion
	if err := retrieval.FuseResults(ctx, sc, u.chunkRepo, u.logger); err != nil {
		return nil, err
	}

	// Stage 4: Cross-encoder reranking
	retrieval.Rerank(ctx, sc, u.reranker, retrieval.RerankConfig{
		Enabled: u.config.Reranking.Enabled,
		TopK:    u.config.Reranking.TopK,
		Timeout: u.config.Reranking.Timeout,
	}, u.logger)

	// Stage 5: Language allocation + quota selection
	allocatedCtxs := retrieval.Allocate(sc, retrieval.AllocateConfig{
		DynamicLanguageAllocationEnabled: u.config.LanguageAllocation.Enabled,
	}, u.logger)

	// Convert retrieval.ContextItem to usecase.ContextItem
	contexts := convertContextItems(allocatedCtxs)

	u.logger.Info("vector_search_completed",
		slog.String("retrieval_id", retrievalID),
		slog.Int("original_hits", len(sc.HitsOriginal)),
		slog.Int("expanded_hits_unique", len(sc.HitsExpanded)),
		slog.Int("final_contexts", len(contexts)))

	retrievalDuration := time.Since(retrievalStart)
	u.logger.Info("retrieval_completed",
		slog.String("retrieval_id", retrievalID),
		slog.Int("contexts_returned", len(contexts)),
		slog.Int64("duration_ms", retrievalDuration.Milliseconds()))

	var expandedQueriesRet []string
	if len(sc.ExpandedQueries) > 0 {
		expandedQueriesRet = sc.ExpandedQueries
	}

	return &RetrieveContextOutput{
		Contexts:        contexts,
		ExpandedQueries: expandedQueriesRet,
	}, nil
}

// SelectContextsDynamic is a pass-through to retrieval.SelectContextsDynamic for backward compatibility.
func SelectContextsDynamic(hitsOriginal []domain.SearchResult, hitsExpanded []ContextItem, totalQuota int) []ContextItem {
	// Convert usecase.ContextItem to retrieval.ContextItem
	rExpanded := make([]retrieval.ContextItem, len(hitsExpanded))
	for i, item := range hitsExpanded {
		rExpanded[i] = retrieval.ContextItem{
			ChunkText:       item.ChunkText,
			URL:             item.URL,
			Title:           item.Title,
			PublishedAt:     item.PublishedAt,
			Score:           item.Score,
			DocumentVersion: item.DocumentVersion,
			ChunkID:         item.ChunkID,
		}
	}

	result := retrieval.SelectContextsDynamic(hitsOriginal, rExpanded, totalQuota)
	return convertContextItems(result)
}

func convertContextItems(items []retrieval.ContextItem) []ContextItem {
	result := make([]ContextItem, len(items))
	for i, item := range items {
		result[i] = ContextItem{
			ChunkText:       item.ChunkText,
			URL:             item.URL,
			Title:           item.Title,
			PublishedAt:     item.PublishedAt,
			Score:           item.Score,
			DocumentVersion: item.DocumentVersion,
			ChunkID:         item.ChunkID,
		}
	}
	return result
}
