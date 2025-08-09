// ABOUTME: This file tests configuration loading and validation
// ABOUTME: Ensures proper environment variable parsing and required field validation

package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	tests := map[string]struct {
		envVars     map[string]string
		expectError bool
		validate    func(t *testing.T, cfg *Config)
	}{
		"valid_full_config": {
			envVars: map[string]string{
				"SERVICE_NAME":                         "test-sidecar",
				"LOG_LEVEL":                           "debug",
				"PRE_PROCESSOR_SIDECAR_DB_PASSWORD":   "test_password",
				"INOREADER_CLIENT_ID":                 "test_client_id",
				"INOREADER_CLIENT_SECRET":             "test_client_secret",
				"INOREADER_REFRESH_TOKEN":             "test_refresh_token",
				"MAX_ARTICLES_PER_REQUEST":            "50",
				"SYNC_INTERVAL":                       "15m",
				"OAUTH2_TOKEN_REFRESH_BUFFER":         "300",
			},
			expectError: false,
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "test-sidecar", cfg.ServiceName)
				assert.Equal(t, "debug", cfg.LogLevel)
				assert.Equal(t, "test_password", cfg.Database.Password)
				assert.Equal(t, "test_client_id", cfg.Inoreader.ClientID)
				assert.Equal(t, "test_client_secret", cfg.Inoreader.ClientSecret)
				assert.Equal(t, "test_refresh_token", cfg.Inoreader.RefreshToken)
				assert.Equal(t, 50, cfg.Inoreader.MaxArticlesPerRequest)
				assert.Equal(t, 15*time.Minute, cfg.RateLimit.SyncInterval)
				assert.Equal(t, 5*time.Minute, cfg.Inoreader.TokenRefreshBuffer)
			},
		},
		"missing_required_db_password": {
			envVars: map[string]string{
				"INOREADER_CLIENT_ID":     "test_client_id",
				"INOREADER_CLIENT_SECRET": "test_client_secret",
				"INOREADER_REFRESH_TOKEN": "test_refresh_token",
			},
			expectError: true,
		},
		"missing_required_oauth_credentials": {
			envVars: map[string]string{
				"PRE_PROCESSOR_SIDECAR_DB_PASSWORD": "test_password",
			},
			expectError: true,
		},
		"default_values": {
			envVars: map[string]string{
				"PRE_PROCESSOR_SIDECAR_DB_PASSWORD": "test_password",
				"INOREADER_CLIENT_ID":               "test_client_id",
				"INOREADER_CLIENT_SECRET":           "test_client_secret",
				"INOREADER_REFRESH_TOKEN":           "test_refresh_token",
			},
			expectError: false,
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "pre-processor-sidecar", cfg.ServiceName)
				assert.Equal(t, "info", cfg.LogLevel)
				assert.Equal(t, 100, cfg.Inoreader.MaxArticlesPerRequest)
				assert.Equal(t, 30*time.Minute, cfg.RateLimit.SyncInterval)
				assert.Equal(t, 5*time.Minute, cfg.Inoreader.TokenRefreshBuffer)
				assert.Equal(t, "https://www.inoreader.com/reader/api/0", cfg.Inoreader.BaseURL)
				assert.Equal(t, "http://envoy-proxy.alt-apps.svc.cluster.local:8081", cfg.Proxy.HTTPSProxy)
			},
		},
		"invalid_integer_parsing": {
			envVars: map[string]string{
				"PRE_PROCESSOR_SIDECAR_DB_PASSWORD": "test_password",
				"INOREADER_CLIENT_ID":               "test_client_id",
				"INOREADER_CLIENT_SECRET":           "test_client_secret",
				"INOREADER_REFRESH_TOKEN":           "test_refresh_token",
				"MAX_ARTICLES_PER_REQUEST":          "invalid_number",
				"OAUTH2_TOKEN_REFRESH_BUFFER":       "invalid_duration",
			},
			expectError: false,
			validate: func(t *testing.T, cfg *Config) {
				// Should fallback to defaults for invalid values
				assert.Equal(t, 100, cfg.Inoreader.MaxArticlesPerRequest)
				assert.Equal(t, 5*time.Minute, cfg.Inoreader.TokenRefreshBuffer)
			},
		},
		"invalid_duration_parsing": {
			envVars: map[string]string{
				"PRE_PROCESSOR_SIDECAR_DB_PASSWORD": "test_password",
				"INOREADER_CLIENT_ID":               "test_client_id",
				"INOREADER_CLIENT_SECRET":           "test_client_secret",
				"INOREADER_REFRESH_TOKEN":           "test_refresh_token",
				"SYNC_INTERVAL":                     "invalid_duration",
			},
			expectError: false,
			validate: func(t *testing.T, cfg *Config) {
				// Should fallback to default for invalid duration
				assert.Equal(t, 30*time.Minute, cfg.RateLimit.SyncInterval)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Backup original environment
			originalEnv := make(map[string]string)
			for key := range tc.envVars {
				originalEnv[key] = os.Getenv(key)
			}

			// Set test environment variables
			for key, value := range tc.envVars {
				os.Setenv(key, value)
			}

			// Restore original environment after test
			defer func() {
				for key := range tc.envVars {
					if originalValue, exists := originalEnv[key]; exists {
						os.Setenv(key, originalValue)
					} else {
						os.Unsetenv(key)
					}
				}
			}()

			cfg, err := LoadConfig()

			if tc.expectError {
				require.Error(t, err)
				assert.Nil(t, cfg)
			} else {
				require.NoError(t, err)
				require.NotNil(t, cfg)
				if tc.validate != nil {
					tc.validate(t, cfg)
				}
			}
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := map[string]struct {
		config      *Config
		expectError bool
		errorMsg    string
	}{
		"valid_config": {
			config: &Config{
				Database: DatabaseConfig{
					Password: "test_password",
				},
				Inoreader: InoreaderConfig{
					ClientID:     "test_client_id",
					ClientSecret: "test_client_secret",
					RefreshToken: "test_refresh_token",
				},
				Proxy: ProxyConfig{
					HTTPSProxy: "http://proxy:8081",
				},
			},
			expectError: false,
		},
		"missing_db_password": {
			config: &Config{
				Database: DatabaseConfig{
					Password: "",
				},
				Inoreader: InoreaderConfig{
					ClientID:     "test_client_id",
					ClientSecret: "test_client_secret",
					RefreshToken: "test_refresh_token",
				},
				Proxy: ProxyConfig{
					HTTPSProxy: "http://proxy:8081",
				},
			},
			expectError: true,
			errorMsg:    "PRE_PROCESSOR_SIDECAR_DB_PASSWORD is required",
		},
		"missing_oauth_client_id": {
			config: &Config{
				Database: DatabaseConfig{
					Password: "test_password",
				},
				Inoreader: InoreaderConfig{
					ClientID:     "",
					ClientSecret: "test_client_secret",
					RefreshToken: "test_refresh_token",
				},
				Proxy: ProxyConfig{
					HTTPSProxy: "http://proxy:8081",
				},
			},
			expectError: true,
			errorMsg:    "INOREADER_CLIENT_ID is required",
		},
		"missing_oauth_client_secret": {
			config: &Config{
				Database: DatabaseConfig{
					Password: "test_password",
				},
				Inoreader: InoreaderConfig{
					ClientID:     "test_client_id",
					ClientSecret: "",
					RefreshToken: "test_refresh_token",
				},
				Proxy: ProxyConfig{
					HTTPSProxy: "http://proxy:8081",
				},
			},
			expectError: true,
			errorMsg:    "INOREADER_CLIENT_SECRET is required",
		},
		"missing_refresh_token": {
			config: &Config{
				Database: DatabaseConfig{
					Password: "test_password",
				},
				Inoreader: InoreaderConfig{
					ClientID:     "test_client_id",
					ClientSecret: "test_client_secret",
					RefreshToken: "",
				},
				Proxy: ProxyConfig{
					HTTPSProxy: "http://proxy:8081",
				},
			},
			expectError: true,
			errorMsg:    "INOREADER_REFRESH_TOKEN is required",
		},
		"missing_https_proxy": {
			config: &Config{
				Database: DatabaseConfig{
					Password: "test_password",
				},
				Inoreader: InoreaderConfig{
					ClientID:     "test_client_id",
					ClientSecret: "test_client_secret",
					RefreshToken: "test_refresh_token",
				},
				Proxy: ProxyConfig{
					HTTPSProxy: "",
				},
			},
			expectError: true,
			errorMsg:    "HTTPS_PROXY is required for Envoy integration",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := tc.config.Validate()

			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGetEnvOrDefault(t *testing.T) {
	tests := map[string]struct {
		envKey       string
		envValue     string
		defaultValue string
		expected     string
	}{
		"env_var_exists": {
			envKey:       "TEST_VAR",
			envValue:     "test_value",
			defaultValue: "default_value",
			expected:     "test_value",
		},
		"env_var_missing": {
			envKey:       "MISSING_VAR",
			envValue:     "",
			defaultValue: "default_value",
			expected:     "default_value",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Backup original environment
			originalValue := os.Getenv(tc.envKey)
			defer func() {
				if originalValue != "" {
					os.Setenv(tc.envKey, originalValue)
				} else {
					os.Unsetenv(tc.envKey)
				}
			}()

			if tc.envValue != "" {
				os.Setenv(tc.envKey, tc.envValue)
			} else {
				os.Unsetenv(tc.envKey)
			}

			result := getEnvOrDefault(tc.envKey, tc.defaultValue)
			assert.Equal(t, tc.expected, result)
		})
	}
}