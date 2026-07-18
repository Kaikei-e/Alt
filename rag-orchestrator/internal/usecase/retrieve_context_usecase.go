package usecase

import (
	"context"
	"log/slog"

	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/usecase/retrieval"

	"github.com/google/uuid"
)

// RetrieveContextInput defines the input parameters for RetrieveContext.
type RetrieveContextInput struct {
	Query               string
	CandidateArticleIDs []string
	ConversationHistory []domain.Message // Recent turns for query rewriting
	SearchQueries       []string         // Pre-filtered queries from query planner (bypass expand-query)
}

// RetrieveContextOutput defines the output for RetrieveContext.
type RetrieveContextOutput struct {
	Contexts          []ContextItem
	ExpandedQueries   []string
	SupplementaryInfo []string // Additional context from tools (recaps, tag clouds, etc.)
	ToolsUsed         []string // Names of tools executed during retrieval
	BM25HitCount      int      // Number of BM25 keyword search results (0 = lexical retrieval failed)
}

// ContextItem represents a single retrieved chunk with metadata.
type ContextItem struct {
	ChunkText       string
	URL             string
	Title           string
	PublishedAt     string // ISO8601 string
	Score           float32
	RerankScore     float32 // Cross-encoder reranker score (meaningful when RerankApplied)
	RerankApplied   bool    // true when RerankScore was produced by the cross-encoder
	DocumentVersion int
	ChunkID         uuid.UUID
	// ArticleID is the stable alt-db articles.id for this chunk's owning
	// document. Required downstream by Augur to emit kind=ARTICLE citations
	// instead of falling back to UNSPECIFIED / disabled links.
	ArticleID string
}

// RetrieveContextUsecase defines the interface for retrieving context.
type RetrieveContextUsecase interface {
	Execute(ctx context.Context, input RetrieveContextInput) (*RetrieveContextOutput, error)
}

type retrieveContextUsecase struct {
	docRepo        domain.RagDocumentRepository
	chunkRepo      domain.RagChunkRepository
	encoder        domain.VectorEncoder
	llmClient      domain.LLMClient
	searchClient   domain.SearchClient
	queryExpander  domain.QueryExpander
	reranker       domain.Reranker       // Optional: cross-encoder reranking
	bm25Searcher   domain.BM25Searcher   // Optional: BM25 search for hybrid fusion
	hybridSearcher domain.HybridSearcher // Optional: in-DB hybrid search (replaces bm25Searcher)
	config         RetrievalConfig
	logger         *slog.Logger
	graph          *retrieval.RetrievalGraph
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

// WithHybridSearcher sets an in-database hybrid searcher (pgvector + tsvector RRF).
// When set, this replaces the separate BM25 + vector search + application-level fusion.
func WithHybridSearcher(h domain.HybridSearcher) RetrieveContextOption {
	return func(u *retrieveContextUsecase) {
		u.hybridSearcher = h
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
	// Fill missing fields per-key so a zero SearchLimit does not discard
	// other explicitly configured values.
	config = applyRetrievalConfigDefaults(config)
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

	// Delegate the 5-stage pipeline to RetrievalGraph instead of duplicating it here.
	u.graph = retrieval.NewRetrievalGraph(retrieval.GraphDeps{
		QueryExpander:  u.queryExpander,
		LLMClient:      u.llmClient,
		SearchClient:   u.searchClient,
		Encoder:        u.encoder,
		Reranker:       u.reranker,
		BM25Searcher:   u.bm25Searcher,
		HybridSearcher: u.hybridSearcher,
		ChunkRepo:      u.chunkRepo,
		Config: retrieval.GraphConfig{
			SearchLimit:                      u.config.SearchLimit,
			RRFK:                             u.config.RRFK,
			QuotaOriginal:                    u.config.QuotaOriginal,
			QuotaExpanded:                    u.config.QuotaExpanded,
			RerankEnabled:                    u.config.Reranking.Enabled,
			RerankTopK:                       u.config.Reranking.TopK,
			RerankTimeout:                    u.config.Reranking.Timeout,
			HybridSearchEnabled:              u.config.HybridSearch.Enabled,
			BM25Limit:                        u.config.HybridSearch.BM25Limit,
			DynamicLanguageAllocationEnabled: u.config.LanguageAllocation.Enabled,
		},
		Logger: u.logger,
	})

	return u
}

func (u *retrieveContextUsecase) Execute(ctx context.Context, input RetrieveContextInput) (*RetrieveContextOutput, error) {
	out, err := u.graph.Execute(ctx, retrieval.GraphInput{
		Query:               input.Query,
		CandidateArticleIDs: input.CandidateArticleIDs,
		ConversationHistory: input.ConversationHistory,
		SearchQueries:       input.SearchQueries,
	})
	if err != nil {
		return nil, err
	}

	return &RetrieveContextOutput{
		Contexts:        convertContextItems(out.Contexts),
		ExpandedQueries: out.ExpandedQueries,
		BM25HitCount:    out.BM25HitCount,
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
			ArticleID:       item.ArticleID,
		}
	}

	result := retrieval.SelectContextsDynamic(hitsOriginal, rExpanded, totalQuota, false)
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
			RerankScore:     item.RerankScore,
			RerankApplied:   item.RerankApplied,
			DocumentVersion: item.DocumentVersion,
			ChunkID:         item.ChunkID,
			ArticleID:       item.ArticleID,
		}
	}
	return result
}
