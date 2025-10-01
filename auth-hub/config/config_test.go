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
			name: "default configuration when no env vars set",
			setupEnv: func() {
				// Clear all relevant env vars
				os.Unsetenv("KRATOS_URL")
				os.Unsetenv("PORT")
				os.Unsetenv("CACHE_TTL")
			},
			cleanupEnv: func() {},
			expected: &Config{
				KratosURL: "http://kratos:4433",
				Port:      "8888",
				CacheTTL:  5 * time.Minute,
			},
			wantErr: false,
		},
		{
			name: "custom configuration from environment variables",
			setupEnv: func() {
				os.Setenv("KRATOS_URL", "http://custom-kratos:4444")
				os.Setenv("PORT", "9999")
				os.Setenv("CACHE_TTL", "10m")
			},
			cleanupEnv: func() {
				os.Unsetenv("KRATOS_URL")
				os.Unsetenv("PORT")
				os.Unsetenv("CACHE_TTL")
			},
			expected: &Config{
				KratosURL: "http://custom-kratos:4444",
				Port:      "9999",
				CacheTTL:  10 * time.Minute,
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
				os.Unsetenv("PORT")
				os.Unsetenv("CACHE_TTL")
			},
			cleanupEnv: func() {
				os.Unsetenv("KRATOS_URL")
			},
			expected: &Config{
				KratosURL: "http://localhost:4433",
				Port:      "8888",
				CacheTTL:  5 * time.Minute,
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
				KratosURL: "http://kratos:4433",
				Port:      "8888",
				CacheTTL:  5 * time.Minute,
			},
			wantErr: false,
		},
		{
			name: "missing Kratos URL",
			config: &Config{
				KratosURL: "",
				Port:      "8888",
				CacheTTL:  5 * time.Minute,
			},
			wantErr:     true,
			errContains: "KRATOS_URL",
		},
		{
			name: "missing port",
			config: &Config{
				KratosURL: "http://kratos:4433",
				Port:      "",
				CacheTTL:  5 * time.Minute,
			},
			wantErr:     true,
			errContains: "PORT",
		},
		{
			name: "invalid cache TTL (zero)",
			config: &Config{
				KratosURL: "http://kratos:4433",
				Port:      "8888",
				CacheTTL:  0,
			},
			wantErr:     true,
			errContains: "CACHE_TTL",
		},
		{
			name: "invalid cache TTL (negative)",
			config: &Config{
				KratosURL: "http://kratos:4433",
				Port:      "8888",
				CacheTTL:  -1 * time.Minute,
			},
			wantErr:     true,
			errContains: "CACHE_TTL",
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
