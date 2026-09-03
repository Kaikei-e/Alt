package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	if err := os.Setenv("DB_PASSWORD", "test-password"); err != nil {
		panic(err)
	}
	if err := os.Setenv("PEER_IDENTITY_MODE", "disabled"); err != nil {
		panic(err)
	}
	// Load() fail-fasts without these (ADR-000954 D7): alt-data-hub is the
	// only route to alt_db and it accepts nothing but a client certificate,
	// so there is no configuration in which they are optional.
	for k, v := range map[string]string{
		"DATAHUB_MTLS_URL": "https://alt-data-hub:9443",
		"MTLS_CERT_FILE":   "/certs/svc-cert.pem",
		"MTLS_KEY_FILE":    "/certs/svc-key.pem",
		"MTLS_CA_FILE":     "/trust/ca-bundle.pem",
	} {
		if err := os.Setenv(k, v); err != nil {
			panic(err)
		}
	}
	os.Exit(m.Run())
}

func TestLoad_DataHub_MTLSURLUnsetPanics(t *testing.T) {
	unsetEnv(t, "DATAHUB_MTLS_URL")

	assert.Panics(t, func() { Load() },
		"unset DATAHUB_MTLS_URL must fail startup; the plaintext surface it replaces no longer exists")
}

func TestLoad_DataHub_PlaintextURLPanics(t *testing.T) {
	t.Setenv("DATAHUB_MTLS_URL", "http://alt-backend:9102")

	assert.Panics(t, func() { Load() },
		"an http:// data-hub URL is the dead pre-split endpoint and must never be accepted")
}

func TestLoad_DataHub_MissingCertMaterialPanics(t *testing.T) {
	t.Setenv("DATAHUB_MTLS_URL", "https://alt-data-hub:9443")
	unsetEnv(t, "MTLS_CERT_FILE")
	unsetEnv(t, "MTLS_KEY_FILE")
	unsetEnv(t, "MTLS_CA_FILE")

	assert.Panics(t, func() { Load() },
		"the client certificate is the only credential alt-data-hub accepts")
}

func TestLoad_DataHub_FullyConfigured(t *testing.T) {
	t.Setenv("DATAHUB_MTLS_URL", "https://alt-data-hub:9443")
	t.Setenv("DATAHUB_MTLS_SERVER_NAME", "alt-data-hub")
	t.Setenv("DATAHUB_TIMEOUT", "45")
	t.Setenv("MTLS_CERT_FILE", "/certs/svc-cert.pem")
	t.Setenv("MTLS_KEY_FILE", "/certs/svc-key.pem")
	t.Setenv("MTLS_CA_FILE", "/trust/ca-bundle.pem")

	cfg := Load()

	assert.Equal(t, "https://alt-data-hub:9443", cfg.DataHub.MTLSURL)
	assert.Equal(t, "alt-data-hub", cfg.DataHub.ServerName)
	assert.Equal(t, "/certs/svc-cert.pem", cfg.DataHub.CertFile)
	assert.Equal(t, "/certs/svc-key.pem", cfg.DataHub.KeyFile)
	assert.Equal(t, "/trust/ca-bundle.pem", cfg.DataHub.CAFile)
	assert.Equal(t, 45, cfg.DataHub.Timeout)
}

// ServerName is the one data-hub setting that may legitimately be empty:
// crypto/tls then derives it from the URL host, which is what makes the
// compose default (https://alt-data-hub:9443 against CERT_SANS=alt-data-hub)
// work without an extra variable. Empty must therefore NOT be a startup
// failure — but it must also not be quietly rewritten to something else.
func TestLoad_DataHub_ServerNameOptional(t *testing.T) {
	t.Setenv("DATAHUB_MTLS_URL", "https://alt-data-hub:9443")
	unsetEnv(t, "DATAHUB_MTLS_SERVER_NAME")

	cfg := Load()

	assert.Empty(t, cfg.DataHub.ServerName)
}

// unsetEnv removes key for the duration of the test, restoring the prior
// value afterwards (t.Setenv registers the restore; os.Unsetenv then clears).
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	_ = os.Unsetenv(key)
}

func TestLoad_PeerIdentityMode_UnsetPanics(t *testing.T) {
	unsetEnv(t, "PEER_IDENTITY_MODE")

	assert.Panics(t, func() { Load() },
		"unset PEER_IDENTITY_MODE must fail startup, never be inferred as disabled")
}

func TestLoad_PeerIdentityMode_InvalidValuePanics(t *testing.T) {
	t.Setenv("PEER_IDENTITY_MODE", "yes-please")

	assert.Panics(t, func() { Load() })
}

func TestLoad_PeerIdentityMode_DisabledIsExplicit(t *testing.T) {
	t.Setenv("PEER_IDENTITY_MODE", "disabled")

	cfg := Load()

	assert.Equal(t, PeerIdentityDisabled, cfg.PeerIdentity.Mode)
}

// MTLS_CERT_FILE/KEY/CA back both directions — the inbound Connect-RPC
// listener and the outbound alt-data-hub client — so unsetting them fails
// startup whichever loader reaches them first. The invariant under test is
// that the process refuses to start, not which loader reports it.
func TestLoad_PeerIdentityMode_MTLSRequiresCerts(t *testing.T) {
	t.Setenv("PEER_IDENTITY_MODE", "mtls")
	unsetEnv(t, "MTLS_CERT_FILE")
	unsetEnv(t, "MTLS_KEY_FILE")
	unsetEnv(t, "MTLS_CA_FILE")

	assert.Panics(t, func() { Load() },
		"mtls mode without cert paths must fail startup")
}

func TestLoad_PeerIdentityMode_MTLSRequiresAllowlist(t *testing.T) {
	t.Setenv("PEER_IDENTITY_MODE", "mtls")
	t.Setenv("MTLS_CERT_FILE", "/certs/svc-cert.pem")
	t.Setenv("MTLS_KEY_FILE", "/certs/svc-key.pem")
	t.Setenv("MTLS_CA_FILE", "/trust/ca-bundle.pem")
	unsetEnv(t, "PEER_IDENTITY_ALLOWED_PEERS")

	assert.Panics(t, func() { Load() },
		"mtls mode without a peer allowlist must fail startup")
}

func TestLoad_PeerIdentityMode_MTLSFullyConfigured(t *testing.T) {
	t.Setenv("PEER_IDENTITY_MODE", "mtls")
	t.Setenv("MTLS_CERT_FILE", "/certs/rag-orchestrator/cert.pem")
	t.Setenv("MTLS_KEY_FILE", "/certs/rag-orchestrator/key.pem")
	t.Setenv("MTLS_CA_FILE", "/certs/ca/ca.pem")
	t.Setenv("PEER_IDENTITY_ALLOWED_PEERS", "alt-backend, alt-butterfly-facade")

	cfg := Load()

	assert.Equal(t, PeerIdentityMTLS, cfg.PeerIdentity.Mode)
	assert.Equal(t, "/certs/rag-orchestrator/cert.pem", cfg.PeerIdentity.CertFile)
	assert.Equal(t, "/certs/rag-orchestrator/key.pem", cfg.PeerIdentity.KeyFile)
	assert.Equal(t, "/certs/ca/ca.pem", cfg.PeerIdentity.CAFile)
	assert.Equal(t, []string{"alt-backend", "alt-butterfly-facade"}, cfg.PeerIdentity.AllowedPeers)
}

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
		SSLMode:  "disable",
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

	assert.Equal(t, 3072, cfg.RAG.MaxTokens, "RAG default max tokens should default to 3072")
}

func TestLoad_AugurKnowledgeModel_Default(t *testing.T) {
	_ = os.Unsetenv("AUGUR_KNOWLEDGE_MODEL")

	cfg := Load()

	assert.Equal(t, "gemma4-e4b-12k", cfg.Augur.Model, "AUGUR_KNOWLEDGE_MODEL should default to gemma4-e4b-12k (RAG-dedicated local-first)")
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

func TestLoad_Backend_RecapWorkerURL_DefaultAndFromEnv(t *testing.T) {
	// Unset: Load() must resolve the default from config, not leave the field
	// empty for a downstream silent fallback (CLAUDE.md rule 8/9). The default
	// must be recap-worker's real listen port (9005) — the port every other
	// consumer and the compose service definition use.
	_ = os.Unsetenv("RECAP_WORKER_URL")
	cfg := Load()
	assert.Equal(t, "http://recap-worker:9005", cfg.Backend.RecapWorkerURL)

	t.Setenv("RECAP_WORKER_URL", "http://recap-worker:9999")
	cfg = Load()
	assert.Equal(t, "http://recap-worker:9999", cfg.Backend.RecapWorkerURL)
}

func TestLoad_LLMBackend_DefaultAndFromEnv(t *testing.T) {
	_ = os.Unsetenv("LLM_BACKEND")

	cfg := Load()
	assert.Equal(t, "ollama", cfg.LLMBackend)

	t.Setenv("LLM_BACKEND", "eino")
	cfg = Load()
	assert.Equal(t, "eino", cfg.LLMBackend)
}

// TestLoad_RerankWindow_Defaults pins the two numbers apart: TopK shapes the
// output of the rerank stage, MaxCandidates shapes its input. Reusing TopK for
// both made reranking unable to promote anything it had not already ranked.
func TestLoad_RerankWindow_Defaults(t *testing.T) {
	for _, key := range []string{"RERANK_TOP_K", "RERANK_MAX_CANDIDATES", "RERANK_TIMEOUT"} {
		_ = os.Unsetenv(key)
	}

	cfg := Load()

	assert.Equal(t, 10, cfg.Rerank.TopK)
	assert.Equal(t, 40, cfg.Rerank.MaxCandidates)
}

// TestLoad_RerankTimeout_OutlastsTheServer: the rerank server enforces its own
// total budget (RERANK_SERVER_TIMEOUT, 10s including semaphore wait). A client
// that gives up first turns a slow-but-successful rerank into a silent fallback
// to retrieval order, and the reason never reaches either log.
func TestLoad_RerankTimeout_OutlastsTheServer(t *testing.T) {
	_ = os.Unsetenv("RERANK_TIMEOUT")

	cfg := Load()

	assert.Greater(t, cfg.Rerank.Timeout, rerankServerTimeoutSeconds,
		"the client deadline must outlast the server's own")
}

func TestLoad_RerankWindow_FromEnv(t *testing.T) {
	t.Setenv("RERANK_TOP_K", "8")
	t.Setenv("RERANK_MAX_CANDIDATES", "15")
	t.Setenv("RERANK_TIMEOUT", "20")

	cfg := Load()

	assert.Equal(t, 8, cfg.Rerank.TopK)
	assert.Equal(t, 15, cfg.Rerank.MaxCandidates)
	assert.Equal(t, 20, cfg.Rerank.Timeout)
}

func TestLoad_DefaultMaxChunksMatchesRerankTopK(t *testing.T) {
	unsetEnv(t, "RAG_DEFAULT_MAX_CHUNKS")
	unsetEnv(t, "RERANK_TOP_K")

	cfg := Load()

	assert.Equal(t, cfg.Rerank.TopK, cfg.RAG.MaxChunks,
		"every hit the reranker keeps must reach the prompt; a smaller MaxChunks silently drops the tail")
}
