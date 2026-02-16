package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad_RAGRetrievalParameters_Defaults(t *testing.T) {
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

	assert.Equal(t, 50, cfg.RAG.SearchLimit, "searchLimit should default to 50")
	assert.Equal(t, 5, cfg.RAG.QuotaOriginal, "quotaOriginal should default to 5")
	assert.Equal(t, 5, cfg.RAG.QuotaExpanded, "quotaExpanded should default to 5")
	assert.Equal(t, 60.0, cfg.RAG.RRFK, "rrfK should default to 60.0")
}

func TestLoad_RAGRetrievalParameters_FromEnv(t *testing.T) {
	t.Setenv("RAG_SEARCH_LIMIT", "100")
	t.Setenv("RAG_QUOTA_ORIGINAL", "7")
	t.Setenv("RAG_QUOTA_EXPANDED", "3")
	t.Setenv("RAG_RRF_K", "50.0")

	cfg := Load()

	assert.Equal(t, 100, cfg.RAG.SearchLimit)
	assert.Equal(t, 7, cfg.RAG.QuotaOriginal)
	assert.Equal(t, 3, cfg.RAG.QuotaExpanded)
	assert.Equal(t, 50.0, cfg.RAG.RRFK)
}

func TestLoad_TemporalBoostParameters_Defaults(t *testing.T) {
	envVars := []string{
		"TEMPORAL_BOOST_6H",
		"TEMPORAL_BOOST_12H",
		"TEMPORAL_BOOST_18H",
	}
	for _, key := range envVars {
		_ = os.Unsetenv(key)
	}

	cfg := Load()

	assert.Equal(t, float32(1.3), cfg.Temporal.Boost6h)
	assert.Equal(t, float32(1.15), cfg.Temporal.Boost12h)
	assert.Equal(t, float32(1.05), cfg.Temporal.Boost18h)
}

func TestLoad_TemporalBoostParameters_FromEnv(t *testing.T) {
	t.Setenv("TEMPORAL_BOOST_6H", "1.5")
	t.Setenv("TEMPORAL_BOOST_12H", "1.25")
	t.Setenv("TEMPORAL_BOOST_18H", "1.1")

	cfg := Load()

	assert.Equal(t, float32(1.5), cfg.Temporal.Boost6h)
	assert.Equal(t, float32(1.25), cfg.Temporal.Boost12h)
	assert.Equal(t, float32(1.1), cfg.Temporal.Boost18h)
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

	assert.True(t, cfg.RAG.DynamicLanguageAllocationEnabled, "dynamic language allocation should be enabled by default")
}

func TestLoad_DynamicLanguageAllocation_Disabled(t *testing.T) {
	t.Setenv("RAG_DYNAMIC_LANGUAGE_ALLOCATION", "false")

	cfg := Load()

	assert.False(t, cfg.RAG.DynamicLanguageAllocationEnabled, "dynamic language allocation should be disabled when env var is false")
}

func TestDBConfig_DSN(t *testing.T) {
	db := DBConfig{
		Host:     "localhost",
		Port:     "5432",
		User:     "testuser",
		Password: "testpass",
		Name:     "testdb",
	}

	expected := "postgres://testuser:testpass@localhost:5432/testdb?sslmode=disable"
	assert.Equal(t, expected, db.DSN())
}

func TestLoad_ServerConfig_Defaults(t *testing.T) {
	_ = os.Unsetenv("PORT")
	_ = os.Unsetenv("CONNECT_PORT")

	cfg := Load()

	assert.Equal(t, "9010", cfg.Server.Port)
	assert.Equal(t, "9011", cfg.Server.ConnectPort)
}

func TestLoad_DBPoolConfig_Defaults(t *testing.T) {
	_ = os.Unsetenv("DB_MAX_CONNS")
	_ = os.Unsetenv("DB_MIN_CONNS")

	cfg := Load()

	assert.Equal(t, int32(20), cfg.DB.MaxConns)
	assert.Equal(t, int32(5), cfg.DB.MinConns)
}

func TestLoad_MorningLetterMaxTokens_Default(t *testing.T) {
	_ = os.Unsetenv("MORNING_LETTER_MAX_TOKENS")

	cfg := Load()

	assert.Equal(t, 4096, cfg.RAG.MorningLetterMaxTokens, "morning letter max tokens should default to 4096")
}

func TestLoad_MorningLetterMaxTokens_FromEnv(t *testing.T) {
	t.Setenv("MORNING_LETTER_MAX_TOKENS", "6144")

	cfg := Load()

	assert.Equal(t, 6144, cfg.RAG.MorningLetterMaxTokens)
}

func TestLoad_RAGDefaultMaxTokens_UpdatedDefault(t *testing.T) {
	_ = os.Unsetenv("RAG_DEFAULT_MAX_TOKENS")

	cfg := Load()

	assert.Equal(t, 6144, cfg.RAG.MaxTokens, "RAG default max tokens should default to 6144")
}

func TestLoad_AugurKnowledgeModel_Default(t *testing.T) {
	_ = os.Unsetenv("AUGUR_KNOWLEDGE_MODEL")

	cfg := Load()

	assert.Equal(t, "gemma3-12b-rag", cfg.Augur.Model, "AUGUR_KNOWLEDGE_MODEL should default to gemma3-12b-rag")
}

func TestLoad_AugurKnowledgeModel_FromEnv(t *testing.T) {
	t.Setenv("AUGUR_KNOWLEDGE_MODEL", "swallow-8b-rag")

	cfg := Load()

	assert.Equal(t, "swallow-8b-rag", cfg.Augur.Model)
}

func TestLoad_MaxPromptTokens_Default(t *testing.T) {
	_ = os.Unsetenv("RAG_MAX_PROMPT_TOKENS")

	cfg := Load()

	assert.Equal(t, 6000, cfg.RAG.MaxPromptTokens, "MaxPromptTokens should default to 6000")
}

func TestLoad_MaxPromptTokens_FromEnv(t *testing.T) {
	t.Setenv("RAG_MAX_PROMPT_TOKENS", "10000")

	cfg := Load()

	assert.Equal(t, 10000, cfg.RAG.MaxPromptTokens)
}

func TestLoad_CacheConfig_Defaults(t *testing.T) {
	_ = os.Unsetenv("RAG_CACHE_SIZE")
	_ = os.Unsetenv("RAG_CACHE_TTL_MINUTES")

	cfg := Load()

	assert.Equal(t, 256, cfg.Cache.Size)
	assert.Equal(t, 10, cfg.Cache.TTL)
}
