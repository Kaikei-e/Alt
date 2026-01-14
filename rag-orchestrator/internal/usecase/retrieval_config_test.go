package usecase

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultRetrievalConfig(t *testing.T) {
	cfg := DefaultRetrievalConfig()

	// Research-backed defaults (EMNLP 2024, Microsoft RAG Guide)
	assert.Equal(t, 50, cfg.SearchLimit, "searchLimit should be 50 (standard re-ranking pool)")
	assert.Equal(t, 5, cfg.QuotaOriginal, "quotaOriginal should be 5")
	assert.Equal(t, 5, cfg.QuotaExpanded, "quotaExpanded should be 5")
	assert.Equal(t, 60.0, cfg.RRFK, "rrfK should be 60.0 (standard RRF constant)")

	// Reranking defaults (Pinecone, ZeroEntropy research)
	assert.True(t, cfg.Reranking.Enabled, "reranking should be enabled by default")
	assert.Equal(t, 10, cfg.Reranking.TopK, "reranking topK should be 10")
	assert.Equal(t, 30*time.Second, cfg.Reranking.Timeout, "reranking timeout should be 30s")

	// Hybrid search defaults (EMNLP 2024, Weaviate)
	assert.True(t, cfg.HybridSearch.Enabled, "hybrid search should be enabled by default")
	assert.Equal(t, 0.3, cfg.HybridSearch.Alpha, "hybrid alpha should be 0.3 (EMNLP 2024 optimal)")
	assert.Equal(t, 50, cfg.HybridSearch.BM25Limit, "hybrid BM25Limit should be 50")

	// Language allocation defaults (dynamic score-based selection)
	assert.True(t, cfg.LanguageAllocation.Enabled, "dynamic language allocation should be enabled by default")
}

func TestRetrievalConfig_TotalQuota(t *testing.T) {
	tests := []struct {
		name     string
		config   RetrievalConfig
		expected int
	}{
		{
			name:     "default config",
			config:   DefaultRetrievalConfig(),
			expected: 10, // 5 + 5 = 10 (within 5-10 optimal range per research)
		},
		{
			name: "custom config",
			config: RetrievalConfig{
				SearchLimit:   100,
				QuotaOriginal: 7,
				QuotaExpanded: 3,
				RRFK:          60.0,
			},
			expected: 10,
		},
		{
			name: "minimal config",
			config: RetrievalConfig{
				QuotaOriginal: 3,
				QuotaExpanded: 2,
			},
			expected: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config.TotalQuota())
		})
	}
}

func TestRerankingConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    RerankingConfig
		wantError bool
	}{
		{
			name:      "valid default config",
			config:    DefaultRerankingConfig(),
			wantError: false,
		},
		{
			name: "valid disabled config",
			config: RerankingConfig{
				Enabled: false,
			},
			wantError: false, // Disabled config should always be valid
		},
		{
			name: "invalid - zero topK when enabled",
			config: RerankingConfig{
				Enabled: true,
				TopK:    0,
				Timeout: 30 * time.Second,
			},
			wantError: true,
		},
		{
			name: "invalid - zero timeout when enabled",
			config: RerankingConfig{
				Enabled: true,
				TopK:    10,
				Timeout: 0,
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHybridSearchConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    HybridSearchConfig
		wantError bool
	}{
		{
			name:      "valid default config",
			config:    DefaultHybridSearchConfig(),
			wantError: false,
		},
		{
			name: "valid disabled config",
			config: HybridSearchConfig{
				Enabled: false,
			},
			wantError: false, // Disabled config should always be valid
		},
		{
			name: "valid edge - alpha=0.0 (pure BM25)",
			config: HybridSearchConfig{
				Enabled:   true,
				Alpha:     0.0,
				BM25Limit: 50,
			},
			wantError: false,
		},
		{
			name: "valid edge - alpha=1.0 (pure vector)",
			config: HybridSearchConfig{
				Enabled:   true,
				Alpha:     1.0,
				BM25Limit: 50,
			},
			wantError: false,
		},
		{
			name: "invalid - alpha < 0",
			config: HybridSearchConfig{
				Enabled:   true,
				Alpha:     -0.1,
				BM25Limit: 50,
			},
			wantError: true,
		},
		{
			name: "invalid - alpha > 1",
			config: HybridSearchConfig{
				Enabled:   true,
				Alpha:     1.1,
				BM25Limit: 50,
			},
			wantError: true,
		},
		{
			name: "invalid - zero BM25Limit when enabled",
			config: HybridSearchConfig{
				Enabled:   true,
				Alpha:     0.3,
				BM25Limit: 0,
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRetrievalConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		config    RetrievalConfig
		wantError bool
	}{
		{
			name:      "valid default config",
			config:    DefaultRetrievalConfig(),
			wantError: false,
		},
		{
			name: "invalid - zero search limit",
			config: RetrievalConfig{
				SearchLimit:   0,
				QuotaOriginal: 5,
				QuotaExpanded: 5,
				RRFK:          60.0,
				// Reranking and HybridSearch are disabled (zero value)
			},
			wantError: true,
		},
		{
			name: "invalid - negative quota",
			config: RetrievalConfig{
				SearchLimit:   50,
				QuotaOriginal: -1,
				QuotaExpanded: 5,
				RRFK:          60.0,
			},
			wantError: true,
		},
		{
			name: "invalid - total quota exceeds research recommendation",
			config: RetrievalConfig{
				SearchLimit:   50,
				QuotaOriginal: 15,
				QuotaExpanded: 10,
				RRFK:          60.0,
			},
			wantError: true, // > 20 degrades performance per research
		},
		{
			name: "invalid - reranking config invalid",
			config: RetrievalConfig{
				SearchLimit:   50,
				QuotaOriginal: 5,
				QuotaExpanded: 5,
				RRFK:          60.0,
				Reranking: RerankingConfig{
					Enabled: true,
					TopK:    0, // Invalid
					Timeout: 30 * time.Second,
				},
			},
			wantError: true,
		},
		{
			name: "invalid - hybrid search config invalid",
			config: RetrievalConfig{
				SearchLimit:   50,
				QuotaOriginal: 5,
				QuotaExpanded: 5,
				RRFK:          60.0,
				HybridSearch: HybridSearchConfig{
					Enabled:   true,
					Alpha:     1.5, // Invalid > 1.0
					BM25Limit: 50,
				},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLanguageAllocationConfig_Default(t *testing.T) {
	cfg := DefaultLanguageAllocationConfig()
	assert.True(t, cfg.Enabled, "dynamic language allocation should be enabled by default")
}
