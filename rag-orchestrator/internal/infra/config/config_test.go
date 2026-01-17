package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad_RAGRetrievalParameters_Defaults(t *testing.T) {
	// Clear all relevant env vars
	envVars := []string{
		"RAG_SEARCH_LIMIT",
		"RAG_QUOTA_ORIGINAL",
		"RAG_QUOTA_EXPANDED",
		"RAG_RRF_K",
	}
	for _, key := range envVars {
		_ = os.Unsetenv(key)
	}

	cfg := Load()

	// Research-backed defaults (EMNLP 2024, Microsoft RAG Guide)
	assert.Equal(t, 50, cfg.RAGSearchLimit, "searchLimit should default to 50")
	assert.Equal(t, 5, cfg.RAGQuotaOriginal, "quotaOriginal should default to 5")
	assert.Equal(t, 5, cfg.RAGQuotaExpanded, "quotaExpanded should default to 5")
	assert.Equal(t, 60.0, cfg.RAGRRFK, "rrfK should default to 60.0")
}

func TestLoad_RAGRetrievalParameters_FromEnv(t *testing.T) {
	// Set custom values
	t.Setenv("RAG_SEARCH_LIMIT", "100")
	t.Setenv("RAG_QUOTA_ORIGINAL", "7")
	t.Setenv("RAG_QUOTA_EXPANDED", "3")
	t.Setenv("RAG_RRF_K", "50.0")

	cfg := Load()

	assert.Equal(t, 100, cfg.RAGSearchLimit)
	assert.Equal(t, 7, cfg.RAGQuotaOriginal)
	assert.Equal(t, 3, cfg.RAGQuotaExpanded)
	assert.Equal(t, 50.0, cfg.RAGRRFK)
}

func TestLoad_TemporalBoostParameters_Defaults(t *testing.T) {
	// Clear all relevant env vars
	envVars := []string{
		"TEMPORAL_BOOST_6H",
		"TEMPORAL_BOOST_12H",
		"TEMPORAL_BOOST_18H",
	}
	for _, key := range envVars {
		_ = os.Unsetenv(key)
	}

	cfg := Load()

	// Current defaults
	assert.Equal(t, float32(1.3), cfg.TemporalBoost6h)
	assert.Equal(t, float32(1.15), cfg.TemporalBoost12h)
	assert.Equal(t, float32(1.05), cfg.TemporalBoost18h)
}

func TestLoad_TemporalBoostParameters_FromEnv(t *testing.T) {
	t.Setenv("TEMPORAL_BOOST_6H", "1.5")
	t.Setenv("TEMPORAL_BOOST_12H", "1.25")
	t.Setenv("TEMPORAL_BOOST_18H", "1.1")

	cfg := Load()

	assert.Equal(t, float32(1.5), cfg.TemporalBoost6h)
	assert.Equal(t, float32(1.25), cfg.TemporalBoost12h)
	assert.Equal(t, float32(1.1), cfg.TemporalBoost18h)
}

func TestGetEnvFloat64(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		fallback float64
		expected float64
	}{
		{
			name:     "valid value",
			envValue: "75.5",
			fallback: 60.0,
			expected: 75.5,
		},
		{
			name:     "invalid value uses fallback",
			envValue: "not-a-number",
			fallback: 60.0,
			expected: 60.0,
		},
		{
			name:     "empty uses fallback",
			envValue: "",
			fallback: 60.0,
			expected: 60.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv("TEST_FLOAT", tt.envValue)
			} else {
				_ = os.Unsetenv("TEST_FLOAT")
			}

			result := getEnvFloat64("TEST_FLOAT", tt.fallback)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetEnvFloat32(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		fallback float32
		expected float32
	}{
		{
			name:     "valid value",
			envValue: "1.5",
			fallback: 1.3,
			expected: 1.5,
		},
		{
			name:     "invalid value uses fallback",
			envValue: "invalid",
			fallback: 1.3,
			expected: 1.3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TEST_FLOAT32", tt.envValue)

			result := getEnvFloat32("TEST_FLOAT32", tt.fallback)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLoad_DynamicLanguageAllocation_Default(t *testing.T) {
	_ = os.Unsetenv("RAG_DYNAMIC_LANGUAGE_ALLOCATION")

	cfg := Load()

	assert.True(t, cfg.DynamicLanguageAllocationEnabled, "dynamic language allocation should be enabled by default")
}

func TestLoad_DynamicLanguageAllocation_Disabled(t *testing.T) {
	t.Setenv("RAG_DYNAMIC_LANGUAGE_ALLOCATION", "false")

	cfg := Load()

	assert.False(t, cfg.DynamicLanguageAllocationEnabled, "dynamic language allocation should be disabled when env var is false")
}
