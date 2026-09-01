package eval

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadProfiles_ValidFile(t *testing.T) {
	profiles, err := LoadProfiles("testdata/profiles.json")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(profiles), 2, "A/B comparison needs at least two profiles")

	baseline, err := profiles.Select("baseline")
	require.NoError(t, err)
	assert.Equal(t, "baseline", baseline.Name)
	assert.NotEmpty(t, baseline.Embedder.Model)
	assert.NotEmpty(t, baseline.AugurAddr)
}

func TestLoadProfiles_MissingFile(t *testing.T) {
	_, err := LoadProfiles(filepath.Join(t.TempDir(), "nope.json"))
	assert.Error(t, err)
}

func TestProfiles_SelectUnknownName(t *testing.T) {
	profiles := Profiles{{Name: "baseline"}}
	_, err := profiles.Select("candidate")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "candidate")
	assert.Contains(t, err.Error(), "baseline", "error should list the available names")
}

func TestProfile_Validate(t *testing.T) {
	valid := Profile{
		Name:      "baseline",
		AugurAddr: "http://localhost:9011",
		Embedder:  EmbedderProfile{Endpoint: "http://localhost:11434", Model: "bge-m3", Dimensions: 1024},
		Rerank:    RerankProfile{Enabled: true, Model: "bge-reranker-v2-m3", TopK: 10},
		Retrieval: RetrievalProfile{VectorLimit: 50, BM25Limit: 50, HybridAlpha: 0.3, RRFK: 60},
	}

	tests := []struct {
		name    string
		mutate  func(*Profile)
		wantErr string
	}{
		{name: "valid", mutate: func(*Profile) {}},
		{name: "missing name", mutate: func(p *Profile) { p.Name = "" }, wantErr: "name"},
		{name: "missing augur addr", mutate: func(p *Profile) { p.AugurAddr = "" }, wantErr: "augur_addr"},
		{name: "missing embedder model", mutate: func(p *Profile) { p.Embedder.Model = "" }, wantErr: "embedder.model"},
		{name: "missing embedder endpoint", mutate: func(p *Profile) { p.Embedder.Endpoint = "" }, wantErr: "embedder.endpoint"},
		{name: "zero dimensions", mutate: func(p *Profile) { p.Embedder.Dimensions = 0 }, wantErr: "embedder.dimensions"},
		{name: "rerank enabled without model", mutate: func(p *Profile) { p.Rerank.Model = "" }, wantErr: "rerank.model"},
		{name: "rerank disabled without model", mutate: func(p *Profile) { p.Rerank.Enabled = false; p.Rerank.Model = ""; p.Rerank.TopK = 0 }},
		{name: "alpha out of range", mutate: func(p *Profile) { p.Retrieval.HybridAlpha = 1.5 }, wantErr: "retrieval.hybrid_alpha"},
		{name: "zero vector limit", mutate: func(p *Profile) { p.Retrieval.VectorLimit = 0 }, wantErr: "retrieval.vector_limit"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := valid
			tt.mutate(&p)
			err := p.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestProfile_Summary(t *testing.T) {
	p := Profile{
		Name:      "candidate",
		Embedder:  EmbedderProfile{Endpoint: "http://localhost:11435", Model: "qwen3-embedding", Dimensions: 1024},
		Rerank:    RerankProfile{Enabled: false},
		Retrieval: RetrievalProfile{VectorLimit: 50, BM25Limit: 50, HybridAlpha: 0.3, RRFK: 60},
	}
	s := p.Summary()
	assert.Equal(t, "candidate", s.Name)
	assert.Equal(t, "qwen3-embedding", s.EmbedderModel)
	assert.False(t, s.RerankEnabled)
}
