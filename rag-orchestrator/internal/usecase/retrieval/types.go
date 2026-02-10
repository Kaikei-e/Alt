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
	DocumentVersion int
	ChunkID         uuid.UUID
}
