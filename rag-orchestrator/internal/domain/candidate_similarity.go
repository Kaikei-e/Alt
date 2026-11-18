package domain

import "strings"

// MaxCandidateSimilarityArticles bounds the number of candidate article IDs
// accepted in a single candidate-set similarity request.
const MaxCandidateSimilarityArticles = 2000

// SimilarityRepresentation names the embedding representation used to score
// similarity between a seed article and candidate articles.
type SimilarityRepresentation string

const (
	SimilarityRepFirstChunk       SimilarityRepresentation = "first_chunk"
	SimilarityRepChunkMean        SimilarityRepresentation = "chunk_mean"
	SimilarityRepNearestChunkPair SimilarityRepresentation = "nearest_chunk_pair"
)

// AllSimilarityRepresentations lists every representation backed directly by rag-db.
var AllSimilarityRepresentations = []SimilarityRepresentation{
	SimilarityRepFirstChunk,
	SimilarityRepChunkMean,
	SimilarityRepNearestChunkPair,
}

// ParseSimilarityRepresentation parses and validates a representation name.
func ParseSimilarityRepresentation(raw string) (SimilarityRepresentation, bool) {
	rep := SimilarityRepresentation(strings.TrimSpace(raw))
	switch rep {
	case SimilarityRepFirstChunk, SimilarityRepChunkMean, SimilarityRepNearestChunkPair:
		return rep, true
	default:
		return "", false
	}
}

// IsValid reports whether r is a supported database representation.
func (r SimilarityRepresentation) IsValid() bool {
	_, ok := ParseSimilarityRepresentation(string(r))
	return ok
}

// CandidateSimilarityItem is one scored candidate article.
type CandidateSimilarityItem struct {
	ArticleID  string
	Similarity float64
}

// CandidateSimilarityResult holds the ranked candidate similarities and
// indexing diagnostics for a single seed evaluation.
type CandidateSimilarityResult struct {
	Results                    []CandidateSimilarityItem
	SeedIndexed                bool
	CandidatesWithoutEmbedding int
}

// RagDocumentMetadata holds display metadata stored in rag_document_versions.
type RagDocumentMetadata struct {
	ArticleID string
	Title     string
	URL       string
}
