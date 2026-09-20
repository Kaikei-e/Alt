package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRAGAuth(t *testing.T) {
	dir := t.TempDir()
	goodTokenFile := filepath.Join(dir, "good.txt")
	if err := os.WriteFile(goodTokenFile, []byte("  a-valid-rag-api-token-that-is-long-enough\n"), 0o600); err != nil {
		t.Fatalf("failed to write test token file: %v", err)
	}

	shortTokenFile := filepath.Join(dir, "short.txt")
	if err := os.WriteFile(shortTokenFile, []byte("too-short\n"), 0o600); err != nil {
		t.Fatalf("failed to write short token file: %v", err)
	}

	tests := []struct {
		name        string
		envVars     map[string]string
		wantToken   string
		wantEnabled bool
		wantErr     bool
		errContains string
	}{
		{
			name:    "unset fails fast rather than falling open",
			envVars: map[string]string{},
			wantErr: true,
		},
		{
			name:        "RAG_API_AUTH=disabled runs without a token",
			envVars:     map[string]string{"RAG_API_AUTH": "disabled"},
			wantToken:   "",
			wantEnabled: false,
		},
		{
			name:        "RAG_API_AUTH=Disabled is case-insensitive",
			envVars:     map[string]string{"RAG_API_AUTH": "Disabled"},
			wantToken:   "",
			wantEnabled: false,
		},
		{
			name:        "RAG_API_TOKEN_FILE resolves the token",
			envVars:     map[string]string{"RAG_API_TOKEN_FILE": goodTokenFile},
			wantToken:   "a-valid-rag-api-token-that-is-long-enough",
			wantEnabled: true,
		},
		{
			name:        "short RAG_API_TOKEN_FILE token fails fast",
			envVars:     map[string]string{"RAG_API_TOKEN_FILE": shortTokenFile},
			wantErr:     true,
			errContains: "at least",
		},
		{
			name:        "unreadable RAG_API_TOKEN_FILE fails fast",
			envVars:     map[string]string{"RAG_API_TOKEN_FILE": filepath.Join(dir, "missing.txt")},
			wantErr:     true,
			errContains: "RAG_API_TOKEN_FILE",
		},
		{
			name:        "RAG_API_TOKEN env resolves the token",
			envVars:     map[string]string{"RAG_API_TOKEN": "an-env-supplied-rag-api-token-value"},
			wantToken:   "an-env-supplied-rag-api-token-value",
			wantEnabled: true,
		},
		{
			name:        "short RAG_API_TOKEN env fails fast",
			envVars:     map[string]string{"RAG_API_TOKEN": "too-short"},
			wantErr:     true,
			errContains: "at least",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("RAG_API_AUTH", "")
			t.Setenv("RAG_API_TOKEN_FILE", "")
			t.Setenv("RAG_API_TOKEN", "")
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			token, enabled, err := LoadRAGAuth()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("LoadRAGAuth() expected error, got none")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("LoadRAGAuth() error = %v, want to contain %q", err, tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadRAGAuth() unexpected error: %v", err)
			}
			if token != tt.wantToken {
				t.Errorf("LoadRAGAuth() token = %q, want %q", token, tt.wantToken)
			}
			if enabled != tt.wantEnabled {
				t.Errorf("LoadRAGAuth() enabled = %v, want %v", enabled, tt.wantEnabled)
			}
		})
	}
}

func TestValidateRAGAuthConfig(t *testing.T) {
	dir := t.TempDir()
	goodTokenFile := filepath.Join(dir, "good.txt")
	if err := os.WriteFile(goodTokenFile, []byte("a-valid-rag-api-token-that-is-long-enough"), 0o600); err != nil {
		t.Fatalf("failed to write test token file: %v", err)
	}

	shortTokenFile := filepath.Join(dir, "short.txt")
	if err := os.WriteFile(shortTokenFile, []byte("too-short"), 0o600); err != nil {
		t.Fatalf("failed to write short token file: %v", err)
	}

	tests := []struct {
		name        string
		cfg         RAGConfig
		appEnv      string
		wantErr     bool
		errContains string
		wantToken   string
	}{
		{
			name:   "empty orchestrator url skips validation",
			cfg:    RAGConfig{OrchestratorURL: ""},
			appEnv: "production",
		},
		{
			name:   "disabled auth is allowed in development",
			cfg:    RAGConfig{OrchestratorURL: "http://rag:9010", APIAuth: "disabled"},
			appEnv: "development",
		},
		{
			name:        "disabled auth is rejected in production",
			cfg:         RAGConfig{OrchestratorURL: "http://rag:9010", APIAuth: "disabled"},
			appEnv:      "production",
			wantErr:     true,
			errContains: "RAG_API_AUTH=disabled is not permitted in production",
		},
		{
			name:      "valid token file loads and trims",
			cfg:       RAGConfig{OrchestratorURL: "http://rag:9010", APITokenFile: goodTokenFile},
			appEnv:    "production",
			wantToken: "a-valid-rag-api-token-that-is-long-enough",
		},
		{
			name:        "missing token file fails fast",
			cfg:         RAGConfig{OrchestratorURL: "http://rag:9010", APITokenFile: filepath.Join(dir, "missing.txt")},
			appEnv:      "production",
			wantErr:     true,
			errContains: "RAG_API_TOKEN_FILE",
		},
		{
			name:        "short token file fails fast",
			cfg:         RAGConfig{OrchestratorURL: "http://rag:9010", APITokenFile: shortTokenFile},
			appEnv:      "production",
			wantErr:     true,
			errContains: "at least",
		},
		{
			name:      "valid token in config passes",
			cfg:       RAGConfig{OrchestratorURL: "http://rag:9010", APIToken: "a-valid-rag-api-token-that-is-long-enough"},
			appEnv:    "production",
			wantToken: "a-valid-rag-api-token-that-is-long-enough",
		},
		{
			name:        "short token in config fails fast",
			cfg:         RAGConfig{OrchestratorURL: "http://rag:9010", APIToken: "too-short"},
			appEnv:      "production",
			wantErr:     true,
			errContains: "at least",
		},
		{
			name:        "unset auth and token fails fast",
			cfg:         RAGConfig{OrchestratorURL: "http://rag:9010"},
			appEnv:      "production",
			wantErr:     true,
			errContains: "RAG_API_TOKEN_FILE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.cfg
			err := ValidateRAGAuthConfig(&cfg, tt.appEnv)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ValidateRAGAuthConfig() expected error, got none")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateRAGAuthConfig() error = %v, want to contain %q", err, tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateRAGAuthConfig() unexpected error: %v", err)
			}
			if tt.wantToken != "" && cfg.APIToken != tt.wantToken {
				t.Errorf("ValidateRAGAuthConfig() token = %q, want %q", cfg.APIToken, tt.wantToken)
			}
		})
	}
}
