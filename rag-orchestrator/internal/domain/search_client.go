package domain

import "context"

// SearchHit represents a single hit from the search engine.
type SearchHit struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}

// SearchClient defines the interface for searching external indices (e.g. Meilisearch).
type SearchClient interface {
	Search(ctx context.Context, query string) ([]SearchHit, error)
}

// BM25SearchResult represents a BM25 (keyword) search result with ranking info.
// Used for hybrid search fusion with vector search results.
type BM25SearchResult struct {
	// ArticleID is the unique identifier for the article.
	ArticleID string
	// ChunkID is the unique identifier for the chunk (if available).
	ChunkID string
	// Content is the text content of the chunk.
	Content string
	// Title is the article title.
	Title string
	// URL is the article URL.
	URL string
	// Rank is the position in BM25 results (1-indexed for RRF calculation).
	Rank int
	// Score is the BM25 relevance score (optional, for debugging).
	Score float32
}

// BM25Searcher defines the interface for keyword-based BM25 search.
// This is typically backed by Meilisearch or similar full-text search engines.
//
// Research basis:
// - EMNLP 2024: Hybrid search with alpha=0.3 outperforms pure vector search
// - IBM Research: 3-way hybrid (BM25+dense+sparse) provides +48% improvement
type BM25Searcher interface {
	// SearchBM25 performs keyword search and returns ranked results.
	// Results are sorted by BM25 relevance score (highest first).
	SearchBM25(ctx context.Context, query string, limit int) ([]BM25SearchResult, error)
}
