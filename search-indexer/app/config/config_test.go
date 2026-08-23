package config

import (
	"os"
	"path/filepath"
	"testing"
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
				"BACKEND_API_URL":     "http://alt-backend:9101",
				"MEILISEARCH_HOST":    "http://localhost:7700",
				"MEILISEARCH_API_KEY": "key",
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
				"BACKEND_API_URL": "http://alt-backend:9101",
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
	t.Setenv("MEILISEARCH_HOST", "http://localhost:7700")
	t.Setenv("MEILISEARCH_API_KEY_FILE", keyPath)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v; want nil", err)
	}
	if cfg.Meilisearch.APIKey != "super-secret-key" {
		t.Errorf("APIKey = %q, want %q (trimmed file content)", cfg.Meilisearch.APIKey, "super-secret-key")
	}
}
