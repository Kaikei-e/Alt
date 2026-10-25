// Package driver: meilisearch_hybrid.go defines the hybrid-search
// configuration the driver applies to every outbound SearchRequest.
//
// When the driver is constructed without a HybridConfig, search falls back
// to pure BM25 — the existing behaviour before ADR-000778. When configured,
// the driver attaches {embedder, semanticRatio} to each SearchRequest so
// Meilisearch blends dense-vector similarity with BM25 at the configured
// ratio.
package driver

import (
	"github.com/meilisearch/meilisearch-go"
)

// HybridConfig captures the Meilisearch hybrid-search fields plus the
// pure-vector noise cutoff applied by the driver.
//
// Embedder is the name of an embedder registered on the index (e.g.
// "bge-m3"). An empty string disables hybrid mode.
//
// SemanticRatio is clamped to [0.0, 1.0]. 0.0 ≈ BM25-only, 1.0 ≈ vector-only,
// 0.3 keeps BM25 dominant while allowing semantic recall.
//
// MinSemanticScore drops pure-vector hits (no "words" match in
// _rankingScoreDetails) whose _rankingScore falls below this threshold while
// preserving all BM25 lexical matches regardless of score.
type HybridConfig struct {
	Embedder         string
	SemanticRatio    float64
	MinSemanticScore float64
}

// Enabled reports whether the driver should attach hybrid params.
func (c *HybridConfig) Enabled() bool {
	return c != nil && c.Embedder != ""
}

// toSDK converts to the Meilisearch SDK shape, applying the [0,1] clamp.
func (c *HybridConfig) toSDK() *meilisearch.SearchRequestHybrid {
	if !c.Enabled() {
		return nil
	}
	ratio := c.SemanticRatio
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	return &meilisearch.SearchRequestHybrid{
		Embedder:      c.Embedder,
		SemanticRatio: ratio,
	}
}
