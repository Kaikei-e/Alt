package config

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantErr bool
	}{
		{
			name: "valid configuration with backend API",
			envVars: map[string]string{
				"BACKEND_API_URL":            "http://alt-backend:9101",
				"USER_JWT_INTROSPECTION_URL": "http://auth-hub:9443/internal/token/introspect",
				"MEILISEARCH_HOST":           "http://localhost:7700",
				"MEILISEARCH_API_KEY":        "key",
				"INFERENCE_AUTH":             "disabled",
			},
			wantErr: false,
		},
		{
			name: "missing BACKEND_API_URL",
			envVars: map[string]string{
				"MEILISEARCH_HOST": "http://localhost:7700",
			},
			wantErr: true,
		},
		{
			name: "missing MEILISEARCH_HOST",
			envVars: map[string]string{
				"BACKEND_API_URL":            "http://alt-backend:9101",
				"USER_JWT_INTROSPECTION_URL": "http://auth-hub:9443/internal/token/introspect",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			cfg, err := Load()

			if tt.wantErr {
				if err == nil {
					t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if cfg.BackendAPI.URL != "http://alt-backend:9101" {
				t.Errorf("BackendAPI.URL = %v, want http://alt-backend:9101", cfg.BackendAPI.URL)
			}
		})
	}
}

// TestLoad_SecretFileUnreadableFailsFast asserts that a *_FILE variable that
// points at an unreadable path fails startup (Rule 9 fail-fast) instead of
// silently falling back to an empty value. In compose/workers.yaml the
// Meilisearch master key is delivered via MEILISEARCH_API_KEY_FILE=/run/secrets/...,
// so a broken secret mount must abort, not connect to Meilisearch with no auth.
func TestLoad_SecretFileUnreadableFailsFast(t *testing.T) {
	t.Setenv("BACKEND_API_URL", "http://alt-backend:9101")
	t.Setenv("USER_JWT_INTROSPECTION_URL", "http://auth-hub:9443/internal/token/introspect")
	t.Setenv("MEILISEARCH_HOST", "http://localhost:7700")
	// _FILE is set but the path does not exist -> ReadFile fails.
	t.Setenv("MEILISEARCH_API_KEY_FILE", filepath.Join(t.TempDir(), "does-not-exist"))

	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil error; want fail-fast error when MEILISEARCH_API_KEY_FILE is unreadable")
	}
}

// TestLoad_SecretFileReadable confirms the happy path still resolves the secret
// from a readable *_FILE mount.
func TestLoad_SecretFileReadable(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "meili_master_key")
	if err := os.WriteFile(keyPath, []byte("  super-secret-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BACKEND_API_URL", "http://alt-backend:9101")
	t.Setenv("USER_JWT_INTROSPECTION_URL", "http://auth-hub:9443/internal/token/introspect")
	t.Setenv("MEILISEARCH_HOST", "http://localhost:7700")
	t.Setenv("MEILISEARCH_API_KEY_FILE", keyPath)
	t.Setenv("INFERENCE_AUTH", "disabled")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v; want nil", err)
	}
	if cfg.Meilisearch.APIKey != "super-secret-key" {
		t.Errorf("APIKey = %q, want %q (trimmed file content)", cfg.Meilisearch.APIKey, "super-secret-key")
	}
}

func setRequiredLoadEnv(t *testing.T) {
	t.Helper()
	t.Setenv("BACKEND_API_URL", "http://alt-backend:9101")
	t.Setenv("USER_JWT_INTROSPECTION_URL", "http://auth-hub:9443/internal/token/introspect")
	t.Setenv("MEILISEARCH_HOST", "http://localhost:7700")
}

// withHybridEmbedder sets MeiliHybridEmbedder, which is read from the
// environment once at package init, for the duration of the test.
func withHybridEmbedder(t *testing.T, name string) {
	t.Helper()
	prev := MeiliHybridEmbedder
	MeiliHybridEmbedder = name
	t.Cleanup(func() { MeiliHybridEmbedder = prev })
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
}

func writeInferenceToken(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "inference_service_token")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// Meilisearch presents this token to embedding-proxy as the hybrid embedder's
// apiKey. Without it the embedder is declared keyless and every embed call
// 401s, so a missing token file has to stop startup.
func TestLoad_InferenceTokenFileRequired(t *testing.T) {
	setRequiredLoadEnv(t)
	withHybridEmbedder(t, "bge-m3")
	unsetEnv(t, "INFERENCE_AUTH")
	unsetEnv(t, "INFERENCE_SERVICE_TOKEN_FILE")

	_, err := Load()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "INFERENCE_SERVICE_TOKEN_FILE")
	assert.Contains(t, err.Error(), "INFERENCE_AUTH=disabled")
}

func TestLoad_InferenceTokenFileEmptyPathRequired(t *testing.T) {
	setRequiredLoadEnv(t)
	withHybridEmbedder(t, "bge-m3")
	unsetEnv(t, "INFERENCE_AUTH")
	t.Setenv("INFERENCE_SERVICE_TOKEN_FILE", "   ")

	_, err := Load()

	require.Error(t, err)
}

func TestLoad_InferenceTokenFileUnreadable(t *testing.T) {
	setRequiredLoadEnv(t)
	withHybridEmbedder(t, "bge-m3")
	unsetEnv(t, "INFERENCE_AUTH")
	t.Setenv("INFERENCE_SERVICE_TOKEN_FILE", filepath.Join(t.TempDir(), "missing"))

	_, err := Load()

	require.Error(t, err)
}

func TestLoad_InferenceTokenFileEmptyContent(t *testing.T) {
	setRequiredLoadEnv(t)
	withHybridEmbedder(t, "bge-m3")
	unsetEnv(t, "INFERENCE_AUTH")
	t.Setenv("INFERENCE_SERVICE_TOKEN_FILE", writeInferenceToken(t, "\n"))

	_, err := Load()

	require.Error(t, err)
}

func TestLoad_InferenceTokenInvalidCharacters(t *testing.T) {
	setRequiredLoadEnv(t)
	withHybridEmbedder(t, "bge-m3")
	unsetEnv(t, "INFERENCE_AUTH")
	t.Setenv("INFERENCE_SERVICE_TOKEN_FILE", writeInferenceToken(t, "has space\n"))

	_, err := Load()

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "has space", "the token must never reach an error message")
}

func TestLoad_InferenceTokenLoaded(t *testing.T) {
	setRequiredLoadEnv(t)
	withHybridEmbedder(t, "bge-m3")
	unsetEnv(t, "INFERENCE_AUTH")
	t.Setenv("INFERENCE_SERVICE_TOKEN_FILE", writeInferenceToken(t, "valid-inference-token==\n"))

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "valid-inference-token==", cfg.Meilisearch.EmbedderInferenceToken)
}

func TestLoad_InferenceAuthDisabledIsExplicit(t *testing.T) {
	setRequiredLoadEnv(t)
	withHybridEmbedder(t, "bge-m3")
	t.Setenv("INFERENCE_AUTH", "disabled")
	unsetEnv(t, "INFERENCE_SERVICE_TOKEN_FILE")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Empty(t, cfg.Meilisearch.EmbedderInferenceToken)
}

// With MEILI_HYBRID_EMBEDDER empty no embedder is declared, so this service
// never reaches embedding-proxy and has no bearer to present.
func TestLoad_InferenceTokenNotNeededWithoutHybridEmbedder(t *testing.T) {
	setRequiredLoadEnv(t)
	withHybridEmbedder(t, "")
	unsetEnv(t, "INFERENCE_AUTH")
	unsetEnv(t, "INFERENCE_SERVICE_TOKEN_FILE")

	cfg, err := Load()

	require.NoError(t, err)
	assert.Empty(t, cfg.Meilisearch.EmbedderInferenceToken)
}

func TestResolveInferenceToken_LogsWiringStateOnce(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		t.Setenv("INFERENCE_AUTH", "Disabled")
		unsetEnv(t, "INFERENCE_SERVICE_TOKEN_FILE")
		var buf bytes.Buffer

		token, err := resolveInferenceToken(slog.New(slog.NewTextHandler(&buf, nil)))

		require.NoError(t, err)
		assert.Empty(t, token)
		assert.Equal(t, 1, strings.Count(buf.String(), "msg=inference_auth_disabled"))
		assert.NotContains(t, buf.String(), "inference_auth_enabled")
	})

	t.Run("enabled", func(t *testing.T) {
		unsetEnv(t, "INFERENCE_AUTH")
		t.Setenv("INFERENCE_SERVICE_TOKEN_FILE", writeInferenceToken(t, "valid-inference-token\n"))
		var buf bytes.Buffer

		token, err := resolveInferenceToken(slog.New(slog.NewTextHandler(&buf, nil)))

		require.NoError(t, err)
		assert.Equal(t, "valid-inference-token", token)
		assert.Equal(t, 1, strings.Count(buf.String(), "msg=inference_auth_enabled"))
		assert.NotContains(t, buf.String(), "valid-inference-token", "the token must never reach the log")
	})
}

// knowledge-embedder-local sits on the internal embedding-raw-network that
// Meilisearch cannot reach; embedding-proxy is the only route to it.
func TestMeiliEmbedderURLDefaultsToEmbeddingProxy(t *testing.T) {
	if os.Getenv("MEILI_EMBEDDER_URL") != "" {
		t.Skip("MEILI_EMBEDDER_URL is set in the test environment")
	}

	assert.Equal(t, "http://embedding-proxy:11436/api/embed", MeiliEmbedderURL)
}
