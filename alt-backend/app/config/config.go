package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// NewConfig creates a new configuration by loading from environment variables
// with fallback to default values
func NewConfig() (*Config, error) {
	config := &Config{}

	if err := loadFromEnvironment(config); err != nil {
		return nil, err
	}

	if err := validateConfig(config); err != nil {
		return nil, err
	}

	if err := loadSecrets(config, os.ReadFile); err != nil {
		return nil, err
	}

	// The two secrets exist precisely so the signing key stays inside the
	// process. Handing both jobs one value again is the exposure this split
	// removed, so it exits non-zero instead of degrading back to it. The guard
	// is environment-independent for the same reason the one above is: APP_ENV
	// is set in no compose file.
	if config.Auth.InternalAuthSecret != "" &&
		config.Auth.InternalAuthSecret == config.Auth.BackendTokenSecret {
		return nil, fmt.Errorf("INTERNAL_AUTH_SECRET must not equal BACKEND_TOKEN_SECRET: " +
			"the /internal shared bearer travels in a plaintext X-Internal-Auth header and reaches " +
			"access logs and OTel span attributes, which is no place for the JWT signing key")
	}

	// Set defaults for JWT issuer and audience if not provided
	if config.Auth.BackendTokenIssuer == "" {
		config.Auth.BackendTokenIssuer = "auth-hub"
	}
	if config.Auth.BackendTokenAudience == "" {
		config.Auth.BackendTokenAudience = "alt-backend"
	}

	if err := validateAppEnv(config.AppEnv); err != nil {
		return nil, fmt.Errorf("app env validation failed: %w", err)
	}
	slog.Info("app_env_resolved", "app_env", config.AppEnv)

	// Validate the image proxy after secrets are loaded. This guard is
	// deliberately environment-independent: keying it on APP_ENV made it dead
	// code, because APP_ENV is set in no compose file.
	if err := validateImageProxyConfig(&config.ImageProxy); err != nil {
		return nil, fmt.Errorf("image proxy config validation failed: %w", err)
	}

	// Validate auth configuration after secrets are loaded
	// This ensures fail-fast behavior for misconfigured production deployments
	if err := validateAuthConfig(&config.Auth, config.AppEnv); err != nil {
		return nil, fmt.Errorf("auth config validation failed: %w", err)
	}

	// Validate sovereign event authentication configuration when sovereign is configured.
	if config.Sovereign.URL != "" {
		if err := validateSovereignConfig(&config.Sovereign, config.AppEnv, os.ReadFile); err != nil {
			return nil, fmt.Errorf("sovereign config validation failed: %w", err)
		}
	}

	// Only resolve when coordination is actually configured: cmd/datahub and
	// cmd/notifier share this loader but have no HOST_RATE_LIMITER_REDIS_URL
	// capability, and a binary already running the explicit local mode has
	// nothing to authenticate against. Gating here keeps redis_auth_disabled
	// from firing for a capability the binary never asked for.
	redisPassword, err := resolveCoordinationRedisPassword(config.RateLimit.CoordinationRedisURL)
	if err != nil {
		return nil, fmt.Errorf("resolve redis password: %w", err)
	}
	config.RateLimit.CoordinationRedisPassword = redisPassword

	return config, nil
}

func validateSovereignConfig(cfg *SovereignConfig, appEnv string, readFile FileReader) error {
	if strings.EqualFold(strings.TrimSpace(cfg.EventAuth), "disabled") {
		if appEnv == "production" {
			return fmt.Errorf("SOVEREIGN_EVENT_AUTH=disabled is not permitted in production: knowledge-sovereign caller authentication is mandatory")
		}
		cfg.EventToken = ""
		return nil
	}

	if cfg.EventTokenFile != "" {
		if err := loadSovereignSecret(cfg, readFile); err != nil {
			return err
		}
		return nil
	}

	cfg.EventToken = strings.TrimSpace(cfg.EventToken)
	if cfg.EventToken != "" {
		if len(cfg.EventToken) < minSovereignEventTokenLen {
			return fmt.Errorf("SOVEREIGN_EVENT_TOKEN must be at least %d characters", minSovereignEventTokenLen)
		}
		return nil
	}

	return fmt.Errorf("SOVEREIGN_EVENT_TOKEN_FILE or SOVEREIGN_EVENT_TOKEN is required when SOVEREIGN_URL is set: " +
		"sovereign event caller authentication requires a non-empty token; " +
		"mount a non-empty secret, set SOVEREIGN_EVENT_TOKEN, or set SOVEREIGN_EVENT_AUTH=disabled")
}

// appEnvValues enumerates the environments alt-backend recognises. Anything
// else is a typo, and a typo must not silently buy the development behaviour
// of the environment-keyed guards.
var appEnvValues = []string{"development", "staging", "production"}

func validateAppEnv(appEnv string) error {
	for _, valid := range appEnvValues {
		if appEnv == valid {
			return nil
		}
	}
	return fmt.Errorf("APP_ENV must be one of: %s, got %s",
		strings.Join(appEnvValues, ", "), appEnv)
}

// validateImageProxyConfig enforces CLAUDE.md rule 9 for the OGP image proxy:
// a missing secret is missing required config, so startup exits non-zero
// instead of limping along with the proxy silently unwired.
//
// IMAGE_PROXY_ENABLED defaults to true and compose/core.yaml mounts
// IMAGE_PROXY_SECRET_FILE=/run/secrets/image_proxy_secret, so an empty or
// whitespace-only secret file used to land in di/image_module.go's
// "enabled && !hasSecret" branch, which logged slog.Error and continued —
// leaving every warmer/backfill job no-oping forever. The old panic there was
// keyed on APP_ENV=production, which is set in no compose file and therefore
// never fired.
//
// The explicit opt-out is IMAGE_PROXY_ENABLED=false, which di/image_module.go
// reports as a loud image_proxy_disabled startup log.
func validateImageProxyConfig(config *ImageProxyConfig) error {
	if !config.Enabled {
		return nil
	}

	if strings.TrimSpace(config.Secret) == "" {
		return fmt.Errorf("IMAGE_PROXY_ENABLED=true requires a non-empty secret: " +
			"set IMAGE_PROXY_SECRET, point IMAGE_PROXY_SECRET_FILE at a non-empty file, " +
			"or set IMAGE_PROXY_ENABLED=false to disable the image proxy explicitly")
	}

	return nil
}
