package retrieval

import (
	"rag-orchestrator/internal/domain"

	"github.com/google/uuid"
)

// StageContext carries data between pipeline stages.
type StageContext struct {
	// Input
	RetrievalID         string
	Query               string
	CandidateArticleIDs []string
	ConversationHistory []domain.Message // Recent turns for multi-turn query rewriting
	PlannerQueries      []string         // Pre-filtered queries from query planner (skip expand-query when set)

	// Stage 1 outputs
	OriginalEmbedding []float32
	ExpandedQueries   []string
	TagQueries        []string

	// Stage 2 outputs
	AdditionalQueries    []string
	AdditionalEmbeddings [][]float32
	OriginalResults      []domain.SearchResult
	BM25Results          []domain.BM25SearchResult

	// Stage 3 outputs
	HitsOriginal []domain.SearchResult
	HitsExpanded []ContextItem

	// Stage 4 metadata
	RerankApplied bool // true if reranking was successfully applied

	// Config values (set once at init)
	SearchLimit   int
	RRFK          float64
	QuotaOriginal int
	QuotaExpanded int
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
	// document. Carried through the pipeline so Augur can build kind=ARTICLE
	// citations without falling back to a UUID-in-URL guess.
	ArticleID string
}
