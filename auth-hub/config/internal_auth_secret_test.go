package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func baseValidConfig(t *testing.T) *Config {
	t.Helper()
	return &Config{
		KratosURL:              "http://kratos:4433",
		Port:                   "8888",
		CacheTTL:               60 * time.Second,
		BackendTokenTTL:        5 * time.Minute,
		CSRFSecret:             "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
		BackendTokenSigningKey: generateTestEd25519Key(t),
		InternalAuthSecret:     "this-is-a-distinct-internal-auth-secret-32-chars",
	}
}

func TestConfig_Validate_InternalAuthSecret(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(*Config)
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid internal auth secret",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseValidConfig(t)
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

func TestLoad_InternalAuthSecret_HasNoDefault(t *testing.T) {
	keyFile, _ := writeEd25519SigningKeyFile(t)
	os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
	os.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", keyFile)
	os.Unsetenv("INTERNAL_AUTH_SECRET")
	defer func() {
		os.Unsetenv("CSRF_SECRET")
		os.Unsetenv("BACKEND_TOKEN_SIGNING_KEY_FILE")
	}()

	_, err := Load()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "INTERNAL_AUTH_SECRET is required")
}

func TestLoad_InternalAuthSecret_FromEnv(t *testing.T) {
	keyFile, _ := writeEd25519SigningKeyFile(t)
	os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
	os.Setenv("BACKEND_TOKEN_SIGNING_KEY_FILE", keyFile)
	os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
	defer func() {
		os.Unsetenv("CSRF_SECRET")
		os.Unsetenv("BACKEND_TOKEN_SIGNING_KEY_FILE")
		os.Unsetenv("INTERNAL_AUTH_SECRET")
	}()

	cfg, err := Load()

	require.NoError(t, err)
	assert.Equal(t, "this-is-a-distinct-internal-auth-secret-32-chars", cfg.InternalAuthSecret)
}
