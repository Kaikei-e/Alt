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
	ChunkText   string
	URL         string
	Title       string
	PublishedAt string // ISO8601 string
	Score       float32
	// ScoreKind declares which space Score lives in, so a consumer can tell a
	// cross-encoder judgement from a rank-derived ordering signal.
	ScoreKind       domain.ScoreKind
	RerankScore     float32 // Cross-encoder reranker score (meaningful when RerankApplied)
	RerankApplied   bool    // true when RerankScore was produced by the cross-encoder
	DocumentVersion int
	ChunkID         uuid.UUID
	// ArticleID is the stable alt-db articles.id for this chunk's owning
	// document. Carried through the pipeline so Augur can build kind=ARTICLE
	// citations without falling back to a UUID-in-URL guess.
	ArticleID string
}

// hitIdentity names a retrieval hit for deduplication and rerank bookkeeping.
//
// A chunk id is the natural identity, but BM25 hits are article-level and
// carry none — search_indexer_client.go sets ChunkID: "" because Meilisearch
// indexes articles, not chunks — so every BM25 hit shares uuid.Nil. Keying on
// the chunk id alone therefore collapses a whole BM25-only result set into a
// single hit, which is exactly what the pipeline degrades to whenever vector
// search returns nothing. Falling back to the article id keeps those hits
// apart at the same granularity fuseHybridResults already fuses on.
//
// Returns "" when the hit carries neither id; callers must treat that as
// "cannot be identified" rather than as a shared key.
func hitIdentity(chunkID uuid.UUID, articleID string) string {
	if chunkID != uuid.Nil {
		return "chunk:" + chunkID.String()
	}
	if articleID != "" {
		return "article:" + articleID
	}
	return ""
}
