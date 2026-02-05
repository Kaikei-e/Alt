package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConfig_Defaults(t *testing.T) {
	// Clear environment
	os.Clearenv()

	cfg := NewConfig()

	assert.Equal(t, "9200", cfg.Port)
	assert.Equal(t, "http://alt-backend:9101", cfg.BackendConnectURL)
	assert.Equal(t, "http://auth-hub:8888", cfg.AuthHubURL)
	assert.Equal(t, "auth-hub", cfg.BackendTokenIssuer)
	assert.Equal(t, "alt-backend", cfg.BackendTokenAudience)
	assert.Equal(t, 30*time.Second, cfg.RequestTimeout)
	assert.Equal(t, 5*time.Minute, cfg.StreamingTimeout)
}

func TestNewConfig_FromEnvironment(t *testing.T) {
	// Set environment variables
	os.Setenv("BFF_PORT", "8080")
	os.Setenv("BACKEND_CONNECT_URL", "http://localhost:9101")
	os.Setenv("AUTH_HUB_INTERNAL_URL", "http://localhost:8888")
	os.Setenv("BACKEND_TOKEN_ISSUER", "custom-issuer")
	os.Setenv("BACKEND_TOKEN_AUDIENCE", "custom-audience")
	os.Setenv("BFF_REQUEST_TIMEOUT", "60s")
	os.Setenv("BFF_STREAMING_TIMEOUT", "10m")
	defer os.Clearenv()

	cfg := NewConfig()

	assert.Equal(t, "8080", cfg.Port)
	assert.Equal(t, "http://localhost:9101", cfg.BackendConnectURL)
	assert.Equal(t, "http://localhost:8888", cfg.AuthHubURL)
	assert.Equal(t, "custom-issuer", cfg.BackendTokenIssuer)
	assert.Equal(t, "custom-audience", cfg.BackendTokenAudience)
	assert.Equal(t, 60*time.Second, cfg.RequestTimeout)
	assert.Equal(t, 10*time.Minute, cfg.StreamingTimeout)
}

func TestNewConfig_InvalidDuration_UsesDefault(t *testing.T) {
	os.Setenv("BFF_REQUEST_TIMEOUT", "invalid")
	os.Setenv("BFF_STREAMING_TIMEOUT", "also-invalid")
	defer os.Clearenv()

	cfg := NewConfig()

	assert.Equal(t, 30*time.Second, cfg.RequestTimeout)
	assert.Equal(t, 5*time.Minute, cfg.StreamingTimeout)
}

func TestConfig_LoadBackendTokenSecret_FromFile(t *testing.T) {
	// Create temporary secret file
	tmpFile, err := os.CreateTemp("", "backend_token_secret")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString("my-secret-key")
	require.NoError(t, err)
	tmpFile.Close()

	os.Setenv("BACKEND_TOKEN_SECRET_FILE", tmpFile.Name())
	defer os.Clearenv()

	cfg := NewConfig()
	secret, err := cfg.LoadBackendTokenSecret()

	require.NoError(t, err)
	assert.Equal(t, []byte("my-secret-key"), secret)
}

func TestConfig_LoadBackendTokenSecret_FromEnv(t *testing.T) {
	os.Setenv("BACKEND_TOKEN_SECRET", "env-secret-key")
	defer os.Clearenv()

	cfg := NewConfig()
	secret, err := cfg.LoadBackendTokenSecret()

	require.NoError(t, err)
	assert.Equal(t, []byte("env-secret-key"), secret)
}

func TestConfig_LoadBackendTokenSecret_FileNotFound(t *testing.T) {
	os.Setenv("BACKEND_TOKEN_SECRET_FILE", "/nonexistent/path")
	defer os.Clearenv()

	cfg := NewConfig()
	_, err := cfg.LoadBackendTokenSecret()

	assert.Error(t, err)
}

func TestConfig_LoadBackendTokenSecret_NoSecret(t *testing.T) {
	os.Clearenv()

	cfg := NewConfig()
	_, err := cfg.LoadBackendTokenSecret()

	assert.Error(t, err)
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
	}{
		{
			name:    "valid config",
			modify:  func(c *Config) {},
			wantErr: false,
		},
		{
			name:    "empty port",
			modify:  func(c *Config) { c.Port = "" },
			wantErr: true,
		},
		{
			name:    "empty backend URL",
			modify:  func(c *Config) { c.BackendConnectURL = "" },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			cfg := NewConfig()
			tt.modify(cfg)

			err := cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNewConfig_BFFFeatureFlags_Defaults(t *testing.T) {
	os.Clearenv()

	cfg := NewConfig()

	// All features enabled by default
	assert.True(t, cfg.EnableCache)
	assert.True(t, cfg.EnableCircuitBreaker)
	assert.True(t, cfg.EnableDedup)
	assert.True(t, cfg.EnableErrorNormalization)

	// Hardcoded cache configuration
	assert.Equal(t, 1000, cfg.CacheMaxSize)
	assert.Equal(t, 30*time.Second, cfg.CacheDefaultTTL)

	// Hardcoded circuit breaker configuration
	assert.Equal(t, 5, cfg.CBFailureThreshold)
	assert.Equal(t, 2, cfg.CBSuccessThreshold)
	assert.Equal(t, 30*time.Second, cfg.CBOpenTimeout)

	// Hardcoded dedup configuration
	assert.Equal(t, 100*time.Millisecond, cfg.DedupWindow)
}
