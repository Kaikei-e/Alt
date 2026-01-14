package domain

import "context"

// RerankCandidate represents a document candidate for cross-encoder reranking.
type RerankCandidate struct {
	// ID is the unique identifier for the chunk (used to map back results).
	ID string
	// Content is the text content to be scored against the query.
	Content string
	// Score is the initial retrieval score (for debugging/logging).
	Score float32
}

// RerankResult represents a reranked document with cross-encoder relevance score.
type RerankResult struct {
	// ID matches the candidate ID for result mapping.
	ID string
	// Score is the cross-encoder relevance score (typically 0.0 to 1.0).
	Score float32
}

// Reranker defines the interface for cross-encoder reranking.
// Implementation should call an external service (e.g., news-creator /v1/rerank).
//
// Research basis:
// - Pinecone: Two-stage retrieval with cross-encoders improves NDCG@10 by 15-30%
// - ZeroEntropy: Reranking reduces LLM hallucinations by 35%
// - Best practice: Rerank 50 candidates down to 10 for LLM context
type Reranker interface {
	// Rerank scores candidates against the query using a cross-encoder model.
	// Returns results sorted by score descending.
	// If an error occurs, callers should fall back to original scores.
	Rerank(ctx context.Context, query string, candidates []RerankCandidate) ([]RerankResult, error)

	// ModelName returns the model identifier for logging/debugging.
	ModelName() string
}
