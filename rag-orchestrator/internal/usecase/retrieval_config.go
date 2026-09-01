package usecase

import (
	"fmt"
	"time"
)

// RerankingConfig holds settings for cross-encoder reranking.
// Research basis:
// - Pinecone: +15-30% NDCG@10 improvement
// - ZeroEntropy: -35% LLM hallucinations
type RerankingConfig struct {
	// Enabled controls whether reranking is applied.
	Enabled bool
	// TopK is the number of results to return after reranking.
	TopK int
	// MaxCandidates is the number of results sent to the cross-encoder. It
	// must exceed TopK, or the stage can only reorder hits retrieval already
	// ranked highest and can never promote one from below the cut.
	MaxCandidates int
	// Timeout is the maximum duration for reranking requests.
	Timeout time.Duration
}

// DefaultRerankingConfig returns research-backed defaults.
func DefaultRerankingConfig() RerankingConfig {
	return RerankingConfig{
		Enabled:       true, // Default enabled per user preference
		TopK:          10,
		MaxCandidates: 40,
		// Above the rerank server's own 10s budget, so the client never gives
		// up on a request the server would still have answered.
		Timeout: 15 * time.Second,
	}
}

// Validate checks if the reranking configuration is valid.
func (c RerankingConfig) Validate() error {
	if c.Enabled {
		if c.TopK <= 0 {
			return fmt.Errorf("reranking topK must be positive, got %d", c.TopK)
		}
		if c.MaxCandidates <= 0 {
			return fmt.Errorf("reranking maxCandidates must be positive, got %d", c.MaxCandidates)
		}
		if c.MaxCandidates < c.TopK {
			return fmt.Errorf("reranking maxCandidates (%d) must be at least topK (%d), or hits the cross-encoder scored are dropped before they can be returned", c.MaxCandidates, c.TopK)
		}
		if c.Timeout <= 0 {
			return fmt.Errorf("reranking timeout must be positive, got %v", c.Timeout)
		}
	}
	return nil
}

// HybridSearchConfig holds settings for BM25+vector hybrid search.
// Research basis:
// - Weaviate/LlamaIndex: RRF fusion with k=60 is best starting point
// - IBM Research: 3-way hybrid (BM25+dense+sparse) +48% improvement
//
// Fusion is rank-based (RRF, constant RRFK), not weighted: there is no arm
// weight to configure.
type HybridSearchConfig struct {
	// Enabled controls whether hybrid search is applied.
	Enabled bool
	// BM25Limit is the number of BM25 results to fetch for fusion.
	BM25Limit int
}

// DefaultHybridSearchConfig returns research-backed defaults.
func DefaultHybridSearchConfig() HybridSearchConfig {
	return HybridSearchConfig{
		Enabled:   true, // Default enabled per user preference
		BM25Limit: 50,   // Match vector search limit
	}
}

// Validate checks if the hybrid search configuration is valid.
func (c HybridSearchConfig) Validate() error {
	if c.Enabled && c.BM25Limit <= 0 {
		return fmt.Errorf("hybrid BM25Limit must be positive, got %d", c.BM25Limit)
	}
	return nil
}

// LanguageAllocationConfig holds settings for dynamic language allocation.
// When enabled, selects top N chunks by score regardless of language.
// When disabled, uses legacy English-first prioritization.
type LanguageAllocationConfig struct {
	// Enabled controls whether dynamic score-based allocation is used.
	// When true: selects top N by score regardless of language (JA/EN ratio varies dynamically)
	// When false: uses legacy two-pass approach (English first, then Japanese)
	Enabled bool
}

// DefaultLanguageAllocationConfig returns the default config.
func DefaultLanguageAllocationConfig() LanguageAllocationConfig {
	return LanguageAllocationConfig{
		Enabled: true, // Use dynamic score-based allocation by default
	}
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

	// LanguageAllocation holds settings for dynamic JA/EN language allocation.
	LanguageAllocation LanguageAllocationConfig
}

// DefaultRetrievalConfig returns research-backed defaults.
// These values are validated against:
// - EMNLP 2024 findings: 5-10 chunks optimal, >20 degrades accuracy
// - Microsoft RAG Guide: 50 for pre-ranking pool, re-rank to top 10
func DefaultRetrievalConfig() RetrievalConfig {
	return RetrievalConfig{
		SearchLimit:        50,                                // Standard for pre-ranking pool
		QuotaOriginal:      5,                                 // 5-10 range optimal
		QuotaExpanded:      5,                                 // 5-10 range optimal
		RRFK:               60.0,                              // Standard RRF constant
		Reranking:          DefaultRerankingConfig(),          // Cross-encoder reranking
		HybridSearch:       DefaultHybridSearchConfig(),       // BM25+vector fusion
		LanguageAllocation: DefaultLanguageAllocationConfig(), // Dynamic JA/EN allocation
	}
}

// applyRetrievalConfigDefaults fills zero-valued fields from DefaultRetrievalConfig
// without replacing fields the caller already set.
func applyRetrievalConfigDefaults(cfg RetrievalConfig) RetrievalConfig {
	def := DefaultRetrievalConfig()
	if cfg.SearchLimit == 0 {
		cfg.SearchLimit = def.SearchLimit
	}
	if cfg.QuotaOriginal == 0 {
		cfg.QuotaOriginal = def.QuotaOriginal
	}
	if cfg.QuotaExpanded == 0 {
		cfg.QuotaExpanded = def.QuotaExpanded
	}
	if cfg.RRFK == 0 {
		cfg.RRFK = def.RRFK
	}
	if cfg.Reranking.TopK == 0 {
		cfg.Reranking.TopK = def.Reranking.TopK
	}
	if cfg.Reranking.MaxCandidates == 0 {
		cfg.Reranking.MaxCandidates = def.Reranking.MaxCandidates
	}
	if cfg.Reranking.Timeout == 0 {
		cfg.Reranking.Timeout = def.Reranking.Timeout
	}
	if cfg.HybridSearch.BM25Limit == 0 {
		cfg.HybridSearch.BM25Limit = def.HybridSearch.BM25Limit
	}
	return cfg
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
