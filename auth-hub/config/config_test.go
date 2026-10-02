package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name        string
		setupEnv    func()
		cleanupEnv  func()
		expected    *Config
		wantErr     bool
		errContains string
	}{
		{
			name: "default configuration when no env vars set (missing CSRF_SECRET)",
			setupEnv: func() {
				// Clear all relevant env vars
				os.Unsetenv("KRATOS_URL")
				os.Unsetenv("PORT")
				os.Unsetenv("CACHE_TTL")
				os.Unsetenv("CSRF_SECRET")
				os.Unsetenv("INTERNAL_AUTH_SECRET")
			},
			cleanupEnv:  func() {},
			expected:    nil,
			wantErr:     true,
			errContains: "CSRF_SECRET is required",
		},
		{
			name: "custom configuration from environment variables",
			setupEnv: func() {
				os.Setenv("KRATOS_URL", "http://custom-kratos:4444")
				os.Setenv("PORT", "9999")
				os.Setenv("CACHE_TTL", "30s")
				os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
				os.Setenv("BACKEND_TOKEN_SECRET", "this-is-a-valid-backend-token-secret-32-chars-long")
				os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
			},
			cleanupEnv: func() {
				os.Unsetenv("KRATOS_URL")
				os.Unsetenv("PORT")
				os.Unsetenv("CACHE_TTL")
				os.Unsetenv("CSRF_SECRET")
				os.Unsetenv("BACKEND_TOKEN_SECRET")
				os.Unsetenv("INTERNAL_AUTH_SECRET")
			},
			expected: &Config{
				KratosURL:  "http://custom-kratos:4444",
				Port:       "9999",
				CacheTTL:   30 * time.Second,
				CSRFSecret: "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
			},
			wantErr: false,
		},
		{
			name: "invalid cache TTL format returns error",
			setupEnv: func() {
				os.Setenv("CACHE_TTL", "invalid")
			},
			cleanupEnv: func() {
				os.Unsetenv("CACHE_TTL")
			},
			expected:    nil,
			wantErr:     true,
			errContains: "invalid CACHE_TTL",
		},
		{
			name: "partial configuration with defaults",
			setupEnv: func() {
				os.Setenv("KRATOS_URL", "http://localhost:4433")
				os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
				os.Setenv("BACKEND_TOKEN_SECRET", "this-is-a-valid-backend-token-secret-32-chars-long")
				os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
				os.Unsetenv("PORT")
				os.Unsetenv("CACHE_TTL")
			},
			cleanupEnv: func() {
				os.Unsetenv("KRATOS_URL")
				os.Unsetenv("CSRF_SECRET")
				os.Unsetenv("BACKEND_TOKEN_SECRET")
				os.Unsetenv("INTERNAL_AUTH_SECRET")
			},
			expected: &Config{
				KratosURL:  "http://localhost:4433",
				Port:       "8888",
				CacheTTL:   60 * time.Second,
				CSRFSecret: "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
			},
			wantErr: false,
		},
		{
			name: "INTROSPECT_RATE_LIMIT=NaN is rejected",
			setupEnv: func() {
				os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
				os.Setenv("BACKEND_TOKEN_SECRET", "this-is-a-valid-backend-token-secret-32-chars-long")
				os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
				os.Setenv("INTROSPECT_RATE_LIMIT", "NaN")
			},
			cleanupEnv: func() {
				os.Unsetenv("CSRF_SECRET")
				os.Unsetenv("BACKEND_TOKEN_SECRET")
				os.Unsetenv("INTERNAL_AUTH_SECRET")
				os.Unsetenv("INTROSPECT_RATE_LIMIT")
			},
			expected:    nil,
			wantErr:     true,
			errContains: "must be a finite positive number",
		},
		{
			name: "INTROSPECT_RATE_LIMIT=+Inf is rejected",
			setupEnv: func() {
				os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
				os.Setenv("BACKEND_TOKEN_SECRET", "this-is-a-valid-backend-token-secret-32-chars-long")
				os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
				os.Setenv("INTROSPECT_RATE_LIMIT", "+Inf")
			},
			cleanupEnv: func() {
				os.Unsetenv("CSRF_SECRET")
				os.Unsetenv("BACKEND_TOKEN_SECRET")
				os.Unsetenv("INTERNAL_AUTH_SECRET")
				os.Unsetenv("INTROSPECT_RATE_LIMIT")
			},
			expected:    nil,
			wantErr:     true,
			errContains: "must be a finite positive number",
		},
		{
			name: "INTROSPECT_RATE_LIMIT=-Inf is rejected",
			setupEnv: func() {
				os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
				os.Setenv("BACKEND_TOKEN_SECRET", "this-is-a-valid-backend-token-secret-32-chars-long")
				os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
				os.Setenv("INTROSPECT_RATE_LIMIT", "-Inf")
			},
			cleanupEnv: func() {
				os.Unsetenv("CSRF_SECRET")
				os.Unsetenv("BACKEND_TOKEN_SECRET")
				os.Unsetenv("INTERNAL_AUTH_SECRET")
				os.Unsetenv("INTROSPECT_RATE_LIMIT")
			},
			expected:    nil,
			wantErr:     true,
			errContains: "must be a finite positive number",
		},
		{
			name: "INTROSPECT_RATE_LIMIT=20.0 is accepted",
			setupEnv: func() {
				os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
				os.Setenv("BACKEND_TOKEN_SECRET", "this-is-a-valid-backend-token-secret-32-chars-long")
				os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
				os.Setenv("INTROSPECT_RATE_LIMIT", "20.0")
			},
			cleanupEnv: func() {
				os.Unsetenv("CSRF_SECRET")
				os.Unsetenv("BACKEND_TOKEN_SECRET")
				os.Unsetenv("INTERNAL_AUTH_SECRET")
				os.Unsetenv("INTROSPECT_RATE_LIMIT")
			},
			expected: &Config{
				KratosURL:           "http://kratos:4433",
				Port:                "8888",
				CacheTTL:            60 * time.Second,
				CSRFSecret:          "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
				IntrospectRateLimit: 20.0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			tt.setupEnv()
			defer tt.cleanupEnv()

			// Execute
			got, err := Load()

			// Assert
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, got)
			assert.Equal(t, tt.expected.KratosURL, got.KratosURL)
			assert.Equal(t, tt.expected.Port, got.Port)
			assert.Equal(t, tt.expected.CacheTTL, got.CacheTTL)
		})
	}
}

func TestLoad_KratosAdminURL_Default(t *testing.T) {
	os.Unsetenv("KRATOS_ADMIN_URL")
	os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
	os.Setenv("BACKEND_TOKEN_SECRET", "this-is-a-valid-backend-token-secret-32-chars-long")
	os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
	defer func() {
		os.Unsetenv("CSRF_SECRET")
		os.Unsetenv("BACKEND_TOKEN_SECRET")
		os.Unsetenv("INTERNAL_AUTH_SECRET")
	}()

	cfg, err := Load()
	assert.NoError(t, err)
	assert.Equal(t, "http://kratos-admin:4434", cfg.KratosAdminURL)
}

func TestLoad_KratosAdminURL_EnvOverride(t *testing.T) {
	os.Setenv("KRATOS_ADMIN_URL", "http://custom-kratos-admin:4434")
	os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
	os.Setenv("BACKEND_TOKEN_SECRET", "this-is-a-valid-backend-token-secret-32-chars-long")
	os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
	defer func() {
		os.Unsetenv("KRATOS_ADMIN_URL")
		os.Unsetenv("CSRF_SECRET")
		os.Unsetenv("BACKEND_TOKEN_SECRET")
		os.Unsetenv("INTERNAL_AUTH_SECRET")
	}()

	cfg, err := Load()
	assert.NoError(t, err)
	assert.Equal(t, "http://custom-kratos-admin:4434", cfg.KratosAdminURL)
}

func TestLoad_ValidateRateLimit_Default(t *testing.T) {
	os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
	os.Setenv("BACKEND_TOKEN_SECRET", "this-is-a-valid-backend-token-secret-32-chars-long")
	os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
	defer func() {
		os.Unsetenv("CSRF_SECRET")
		os.Unsetenv("BACKEND_TOKEN_SECRET")
		os.Unsetenv("INTERNAL_AUTH_SECRET")
		os.Unsetenv("VALIDATE_RATE_LIMIT")
	}()

	cfg, err := Load()
	assert.NoError(t, err)
	assert.InDelta(t, 100.0/60.0, cfg.ValidateRateLimit, 0.001)
}

func TestLoad_ValidateRateLimit_EnvOverride(t *testing.T) {
	os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
	os.Setenv("BACKEND_TOKEN_SECRET", "this-is-a-valid-backend-token-secret-32-chars-long")
	os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
	os.Setenv("VALIDATE_RATE_LIMIT", "50.0")
	defer func() {
		os.Unsetenv("CSRF_SECRET")
		os.Unsetenv("BACKEND_TOKEN_SECRET")
		os.Unsetenv("INTERNAL_AUTH_SECRET")
		os.Unsetenv("VALIDATE_RATE_LIMIT")
	}()

	cfg, err := Load()
	assert.NoError(t, err)
	assert.InDelta(t, 50.0, cfg.ValidateRateLimit, 0.001)
}

func TestLoad_ValidateRateLimit_Invalid(t *testing.T) {
	os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
	os.Setenv("BACKEND_TOKEN_SECRET", "this-is-a-valid-backend-token-secret-32-chars-long")
	os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
	os.Setenv("VALIDATE_RATE_LIMIT", "not-a-number")
	defer func() {
		os.Unsetenv("CSRF_SECRET")
		os.Unsetenv("BACKEND_TOKEN_SECRET")
		os.Unsetenv("INTERNAL_AUTH_SECRET")
		os.Unsetenv("VALIDATE_RATE_LIMIT")
	}()

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid VALIDATE_RATE_LIMIT")
}

func TestLoad_CSRFRateLimit_Default(t *testing.T) {
	os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
	os.Setenv("BACKEND_TOKEN_SECRET", "this-is-a-valid-backend-token-secret-32-chars-long")
	os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
	defer func() {
		os.Unsetenv("CSRF_SECRET")
		os.Unsetenv("BACKEND_TOKEN_SECRET")
		os.Unsetenv("INTERNAL_AUTH_SECRET")
		os.Unsetenv("CSRF_RATE_LIMIT")
	}()

	cfg, err := Load()
	assert.NoError(t, err)
	assert.InDelta(t, 100.0, cfg.CSRFRateLimit, 0.001)
}

func TestLoad_CSRFRateLimit_EnvOverride(t *testing.T) {
	os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
	os.Setenv("BACKEND_TOKEN_SECRET", "this-is-a-valid-backend-token-secret-32-chars-long")
	os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
	os.Setenv("CSRF_RATE_LIMIT", "50.0")
	defer func() {
		os.Unsetenv("CSRF_SECRET")
		os.Unsetenv("BACKEND_TOKEN_SECRET")
		os.Unsetenv("INTERNAL_AUTH_SECRET")
		os.Unsetenv("CSRF_RATE_LIMIT")
	}()

	cfg, err := Load()
	assert.NoError(t, err)
	assert.InDelta(t, 50.0, cfg.CSRFRateLimit, 0.001)
}

func TestLoad_CSRFRateLimit_Invalid(t *testing.T) {
	os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
	os.Setenv("BACKEND_TOKEN_SECRET", "this-is-a-valid-backend-token-secret-32-chars-long")
	os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
	os.Setenv("CSRF_RATE_LIMIT", "not-a-number")
	defer func() {
		os.Unsetenv("CSRF_SECRET")
		os.Unsetenv("BACKEND_TOKEN_SECRET")
		os.Unsetenv("INTERNAL_AUTH_SECRET")
		os.Unsetenv("CSRF_RATE_LIMIT")
	}()

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid CSRF_RATE_LIMIT")
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		wantErr     bool
		errContains string
	}{
		{
			name: "valid configuration",
			config: &Config{
				KratosURL:          "http://kratos:4433",
				Port:               "8888",
				CacheTTL:           60 * time.Second,
				BackendTokenTTL:    5 * time.Minute,
				CSRFSecret:         "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
				BackendTokenSecret: "this-is-a-valid-backend-token-secret-32-chars-long",
				InternalAuthSecret: "this-is-a-distinct-internal-auth-secret-32-chars",
			},
			wantErr: false,
		},
		{
			name: "missing Kratos URL",
			config: &Config{
				KratosURL:          "",
				Port:               "8888",
				CacheTTL:           60 * time.Second,
				BackendTokenTTL:    5 * time.Minute,
				CSRFSecret:         "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
				BackendTokenSecret: "this-is-a-valid-backend-token-secret-32-chars-long",
				InternalAuthSecret: "this-is-a-distinct-internal-auth-secret-32-chars",
			},
			wantErr:     true,
			errContains: "KRATOS_URL",
		},
		{
			name: "missing port",
			config: &Config{
				KratosURL:          "http://kratos:4433",
				Port:               "",
				CacheTTL:           60 * time.Second,
				BackendTokenTTL:    5 * time.Minute,
				CSRFSecret:         "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
				BackendTokenSecret: "this-is-a-valid-backend-token-secret-32-chars-long",
				InternalAuthSecret: "this-is-a-distinct-internal-auth-secret-32-chars",
			},
			wantErr:     true,
			errContains: "PORT",
		},
		{
			name: "invalid cache TTL (zero)",
			config: &Config{
				KratosURL:          "http://kratos:4433",
				Port:               "8888",
				CacheTTL:           0,
				BackendTokenTTL:    5 * time.Minute,
				CSRFSecret:         "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
				BackendTokenSecret: "this-is-a-valid-backend-token-secret-32-chars-long",
				InternalAuthSecret: "this-is-a-distinct-internal-auth-secret-32-chars",
			},
			wantErr:     true,
			errContains: "CACHE_TTL",
		},
		{
			name: "invalid cache TTL (negative)",
			config: &Config{
				KratosURL:          "http://kratos:4433",
				Port:               "8888",
				CacheTTL:           -1 * time.Minute,
				BackendTokenTTL:    5 * time.Minute,
				CSRFSecret:         "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
				BackendTokenSecret: "this-is-a-valid-backend-token-secret-32-chars-long",
				InternalAuthSecret: "this-is-a-distinct-internal-auth-secret-32-chars",
			},
			wantErr:     true,
			errContains: "CACHE_TTL",
		},
		{
			name: "missing CSRF secret",
			config: &Config{
				KratosURL:          "http://kratos:4433",
				Port:               "8888",
				CacheTTL:           60 * time.Second,
				BackendTokenTTL:    5 * time.Minute,
				CSRFSecret:         "",
				BackendTokenSecret: "this-is-a-valid-backend-token-secret-32-chars-long",
				InternalAuthSecret: "this-is-a-distinct-internal-auth-secret-32-chars",
			},
			wantErr:     true,
			errContains: "CSRF_SECRET",
		},
		{
			name: "CSRF secret too short",
			config: &Config{
				KratosURL:          "http://kratos:4433",
				Port:               "8888",
				CacheTTL:           60 * time.Second,
				BackendTokenTTL:    5 * time.Minute,
				CSRFSecret:         "short-secret",
				BackendTokenSecret: "this-is-a-valid-backend-token-secret-32-chars-long",
				InternalAuthSecret: "this-is-a-distinct-internal-auth-secret-32-chars",
			},
			wantErr:     true,
			errContains: "CSRF_SECRET must be at least 32 characters",
		},
		{
			name: "valid CSRF secret",
			config: &Config{
				KratosURL:          "http://kratos:4433",
				Port:               "8888",
				CacheTTL:           60 * time.Second,
				BackendTokenTTL:    5 * time.Minute,
				CSRFSecret:         "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
				BackendTokenSecret: "this-is-a-valid-backend-token-secret-32-chars-long",
				InternalAuthSecret: "this-is-a-distinct-internal-auth-secret-32-chars",
			},
			wantErr: false,
		},
		{
			name: "missing backend token secret",
			config: &Config{
				KratosURL:          "http://kratos:4433",
				Port:               "8888",
				CacheTTL:           60 * time.Second,
				BackendTokenTTL:    5 * time.Minute,
				CSRFSecret:         "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
				BackendTokenSecret: "",
				InternalAuthSecret: "this-is-a-distinct-internal-auth-secret-32-chars",
			},
			wantErr:     true,
			errContains: "BACKEND_TOKEN_SECRET",
		},
		{
			name: "backend token secret too short",
			config: &Config{
				KratosURL:          "http://kratos:4433",
				Port:               "8888",
				CacheTTL:           60 * time.Second,
				BackendTokenTTL:    5 * time.Minute,
				CSRFSecret:         "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
				BackendTokenSecret: "short-secret",
				InternalAuthSecret: "this-is-a-distinct-internal-auth-secret-32-chars",
			},
			wantErr:     true,
			errContains: "BACKEND_TOKEN_SECRET must be at least 32 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfig_BoundedSessionCacheAndJWT(t *testing.T) {
	// Test Load() caps CacheTTL to <= 60s
	t.Run("Load caps CACHE_TTL to 60s", func(t *testing.T) {
		os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
		os.Setenv("BACKEND_TOKEN_SECRET", "this-is-a-valid-backend-token-secret-32-chars-long")
		os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
		os.Setenv("CACHE_TTL", "10m")
		defer func() {
			os.Unsetenv("CSRF_SECRET")
			os.Unsetenv("BACKEND_TOKEN_SECRET")
			os.Unsetenv("INTERNAL_AUTH_SECRET")
			os.Unsetenv("CACHE_TTL")
		}()

		cfg, err := Load()
		assert.NoError(t, err)
		assert.Equal(t, 60*time.Second, cfg.CacheTTL, "CACHE_TTL must be capped to <= 60s")
	})

	// Test default CacheTTL is <= 60s
	t.Run("default CacheTTL is 60s", func(t *testing.T) {
		os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
		os.Setenv("BACKEND_TOKEN_SECRET", "this-is-a-valid-backend-token-secret-32-chars-long")
		os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
		os.Unsetenv("CACHE_TTL")
		defer func() {
			os.Unsetenv("CSRF_SECRET")
			os.Unsetenv("BACKEND_TOKEN_SECRET")
			os.Unsetenv("INTERNAL_AUTH_SECRET")
		}()

		cfg, err := Load()
		assert.NoError(t, err)
		assert.Equal(t, 60*time.Second, cfg.CacheTTL, "Default CacheTTL must be 60s")
	})

	// Test Load() caps BackendTokenTTL to <= 5m
	t.Run("Load caps BACKEND_TOKEN_TTL to 5m", func(t *testing.T) {
		os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
		os.Setenv("BACKEND_TOKEN_SECRET", "this-is-a-valid-backend-token-secret-32-chars-long")
		os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
		os.Setenv("BACKEND_TOKEN_TTL", "30m")
		defer func() {
			os.Unsetenv("CSRF_SECRET")
			os.Unsetenv("BACKEND_TOKEN_SECRET")
			os.Unsetenv("INTERNAL_AUTH_SECRET")
			os.Unsetenv("BACKEND_TOKEN_TTL")
		}()

		cfg, err := Load()
		assert.NoError(t, err)
		assert.Equal(t, 5*time.Minute, cfg.BackendTokenTTL, "BACKEND_TOKEN_TTL must be capped to <= 5m")
	})

	// Test default BackendTokenTTL is <= 5m
	t.Run("default BackendTokenTTL is 5m", func(t *testing.T) {
		os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
		os.Setenv("BACKEND_TOKEN_SECRET", "this-is-a-valid-backend-token-secret-32-chars-long")
		os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
		os.Unsetenv("BACKEND_TOKEN_TTL")
		defer func() {
			os.Unsetenv("CSRF_SECRET")
			os.Unsetenv("BACKEND_TOKEN_SECRET")
			os.Unsetenv("INTERNAL_AUTH_SECRET")
		}()

		cfg, err := Load()
		assert.NoError(t, err)
		assert.Equal(t, 5*time.Minute, cfg.BackendTokenTTL, "Default BackendTokenTTL must be 5m")
	})

	// Test Validate() bounds
	t.Run("Validate enforces bounds and positive values", func(t *testing.T) {
		baseCfg := func() *Config {
			return &Config{
				KratosURL:          "http://kratos:4433",
				Port:               "8888",
				CacheTTL:           60 * time.Second,
				BackendTokenTTL:    5 * time.Minute,
				CSRFSecret:         "this-is-a-valid-csrf-secret-that-is-at-least-32-chars",
				BackendTokenSecret: "this-is-a-valid-backend-token-secret-32-chars-long",
				InternalAuthSecret: "this-is-a-distinct-internal-auth-secret-32-chars",
			}
		}

		// Valid
		assert.NoError(t, baseCfg().Validate())

		// CacheTTL > 60s
		c := baseCfg()
		c.CacheTTL = 61 * time.Second
		err := c.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "CACHE_TTL cannot exceed 60s")

		// BackendTokenTTL <= 0
		c = baseCfg()
		c.BackendTokenTTL = 0
		err = c.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "BACKEND_TOKEN_TTL must be positive")

		// BackendTokenTTL > 5m
		c = baseCfg()
		c.BackendTokenTTL = 6 * time.Minute
		err = c.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "BACKEND_TOKEN_TTL cannot exceed 5m")

		// IntrospectRateLimit < 0
		c = baseCfg()
		c.IntrospectRateLimit = -1
		err = c.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "INTROSPECT_RATE_LIMIT cannot be negative")

		// IntrospectBurst < 0
		c = baseCfg()
		c.IntrospectBurst = -1
		err = c.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "INTROSPECT_BURST cannot be negative")
	})

	t.Run("introspect rate limit env parsing", func(t *testing.T) {
		os.Setenv("CSRF_SECRET", "this-is-a-valid-csrf-secret-that-is-at-least-32-chars")
		os.Setenv("BACKEND_TOKEN_SECRET", "this-is-a-valid-backend-token-secret-32-chars-long")
		os.Setenv("INTERNAL_AUTH_SECRET", "this-is-a-distinct-internal-auth-secret-32-chars")
		os.Setenv("INTROSPECT_RATE_LIMIT", "25.5")
		os.Setenv("INTROSPECT_BURST", "150")
		defer func() {
			os.Unsetenv("CSRF_SECRET")
			os.Unsetenv("BACKEND_TOKEN_SECRET")
			os.Unsetenv("INTERNAL_AUTH_SECRET")
			os.Unsetenv("INTROSPECT_RATE_LIMIT")
			os.Unsetenv("INTROSPECT_BURST")
		}()

		cfg, err := Load()
		assert.NoError(t, err)
		assert.Equal(t, 25.5, cfg.IntrospectRateLimit)
		assert.Equal(t, 150, cfg.IntrospectBurst)
	})
}
