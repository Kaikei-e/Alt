package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Profile names one candidate stack. Two profiles run over the same golden set
// produce two reports that ComputeDiff can put side by side, so "which embedder
// wins" is answered with numbers instead of impressions.
//
// The profile describes the stack a run was executed against; it does not
// reconfigure a running Augur. Point AugurAddr at an instance already started
// with the matching embedder / reranker settings, so the report records what
// actually served the run.
type Profile struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	AugurAddr   string           `json:"augur_addr"`
	Embedder    EmbedderProfile  `json:"embedder"`
	Rerank      RerankProfile    `json:"rerank"`
	Retrieval   RetrievalProfile `json:"retrieval"`
}

// EmbedderProfile identifies the embedding stack under test.
type EmbedderProfile struct {
	Endpoint   string `json:"endpoint"`
	Model      string `json:"model"`
	Dimensions int    `json:"dimensions"`
}

// RerankProfile identifies the cross-encoder stage under test.
type RerankProfile struct {
	Enabled  bool   `json:"enabled"`
	Endpoint string `json:"endpoint,omitempty"`
	Model    string `json:"model,omitempty"`
	TopK     int    `json:"top_k,omitempty"`
}

// RetrievalProfile holds the fusion parameters under test.
type RetrievalProfile struct {
	VectorLimit int     `json:"vector_limit"`
	BM25Limit   int     `json:"bm25_limit"`
	HybridAlpha float64 `json:"hybrid_alpha"`
	RRFK        int     `json:"rrf_k"`
}

// ProfileSummary is the slice of a profile that is stamped into a report, so a
// saved report always says which stack produced it.
type ProfileSummary struct {
	Name          string  `json:"name"`
	EmbedderModel string  `json:"embedder_model"`
	EmbedderDims  int     `json:"embedder_dimensions"`
	RerankEnabled bool    `json:"rerank_enabled"`
	RerankModel   string  `json:"rerank_model,omitempty"`
	HybridAlpha   float64 `json:"hybrid_alpha"`
}

// Summary reduces the profile to what a report needs to identify the run.
func (p Profile) Summary() ProfileSummary {
	return ProfileSummary{
		Name:          p.Name,
		EmbedderModel: p.Embedder.Model,
		EmbedderDims:  p.Embedder.Dimensions,
		RerankEnabled: p.Rerank.Enabled,
		RerankModel:   p.Rerank.Model,
		HybridAlpha:   p.Retrieval.HybridAlpha,
	}
}

// Validate rejects an under-specified profile. A run whose stack is only
// half-declared produces a report nobody can reproduce, so this fails loudly
// instead of filling in defaults.
func (p Profile) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("profile: name is required")
	}
	if strings.TrimSpace(p.AugurAddr) == "" {
		return fmt.Errorf("profile %q: augur_addr is required", p.Name)
	}
	if strings.TrimSpace(p.Embedder.Endpoint) == "" {
		return fmt.Errorf("profile %q: embedder.endpoint is required", p.Name)
	}
	if strings.TrimSpace(p.Embedder.Model) == "" {
		return fmt.Errorf("profile %q: embedder.model is required", p.Name)
	}
	if p.Embedder.Dimensions <= 0 {
		return fmt.Errorf("profile %q: embedder.dimensions must be positive, got %d", p.Name, p.Embedder.Dimensions)
	}
	if p.Rerank.Enabled {
		if strings.TrimSpace(p.Rerank.Model) == "" {
			return fmt.Errorf("profile %q: rerank.model is required when rerank is enabled", p.Name)
		}
		if p.Rerank.TopK <= 0 {
			return fmt.Errorf("profile %q: rerank.top_k must be positive when rerank is enabled, got %d", p.Name, p.Rerank.TopK)
		}
	}
	if p.Retrieval.VectorLimit <= 0 {
		return fmt.Errorf("profile %q: retrieval.vector_limit must be positive, got %d", p.Name, p.Retrieval.VectorLimit)
	}
	if p.Retrieval.BM25Limit <= 0 {
		return fmt.Errorf("profile %q: retrieval.bm25_limit must be positive, got %d", p.Name, p.Retrieval.BM25Limit)
	}
	if p.Retrieval.HybridAlpha < 0.0 || p.Retrieval.HybridAlpha > 1.0 {
		return fmt.Errorf("profile %q: retrieval.hybrid_alpha must be in [0.0, 1.0], got %f", p.Name, p.Retrieval.HybridAlpha)
	}
	if p.Retrieval.RRFK <= 0 {
		return fmt.Errorf("profile %q: retrieval.rrf_k must be positive, got %d", p.Name, p.Retrieval.RRFK)
	}
	return nil
}

// Profiles is the set of stacks declared in the profile file.
type Profiles []Profile

// LoadProfiles reads and validates the profile file.
func LoadProfiles(path string) (Profiles, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- operator-supplied CLI path for a local eval tool
	if err != nil {
		return nil, fmt.Errorf("read profiles: %w", err)
	}
	var profiles Profiles
	if err := json.Unmarshal(data, &profiles); err != nil {
		return nil, fmt.Errorf("parse profiles: %w", err)
	}
	if len(profiles) == 0 {
		return nil, fmt.Errorf("parse profiles: %s declares no profiles", path)
	}
	seen := make(map[string]bool, len(profiles))
	for _, p := range profiles {
		if err := p.Validate(); err != nil {
			return nil, fmt.Errorf("validate profiles: %w", err)
		}
		if seen[p.Name] {
			return nil, fmt.Errorf("validate profiles: duplicate profile name %q", p.Name)
		}
		seen[p.Name] = true
	}
	return profiles, nil
}

// Select returns the named profile, or an error listing what is available.
func (ps Profiles) Select(name string) (Profile, error) {
	for _, p := range ps {
		if p.Name == name {
			return p, nil
		}
	}
	return Profile{}, fmt.Errorf("unknown profile %q; available: %s", name, strings.Join(ps.Names(), ", "))
}

// Names lists the declared profile names in file order.
func (ps Profiles) Names() []string {
	names := make([]string, 0, len(ps))
	for _, p := range ps {
		names = append(names, p.Name)
	}
	return names
}
