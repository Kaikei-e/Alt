package retrieval

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"rag-orchestrator/internal/domain"

	"github.com/google/uuid"
)

// GraphInput defines the input parameters for RetrievalGraph.Execute.
type GraphInput struct {
	Query               string
	CandidateArticleIDs []string
	ConversationHistory []domain.Message
}

// GraphOutput defines the output of RetrievalGraph.Execute.
type GraphOutput struct {
	Contexts        []ContextItem
	ExpandedQueries []string
	BM25HitCount    int
}

// GraphConfig holds the retrieval parameters needed by the graph.
// These mirror the fields from usecase.RetrievalConfig that stages consume.
type GraphConfig struct {
	SearchLimit                      int
	RRFK                             float64
	QuotaOriginal                    int
	QuotaExpanded                    int
	RerankEnabled                    bool
	RerankTopK                       int
	RerankTimeout                    time.Duration
	HybridSearchEnabled              bool
	BM25Limit                        int
	DynamicLanguageAllocationEnabled bool
}

// GraphDeps collects all dependencies needed by the 5-stage retrieval pipeline.
type GraphDeps struct {
	QueryExpander domain.QueryExpander
	LLMClient     domain.LLMClient
	SearchClient  domain.SearchClient
	Encoder       domain.VectorEncoder
	Reranker      domain.Reranker     // Optional: cross-encoder reranking
	BM25Searcher  domain.BM25Searcher // Optional: BM25 search for hybrid fusion
	ChunkRepo     domain.RagChunkRepository
	Config        GraphConfig
	Logger        *slog.Logger
}

// RetrievalGraph wraps the 5-stage retrieval pipeline as a single callable unit.
// It delegates to the existing package-level stage functions (ExpandQueries, EmbedAndSearch,
// FuseResults, Rerank, Allocate) without replacing them.
//
// This is the first step toward Eino compose.Graph integration (Phase 4).
// The graph can later be registered as a tool with the Eino ChatModelAgent.
type RetrievalGraph struct {
	queryExpander domain.QueryExpander
	llmClient     domain.LLMClient
	searchClient  domain.SearchClient
	encoder       domain.VectorEncoder
	reranker      domain.Reranker
	bm25Searcher  domain.BM25Searcher
	chunkRepo     domain.RagChunkRepository
	config        GraphConfig
	logger        *slog.Logger
}

// NewRetrievalGraph creates a new RetrievalGraph with the given dependencies.
func NewRetrievalGraph(deps GraphDeps) *RetrievalGraph {
	return &RetrievalGraph{
		queryExpander: deps.QueryExpander,
		llmClient:     deps.LLMClient,
		searchClient:  deps.SearchClient,
		encoder:       deps.Encoder,
		reranker:      deps.Reranker,
		bm25Searcher:  deps.BM25Searcher,
		chunkRepo:     deps.ChunkRepo,
		config:        deps.Config,
		logger:        deps.Logger,
	}
}

// Execute runs the full 5-stage retrieval pipeline and returns the results.
func (g *RetrievalGraph) Execute(ctx context.Context, input GraphInput) (*GraphOutput, error) {
	if input.Query == "" {
		return nil, fmt.Errorf("query is empty")
	}

	retrievalStart := time.Now()
	retrievalID := uuid.NewString()
	g.logger.Info("retrieval_graph_started",
		slog.String("retrieval_id", retrievalID),
		slog.String("query", input.Query),
		slog.Int("candidate_articles", len(input.CandidateArticleIDs)))

	// Initialize StageContext
	sc := &StageContext{
		RetrievalID:         retrievalID,
		Query:               input.Query,
		CandidateArticleIDs: input.CandidateArticleIDs,
		ConversationHistory: input.ConversationHistory,
		SearchLimit:         g.config.SearchLimit,
		RRFK:                g.config.RRFK,
		QuotaOriginal:       g.config.QuotaOriginal,
		QuotaExpanded:       g.config.QuotaExpanded,
	}

	// Stage 1: Query expansion + tag search + embedding (parallel)
	if err := ExpandQueries(ctx, sc, g.queryExpander, g.llmClient, g.searchClient, g.encoder, g.logger); err != nil {
		return nil, fmt.Errorf("stage1 expand_queries: %w", err)
	}

	stage1Duration := time.Since(retrievalStart)
	g.logger.Info("retrieval_graph_stage1_completed",
		slog.String("retrieval_id", retrievalID),
		slog.Int("expanded_queries", len(sc.ExpandedQueries)),
		slog.Int64("duration_ms", stage1Duration.Milliseconds()))

	// Stage 2: BM25 search + original vector search + expanded embedding (parallel)
	if err := EmbedAndSearch(ctx, sc, g.encoder, g.bm25Searcher, g.chunkRepo,
		g.config.HybridSearchEnabled, g.config.BM25Limit, g.logger); err != nil {
		return nil, fmt.Errorf("stage2 embed_and_search: %w", err)
	}

	// Stage 3: Parallel vector search for expanded queries + RRF fusion
	if err := FuseResults(ctx, sc, g.chunkRepo, g.logger); err != nil {
		return nil, fmt.Errorf("stage3 fuse_results: %w", err)
	}

	// Stage 4: Cross-encoder reranking
	Rerank(ctx, sc, g.reranker, RerankConfig{
		Enabled: g.config.RerankEnabled,
		TopK:    g.config.RerankTopK,
		Timeout: g.config.RerankTimeout,
	}, g.logger)

	// Stage 5: Language allocation + quota selection
	allocatedCtxs := Allocate(sc, AllocateConfig{
		DynamicLanguageAllocationEnabled: g.config.DynamicLanguageAllocationEnabled,
	}, g.logger)

	retrievalDuration := time.Since(retrievalStart)
	g.logger.Info("retrieval_graph_completed",
		slog.String("retrieval_id", retrievalID),
		slog.Int("contexts_returned", len(allocatedCtxs)),
		slog.Int64("duration_ms", retrievalDuration.Milliseconds()))

	var expandedQueries []string
	if len(sc.ExpandedQueries) > 0 {
		expandedQueries = sc.ExpandedQueries
	}

	return &GraphOutput{
		Contexts:        allocatedCtxs,
		ExpandedQueries: expandedQueries,
		BM25HitCount:    len(sc.BM25Results),
	}, nil
}
