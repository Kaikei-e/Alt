package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// baseValidConfig is a config that passes Validate once the /internal shared
// bearer is separated from the JWT signing key.
func baseValidConfig() *Config {
	return &Config{
		KratosURL:          "http://kratos:4433",
		Port:               "8888",
		CacheTTL:           5 * time.Minute,
		CSRFSecret:         "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
		BackendTokenSecret: "this-is-a-valid-backend-token-secret-32-chars-long",
		InternalAuthSecret: "this-is-a-distinct-internal-auth-secret-32-chars",
	}
}

// The HS256 key that signs backend JWTs must never leave the process, but the
// /internal shared bearer travels in a plaintext X-Internal-Auth header on
// every alt-backend GetSystemUser call — and from there into access logs and
// OTel span attributes. Sharing one value between the two jobs put the signing
// key on that wire, so they are now two separate required secrets.
func TestConfig_Validate_InternalAuthSecret(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(*Config)
		wantErr     bool
		errContains string
	}{
		{
			name:    "distinct internal auth secret is valid",
			mutate:  func(*Config) {},
			wantErr: false,
		},
		{
			name:        "missing internal auth secret",
			mutate:      func(c *Config) { c.InternalAuthSecret = "" },
			wantErr:     true,
			errContains: "INTERNAL_AUTH_SECRET is required",
		},
		{
			name:        "internal auth secret too short",
			mutate:      func(c *Config) { c.InternalAuthSecret = "short-secret" },
			wantErr:     true,
			errContains: "INTERNAL_AUTH_SECRET must be at least 32 characters",
		},
		{
			name:        "internal auth secret reuses the JWT signing key",
			mutate:      func(c *Config) { c.InternalAuthSecret = c.BackendTokenSecret },
			wantErr:     true,
			errContains: "INTERNAL_AUTH_SECRET must not equal BACKEND_TOKEN_SECRET",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseValidConfig()
			tt.mutate(cfg)

			err := cfg.Validate()

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			assert.NoError(t, err)
		})
	}
}

// Rule 9: an unset INTERNAL_AUTH_SECRET exits non-zero instead of falling back
// to BACKEND_TOKEN_SECRET (which is what the single-secret wiring effectively
// did) or to any built-in default.
func TestLoad_InternalAuthSecret_HasNoDefault(t *testing.T) {
	os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
	os.Setenv("BACKEND_TOKEN_SECRET", "this-is-a-valid-backend-token-secret-32-chars-long")
	os.Unsetenv("INTERNAL_AUTH_SECRET")
	defer func() {
		os.Unsetenv("CSRF_SECRET")
		os.Unsetenv("BACKEND_TOKEN_SECRET")
	}()

	_, err := Load()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "INTERNAL_AUTH_SECRET is required")
}

func TestLoad_InternalAuthSecret_RejectsReuseOfSigningKey(t *testing.T) {
	const shared = "this-is-a-valid-backend-token-secret-32-chars-long"
	os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
	os.Setenv("BACKEND_TOKEN_SECRET", shared)
	os.Setenv("INTERNAL_AUTH_SECRET", shared)
	defer func() {
		os.Unsetenv("CSRF_SECRET")
		os.Unsetenv("BACKEND_TOKEN_SECRET")
		os.Unsetenv("INTERNAL_AUTH_SECRET")
	}()

	_, err := Load()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "INTERNAL_AUTH_SECRET must not equal BACKEND_TOKEN_SECRET")
}

func TestLoad_InternalAuthSecret_FromEnv(t *testing.T) {
	os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
	os.Setenv("BACKEND_TOKEN_SECRET", "this-is-a-valid-backend-token-secret-32-chars-long")
	os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
	defer func() {
		os.Unsetenv("CSRF_SECRET")
		os.Unsetenv("BACKEND_TOKEN_SECRET")
		os.Unsetenv("INTERNAL_AUTH_SECRET")
	}()

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "this-is-a-distinct-internal-auth-secret-32-chars", cfg.InternalAuthSecret)
	assert.NotEqual(t, cfg.BackendTokenSecret, cfg.InternalAuthSecret)
}
