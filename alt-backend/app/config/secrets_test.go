package config

import (
	"errors"
	"strings"
	"testing"
)

func TestLoadBackendTokenSecret(t *testing.T) {
	tests := []struct {
		name       string
		secretFile string
		readFile   FileReader
		wantErr    bool
		errContain string
		wantSecret string
	}{
		{
			name:       "empty path is a no-op",
			secretFile: "",
			readFile: func(string) ([]byte, error) {
				return nil, errors.New("should not be called")
			},
			wantErr:    false,
			wantSecret: "",
		},
		{
			name:       "file read error fails fast",
			secretFile: "/run/secrets/backend_secret",
			readFile: func(string) ([]byte, error) {
				return nil, errors.New("permission denied")
			},
			wantErr:    true,
			errContain: "read BACKEND_TOKEN_SECRET_FILE",
		},
		{
			name:       "empty content fails fast",
			secretFile: "/run/secrets/backend_secret",
			readFile: func(string) ([]byte, error) {
				return []byte("   \n\t  "), nil
			},
			wantErr:    true,
			errContain: "resolved to an empty secret",
		},
		{
			name:       "valid secret is trimmed and set",
			secretFile: "/run/secrets/backend_secret",
			readFile: func(string) ([]byte, error) {
				return []byte("valid-backend-secret-content\n"), nil
			},
			wantErr:    false,
			wantSecret: "valid-backend-secret-content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &AuthConfig{BackendTokenSecretFile: tt.secretFile}
			err := loadBackendTokenSecret(cfg, tt.readFile)
			if (err != nil) != tt.wantErr {
				t.Fatalf("loadBackendTokenSecret() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errContain) {
				t.Fatalf("loadBackendTokenSecret() error = %v, want to contain %q", err, tt.errContain)
			}
			if !tt.wantErr && cfg.BackendTokenSecret != tt.wantSecret {
				t.Fatalf("cfg.BackendTokenSecret = %q, want %q", cfg.BackendTokenSecret, tt.wantSecret)
			}
		})
	}
}

func TestLoadInternalAuthSecret(t *testing.T) {
	tests := []struct {
		name       string
		secretFile string
		readFile   FileReader
		wantErr    bool
		errContain string
		wantSecret string
	}{
		{
			name:       "empty path is a no-op",
			secretFile: "",
			readFile: func(string) ([]byte, error) {
				return nil, errors.New("should not be called")
			},
			wantErr:    false,
			wantSecret: "",
		},
		{
			name:       "file read error fails fast",
			secretFile: "/run/secrets/internal_auth",
			readFile: func(string) ([]byte, error) {
				return nil, errors.New("permission denied")
			},
			wantErr:    true,
			errContain: "read INTERNAL_AUTH_SECRET_FILE",
		},
		{
			name:       "empty content fails fast",
			secretFile: "/run/secrets/internal_auth",
			readFile: func(string) ([]byte, error) {
				return []byte("   \n\t  "), nil
			},
			wantErr:    true,
			errContain: "resolved to an empty secret",
		},
		{
			name:       "valid secret is trimmed and set",
			secretFile: "/run/secrets/internal_auth",
			readFile: func(string) ([]byte, error) {
				return []byte("valid-internal-auth-secret\n"), nil
			},
			wantErr:    false,
			wantSecret: "valid-internal-auth-secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &AuthConfig{InternalAuthSecretFile: tt.secretFile}
			err := loadInternalAuthSecret(cfg, tt.readFile)
			if (err != nil) != tt.wantErr {
				t.Fatalf("loadInternalAuthSecret() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errContain) {
				t.Fatalf("loadInternalAuthSecret() error = %v, want to contain %q", err, tt.errContain)
			}
			if !tt.wantErr && cfg.InternalAuthSecret != tt.wantSecret {
				t.Fatalf("cfg.InternalAuthSecret = %q, want %q", cfg.InternalAuthSecret, tt.wantSecret)
			}
		})
	}
}

func TestLoadRAGAPIToken(t *testing.T) {
	tests := []struct {
		name       string
		secretFile string
		readFile   FileReader
		wantErr    bool
		errContain string
		wantSecret string
	}{
		{
			name:       "empty path is a no-op",
			secretFile: "",
			readFile: func(string) ([]byte, error) {
				return nil, errors.New("should not be called")
			},
			wantErr:    false,
			wantSecret: "",
		},
		{
			name:       "file read error fails fast",
			secretFile: "/run/secrets/rag_token",
			readFile: func(string) ([]byte, error) {
				return nil, errors.New("file missing")
			},
			wantErr:    true,
			errContain: "read RAG_API_TOKEN_FILE",
		},
		{
			name:       "empty content fails fast",
			secretFile: "/run/secrets/rag_token",
			readFile: func(string) ([]byte, error) {
				return []byte(""), nil
			},
			wantErr:    true,
			errContain: "resolved to an empty secret",
		},
		{
			name:       "valid secret is trimmed and set",
			secretFile: "/run/secrets/rag_token",
			readFile: func(string) ([]byte, error) {
				return []byte("valid-rag-api-token-long-enough\n"), nil
			},
			wantErr:    false,
			wantSecret: "valid-rag-api-token-long-enough",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &RAGConfig{APITokenFile: tt.secretFile}
			err := loadRAGAPIToken(cfg, tt.readFile)
			if (err != nil) != tt.wantErr {
				t.Fatalf("loadRAGAPIToken() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errContain) {
				t.Fatalf("loadRAGAPIToken() error = %v, want to contain %q", err, tt.errContain)
			}
			if !tt.wantErr && cfg.APIToken != tt.wantSecret {
				t.Fatalf("cfg.APIToken = %q, want %q", cfg.APIToken, tt.wantSecret)
			}
		})
	}
}

func TestLoadSovereignSecret(t *testing.T) {
	tests := []struct {
		name       string
		secretFile string
		readFile   FileReader
		wantErr    bool
		errContain string
		wantSecret string
	}{
		{
			name:       "read error fails fast",
			secretFile: "/run/secrets/sovereign_token",
			readFile: func(string) ([]byte, error) {
				return nil, errors.New("read failed")
			},
			wantErr:    true,
			errContain: "read SOVEREIGN_EVENT_TOKEN_FILE",
		},
		{
			name:       "short token fails fast",
			secretFile: "/run/secrets/sovereign_token",
			readFile: func(string) ([]byte, error) {
				return []byte("too-short"), nil
			},
			wantErr:    true,
			errContain: "must be at least 24 characters",
		},
		{
			name:       "valid token is set",
			secretFile: "/run/secrets/sovereign_token",
			readFile: func(string) ([]byte, error) {
				return []byte("valid-sovereign-token-24-characters\n"), nil
			},
			wantErr:    false,
			wantSecret: "valid-sovereign-token-24-characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &SovereignConfig{EventTokenFile: tt.secretFile}
			err := loadSovereignSecret(cfg, tt.readFile)
			if (err != nil) != tt.wantErr {
				t.Fatalf("loadSovereignSecret() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errContain) {
				t.Fatalf("loadSovereignSecret() error = %v, want to contain %q", err, tt.errContain)
			}
			if !tt.wantErr && cfg.EventToken != tt.wantSecret {
				t.Fatalf("cfg.EventToken = %q, want %q", cfg.EventToken, tt.wantSecret)
			}
		})
	}
}

func TestLoadImageProxyAndWebPushSecrets(t *testing.T) {
	mockFiles := map[string][]byte{
		"/secrets/image": []byte("image-secret\n"),
		"/secrets/pub":   []byte("vapid-pub-key\n"),
		"/secrets/priv":  []byte("vapid-priv-key\n"),
	}
	readFile := func(path string) ([]byte, error) {
		if content, ok := mockFiles[path]; ok {
			return content, nil
		}
		return nil, errors.New("not found")
	}

	imgCfg := &ImageProxyConfig{SecretFile: "/secrets/image"}
	if err := loadImageProxySecret(imgCfg, readFile); err != nil {
		t.Fatalf("loadImageProxySecret failed: %v", err)
	}
	if imgCfg.Secret != "image-secret" {
		t.Errorf("imgCfg.Secret = %q, want %q", imgCfg.Secret, "image-secret")
	}

	pushCfg := &WebPushConfig{
		PublicKeyFile:  "/secrets/pub",
		PrivateKeyFile: "/secrets/priv",
	}
	if err := loadWebPushSecrets(pushCfg, readFile); err != nil {
		t.Fatalf("loadWebPushSecrets failed: %v", err)
	}
	if pushCfg.PublicKey != "vapid-pub-key" {
		t.Errorf("pushCfg.PublicKey = %q, want %q", pushCfg.PublicKey, "vapid-pub-key")
	}
	if pushCfg.PrivateKey != "vapid-priv-key" {
		t.Errorf("pushCfg.PrivateKey = %q, want %q", pushCfg.PrivateKey, "vapid-priv-key")
	}
}
