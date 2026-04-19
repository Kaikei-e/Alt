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

// HybridConfig captures the two Meilisearch hybrid-search fields.
//
// Embedder is the name of an embedder registered on the index (e.g.
// "qwen3"). An empty string disables hybrid mode.
//
// SemanticRatio is clamped to [0.0, 1.0]. 0.0 ≈ BM25-only, 1.0 ≈ vector-only,
// 0.5 is the balanced default we ship with ADR-000778.
type HybridConfig struct {
	Embedder      string
	SemanticRatio float64
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
