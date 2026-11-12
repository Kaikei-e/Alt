package config

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"strings"
	"testing"
)

func marshalTestEd25519SPKIPEM(t *testing.T) (ed25519.PublicKey, []byte) {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("marshal ed25519 public key: %v", err)
	}
	return pub, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}

func marshalTestECDSASPKIPEM(t *testing.T) []byte {
	t.Helper()
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ecdsa key: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(&ecKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal ecdsa public key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}

func marshalTestEd25519PKCS8PrivatePEM(t *testing.T) []byte {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal ed25519 private key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

func TestLoadBackendTokenVerifyKey(t *testing.T) {
	validPub, validPEM := marshalTestEd25519SPKIPEM(t)
	if !bytes.HasSuffix(validPEM, []byte("\n")) {
		t.Fatalf("expected pem.EncodeToMemory output to end with newline")
	}
	crlfPEM := bytes.ReplaceAll(validPEM, []byte("\n"), []byte("\r\n"))
	extraBlankLinesPEM := append(append([]byte{}, validPEM...), []byte("\n\n   \r\n\n")...)
	trailingDataPEM := append(append([]byte{}, validPEM...), []byte("\nnon-whitespace-trailing\n")...)
	ecdsaPEM := marshalTestECDSASPKIPEM(t)
	privatePEM := marshalTestEd25519PKCS8PrivatePEM(t)

	tests := []struct {
		name         string
		keyFile      string
		prePopulated ed25519.PublicKey
		readFile     FileReader
		wantErr      bool
		errContain   string
		wantKey      ed25519.PublicKey
	}{
		{
			name:    "unset BACKEND_TOKEN_VERIFY_KEY_FILE fails fast",
			keyFile: "",
			readFile: func(string) ([]byte, error) {
				return nil, errors.New("should not be called")
			},
			wantErr:    true,
			errContain: "BACKEND_TOKEN_VERIFY_KEY_FILE",
		},
		{
			name:         "unset BACKEND_TOKEN_VERIFY_KEY_FILE fails even when public key is pre-populated",
			keyFile:      "",
			prePopulated: validPub,
			readFile: func(string) ([]byte, error) {
				return nil, errors.New("should not be called")
			},
			wantErr:    true,
			errContain: "BACKEND_TOKEN_VERIFY_KEY_FILE",
		},
		{
			name:    "file read error fails fast",
			keyFile: "/run/secrets/backend_token_verify_key",
			readFile: func(string) ([]byte, error) {
				return nil, errors.New("permission denied")
			},
			wantErr:    true,
			errContain: "read BACKEND_TOKEN_VERIFY_KEY_FILE",
		},
		{
			name:    "empty content fails fast",
			keyFile: "/run/secrets/backend_token_verify_key",
			readFile: func(string) ([]byte, error) {
				return []byte("   \n\t  "), nil
			},
			wantErr:    true,
			errContain: "BACKEND_TOKEN_VERIFY_KEY_FILE",
		},
		{
			name:    "non-Ed25519 SPKI public key fails fast",
			keyFile: "/run/secrets/backend_token_verify_key",
			readFile: func(string) ([]byte, error) {
				return ecdsaPEM, nil
			},
			wantErr:    true,
			errContain: "Ed25519",
		},
		{
			name:    "PKCS8 private key PEM fails fast",
			keyFile: "/run/secrets/backend_token_verify_key",
			readFile: func(string) ([]byte, error) {
				return privatePEM, nil
			},
			wantErr:    true,
			errContain: "PUBLIC KEY",
		},
		{
			name:    "valid PEM with trailing newline is parsed and set",
			keyFile: "/run/secrets/backend_token_verify_key",
			readFile: func(string) ([]byte, error) {
				return validPEM, nil
			},
			wantErr: false,
			wantKey: validPub,
		},
		{
			name:    "valid PEM with CRLF line endings is parsed and set",
			keyFile: "/run/secrets/backend_token_verify_key",
			readFile: func(string) ([]byte, error) {
				return crlfPEM, nil
			},
			wantErr: false,
			wantKey: validPub,
		},
		{
			name:    "valid PEM with extra blank lines after END line is parsed and set",
			keyFile: "/run/secrets/backend_token_verify_key",
			readFile: func(string) ([]byte, error) {
				return extraBlankLinesPEM, nil
			},
			wantErr: false,
			wantKey: validPub,
		},
		{
			name:    "non-whitespace data after END line is rejected",
			keyFile: "/run/secrets/backend_token_verify_key",
			readFile: func(string) ([]byte, error) {
				return trailingDataPEM, nil
			},
			wantErr:    true,
			errContain: "trailing data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BACKEND_TOKEN_VERIFY_KEY_FILE", tt.keyFile)
			cfg := &AuthConfig{BackendTokenPublicKey: tt.prePopulated}
			err := loadBackendTokenVerifyKey(cfg, tt.readFile)
			if (err != nil) != tt.wantErr {
				t.Fatalf("loadBackendTokenVerifyKey() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errContain) {
				t.Fatalf("loadBackendTokenVerifyKey() error = %v, want to contain %q", err, tt.errContain)
			}
			if !tt.wantErr && !cfg.BackendTokenPublicKey.Equal(tt.wantKey) {
				t.Fatalf("cfg.BackendTokenPublicKey = %x, want %x", cfg.BackendTokenPublicKey, tt.wantKey)
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
