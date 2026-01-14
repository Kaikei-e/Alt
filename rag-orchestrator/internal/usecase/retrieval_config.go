package usecase

import (
	"fmt"
	"time"
)

// RerankingConfig holds settings for cross-encoder reranking.
// Research basis:
// - Pinecone: +15-30% NDCG@10 improvement
// - ZeroEntropy: -35% LLM hallucinations
// - Recommended: Rerank 50 candidates to top 10
type RerankingConfig struct {
	// Enabled controls whether reranking is applied.
	Enabled bool
	// TopK is the number of results to return after reranking.
	TopK int
	// Timeout is the maximum duration for reranking requests.
	Timeout time.Duration
}

// DefaultRerankingConfig returns research-backed defaults.
func DefaultRerankingConfig() RerankingConfig {
	return RerankingConfig{
		Enabled: true, // Default enabled per user preference
		TopK:    10,   // Rerank 50 -> 10
		Timeout: 30 * time.Second,
	}
}

// Validate checks if the reranking configuration is valid.
func (c RerankingConfig) Validate() error {
	if c.Enabled {
		if c.TopK <= 0 {
			return fmt.Errorf("reranking topK must be positive, got %d", c.TopK)
		}
		if c.Timeout <= 0 {
			return fmt.Errorf("reranking timeout must be positive, got %v", c.Timeout)
		}
	}
	return nil
}

// HybridSearchConfig holds settings for BM25+vector hybrid search.
// Research basis:
// - EMNLP 2024: Alpha=0.3 optimal
// - Weaviate/LlamaIndex: RRF fusion with k=60 is best starting point
// - IBM Research: 3-way hybrid (BM25+dense+sparse) +48% improvement
type HybridSearchConfig struct {
	// Enabled controls whether hybrid search is applied.
	Enabled bool
	// Alpha controls the weight between BM25 (0.0) and vector (1.0) search.
	// Research recommends 0.3 (slightly BM25-heavy).
	Alpha float64
	// BM25Limit is the number of BM25 results to fetch for fusion.
	BM25Limit int
}

// DefaultHybridSearchConfig returns research-backed defaults.
func DefaultHybridSearchConfig() HybridSearchConfig {
	return HybridSearchConfig{
		Enabled:   true, // Default enabled per user preference
		Alpha:     0.3,  // EMNLP 2024 optimal
		BM25Limit: 50,   // Match vector search limit
	}
}

// Validate checks if the hybrid search configuration is valid.
func (c HybridSearchConfig) Validate() error {
	if c.Enabled {
		if c.Alpha < 0.0 || c.Alpha > 1.0 {
			return fmt.Errorf("hybrid alpha must be in [0.0, 1.0], got %f", c.Alpha)
		}
		if c.BM25Limit <= 0 {
			return fmt.Errorf("hybrid BM25Limit must be positive, got %d", c.BM25Limit)
		}
	}
	return nil
}

// RetrievalConfig holds tunable parameters for RAG retrieval.
// Default values are based on research findings:
// - EMNLP 2024: "Searching for Best Practices in RAG"
// - Microsoft RAG Techniques Guide
// - Databricks Long Context RAG Performance
type RetrievalConfig struct {
	// SearchLimit is the number of candidates to fetch from vector search
	// before applying quota filtering. Standard value is 50 for re-ranking pool.
	SearchLimit int

	// QuotaOriginal is the number of chunks to select from the original query results.
	// Research suggests 5-10 total chunks is optimal; beyond 20 degrades performance.
	QuotaOriginal int

	// QuotaExpanded is the number of chunks to select from expanded query results.
	// Combined with QuotaOriginal should stay within 5-10 range for optimal results.
	QuotaExpanded int

	// RRFK is the Reciprocal Rank Fusion constant.
	// Standard value is 60.0.
	RRFK float64

	// Reranking holds cross-encoder reranking settings.
	Reranking RerankingConfig

	// HybridSearch holds BM25+vector fusion settings.
	HybridSearch HybridSearchConfig
}

// DefaultRetrievalConfig returns research-backed defaults.
// These values are validated against:
// - EMNLP 2024 findings: 5-10 chunks optimal, >20 degrades accuracy
// - Microsoft RAG Guide: 50 for pre-ranking pool, re-rank to top 10
func DefaultRetrievalConfig() RetrievalConfig {
	return RetrievalConfig{
		SearchLimit:   50,                          // Standard for pre-ranking pool
		QuotaOriginal: 5,                           // 5-10 range optimal
		QuotaExpanded: 5,                           // 5-10 range optimal
		RRFK:          60.0,                        // Standard RRF constant
		Reranking:     DefaultRerankingConfig(),    // Cross-encoder reranking
		HybridSearch:  DefaultHybridSearchConfig(), // BM25+vector fusion
	}
}

// TotalQuota returns the total number of chunks to pass to LLM.
func (c RetrievalConfig) TotalQuota() int {
	return c.QuotaOriginal + c.QuotaExpanded
}

// Validate checks if the configuration values are within acceptable ranges.
func (c RetrievalConfig) Validate() error {
	if c.SearchLimit <= 0 {
		return fmt.Errorf("searchLimit must be positive, got %d", c.SearchLimit)
	}
	if c.QuotaOriginal < 0 {
		return fmt.Errorf("quotaOriginal must be non-negative, got %d", c.QuotaOriginal)
	}
	if c.QuotaExpanded < 0 {
		return fmt.Errorf("quotaExpanded must be non-negative, got %d", c.QuotaExpanded)
	}
	if c.TotalQuota() > 20 {
		return fmt.Errorf("total quota (%d) exceeds recommended maximum of 20 (research shows degradation beyond this)", c.TotalQuota())
	}
	if err := c.Reranking.Validate(); err != nil {
		return fmt.Errorf("reranking config invalid: %w", err)
	}
	if err := c.HybridSearch.Validate(); err != nil {
		return fmt.Errorf("hybrid search config invalid: %w", err)
	}
	return nil
}
