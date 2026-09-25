package config

import (
	"fmt"
	"strings"
)

const minSovereignEventTokenLen = 24

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

// loadBackendTokenSecret loads backend token secret from file if configured (Docker Secrets support).
//
// Unlike the two blocks above this one does NOT fall back to the env value
// on failure. middleware/jwt_middleware.go and
// connect/v2/middleware/auth_interceptor.go verify every browser token
// against this secret, and an empty one rejects all of them: the container
// stays healthy, /health answers 200, and every authenticated REST and
// Connect call answers 401 with nothing in the log to say why. That cannot
// be a fallback state, so a mounted-but-unreadable or mounted-but-empty
// secret exits non-zero here (CLAUDE.md rule 9).
//
// The guard is deliberately environment-independent. validateAuthConfig
// only requires the secret when APP_ENV == "production", and APP_ENV is set
// in no compose file — the same trap validateImageProxyConfig documents.
func loadBackendTokenSecret(cfg *AuthConfig, readFile FileReader) error {
	if cfg.BackendTokenSecretFile != "" {
		content, err := readFile(cfg.BackendTokenSecretFile)
		if err != nil {
			return fmt.Errorf("read BACKEND_TOKEN_SECRET_FILE %s: %w",
				cfg.BackendTokenSecretFile, err)
		}
		secret := strings.TrimSpace(string(content))
		if secret == "" {
			return fmt.Errorf("BACKEND_TOKEN_SECRET_FILE=%s resolved to an empty secret: "+
				"every authenticated request would be rejected with 401 while the container stayed healthy; "+
				"mount a non-empty secret or set BACKEND_TOKEN_SECRET instead",
				cfg.BackendTokenSecretFile)
		}
		cfg.BackendTokenSecret = secret
	}
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
	content, err := readFile(cfg.EventTokenFile)
	if err != nil {
		return fmt.Errorf("read SOVEREIGN_EVENT_TOKEN_FILE %s: %w", cfg.EventTokenFile, err)
	}
	token := strings.TrimSpace(string(content))
	if len(token) < minSovereignEventTokenLen {
		return fmt.Errorf("event token from SOVEREIGN_EVENT_TOKEN_FILE must be at least %d characters", minSovereignEventTokenLen)
	}
	cfg.EventToken = token
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
	if err := loadBackendTokenSecret(&cfg.Auth, readFile); err != nil {
		return err
	}
	if err := loadInternalAuthSecret(&cfg.Auth, readFile); err != nil {
		return err
	}
	if err := loadRAGAPIToken(&cfg.Rag, readFile); err != nil {
		return err
	}
	return nil
}
