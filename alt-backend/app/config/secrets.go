package config

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	minSovereignEventTokenLen    = 24
	backendTokenVerifyKeyFileEnv = "BACKEND_TOKEN_VERIFY_KEY_FILE"
)

// FileReader defines the file reading signature for secret ingestion.
type FileReader func(string) ([]byte, error)

// loadImageProxySecret loads image proxy secret from file if configured (Docker Secrets support).
func loadImageProxySecret(cfg *ImageProxyConfig, readFile FileReader) error {
	if cfg.SecretFile != "" {
		content, err := readFile(cfg.SecretFile)
		if err == nil {
			cfg.Secret = strings.TrimSpace(string(content))
		}
	}
	return nil
}

// loadWebPushSecrets loads the VAPID keys from files if configured (Docker Secrets support).
//
// A read failure on public key leaves the env value in place, and
// ValidateBackendConfig rejects an empty result — so a mounted-but-broken
// secret fails startup rather than serving an empty key.
//
// Same pattern for the signing key, read only by cmd/notifier.
// ValidateNotifierConfig rejects an empty result, so a secret that is
// mounted but unreadable fails startup instead of producing a dispatcher
// that answers every send with a 401 nobody looks at.
func loadWebPushSecrets(cfg *WebPushConfig, readFile FileReader) error {
	if cfg.PublicKeyFile != "" {
		content, err := readFile(cfg.PublicKeyFile)
		if err == nil {
			cfg.PublicKey = strings.TrimSpace(string(content))
		}
	}
	if cfg.PrivateKeyFile != "" {
		content, err := readFile(cfg.PrivateKeyFile)
		if err == nil {
			cfg.PrivateKey = strings.TrimSpace(string(content))
		}
	}
	return nil
}

func parseEd25519VerifyKeyPEM(raw []byte) (ed25519.PublicKey, error) {
	block, rest := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	if len(bytes.TrimSpace(rest)) > 0 {
		return nil, fmt.Errorf("unexpected trailing data after PEM block")
	}
	if block.Type != "PUBLIC KEY" {
		return nil, fmt.Errorf("PEM block type must be PUBLIC KEY, got %q", block.Type)
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse SPKI public key: %w", err)
	}
	edPub, ok := pub.(ed25519.PublicKey)
	if !ok || len(edPub) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("public key must be Ed25519, got %T", pub)
	}
	return edPub, nil
}

// loadBackendTokenVerifyKey loads the SPKI Ed25519 public key from
// BACKEND_TOKEN_VERIFY_KEY_FILE. Called only by ValidateBackendConfig so
// binaries that never verify user tokens neither require nor read the key.
func loadBackendTokenVerifyKey(cfg *AuthConfig, readFile FileReader) error {
	path := strings.TrimSpace(cfg.BackendTokenVerifyKeyFile)
	if path == "" {
		path = strings.TrimSpace(os.Getenv(backendTokenVerifyKeyFileEnv))
	}
	if path == "" {
		if len(cfg.BackendTokenPublicKey) > 0 && len(cfg.BackendTokenPublicKey) != ed25519.PublicKeySize {
			return fmt.Errorf("%s: invalid Ed25519 public key length %d", backendTokenVerifyKeyFileEnv, len(cfg.BackendTokenPublicKey))
		}
		return nil
	}
	cleanPath := filepath.Clean(path)
	content, err := readFile(cleanPath)
	if err != nil {
		return fmt.Errorf("read %s %s: %w", backendTokenVerifyKeyFileEnv, cleanPath, err)
	}
	pub, err := parseEd25519VerifyKeyPEM(content)
	if err != nil {
		return fmt.Errorf("parse %s %s: %w", backendTokenVerifyKeyFileEnv, cleanPath, err)
	}
	cfg.BackendTokenPublicKey = pub
	return nil
}

// loadInternalAuthSecret loads the shared bearer from file if configured.
//
// Same shape for the /internal shared bearer, and for the same reason: an
// empty result is not a disabled feature. cmd/datahub would then send an
// empty X-Internal-Auth and auth-hub would answer 401 to every
// GetSystemUser call while both containers stayed healthy.
func loadInternalAuthSecret(cfg *AuthConfig, readFile FileReader) error {
	if cfg.InternalAuthSecretFile != "" {
		content, err := readFile(cfg.InternalAuthSecretFile)
		if err != nil {
			return fmt.Errorf("read INTERNAL_AUTH_SECRET_FILE %s: %w",
				cfg.InternalAuthSecretFile, err)
		}
		secret := strings.TrimSpace(string(content))
		if secret == "" {
			return fmt.Errorf("INTERNAL_AUTH_SECRET_FILE=%s resolved to an empty secret: "+
				"auth-hub would reject every /internal/system-user call with 401 while the container stayed healthy; "+
				"mount a non-empty secret or set INTERNAL_AUTH_SECRET instead",
				cfg.InternalAuthSecretFile)
		}
		cfg.InternalAuthSecret = secret
	}
	return nil
}

// loadRAGAPIToken loads RAG API token from file if configured.
func loadRAGAPIToken(cfg *RAGConfig, readFile FileReader) error {
	if cfg.APITokenFile != "" {
		content, err := readFile(cfg.APITokenFile)
		if err != nil {
			return fmt.Errorf("read RAG_API_TOKEN_FILE %s: %w",
				cfg.APITokenFile, err)
		}
		secret := strings.TrimSpace(string(content))
		if secret == "" {
			return fmt.Errorf("RAG_API_TOKEN_FILE=%s resolved to an empty secret: "+
				"mount a non-empty secret or set RAG_API_TOKEN instead",
				cfg.APITokenFile)
		}
		cfg.APIToken = secret
	}
	return nil
}

// loadSovereignSecret loads the event caller authentication token for Knowledge Sovereign from file.
func loadSovereignSecret(cfg *SovereignConfig, readFile FileReader) error {
	var envName string
	switch cfg.EventTokenFile {
	case os.Getenv("SOVEREIGN_BACKEND_TOKEN_FILE"):
		envName = "SOVEREIGN_BACKEND_TOKEN_FILE"
	case os.Getenv("SOVEREIGN_HARVESTER_TOKEN_FILE"):
		envName = "SOVEREIGN_HARVESTER_TOKEN_FILE"
	case os.Getenv("SOVEREIGN_DATAHUB_TOKEN_FILE"):
		envName = "SOVEREIGN_DATAHUB_TOKEN_FILE"
	default:
		envName = "SOVEREIGN_EVENT_TOKEN_FILE"
	}
	content, err := readFile(cfg.EventTokenFile)
	if err != nil {
		return fmt.Errorf("read %s %s: %w", envName, cfg.EventTokenFile, err)
	}
	token := strings.TrimSpace(string(content))
	if len(token) < minSovereignEventTokenLen {
		return fmt.Errorf("event token from %s must be at least %d characters", envName, minSovereignEventTokenLen)
	}
	cfg.EventToken = token
	return nil
}

// loadMQHubAuthToken loads the authentication token for mq-hub from file if configured.
func loadMQHubAuthToken(cfg *MQHubConfig, readFile FileReader) error {
	if cfg.AuthTokenFile != "" {
		content, err := readFile(cfg.AuthTokenFile)
		if err != nil {
			return fmt.Errorf("read MQHUB_AUTH_TOKEN_FILE %s: %w", cfg.AuthTokenFile, err)
		}
		secret := strings.TrimSpace(string(content))
		if secret == "" {
			return fmt.Errorf("MQHUB_AUTH_TOKEN_FILE=%s resolved to an empty token: "+
				"mount a non-empty secret or set MQHUB_AUTH_TOKEN instead",
				cfg.AuthTokenFile)
		}
		cfg.AuthToken = secret
	}
	return nil
}

// loadSecrets loads and populates all secret-file references using the injected file reader.
func loadSecrets(cfg *Config, readFile FileReader) error {
	if err := loadImageProxySecret(&cfg.ImageProxy, readFile); err != nil {
		return err
	}
	if err := loadWebPushSecrets(&cfg.WebPush, readFile); err != nil {
		return err
	}
	if err := loadInternalAuthSecret(&cfg.Auth, readFile); err != nil {
		return err
	}
	if err := loadRAGAPIToken(&cfg.Rag, readFile); err != nil {
		return err
	}
	if err := loadMQHubAuthToken(&cfg.MQHub, readFile); err != nil {
		return err
	}
	return nil
}
