// ABOUTME: This file tests configuration management and environment variable loading
// ABOUTME: Tests config validation, defaults, and error handling for production use
package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TDD RED PHASE: Test configuration loading from environment variables
func TestLoadConfig(t *testing.T) {
	tests := map[string]struct {
		envVars     map[string]string
		expectError bool
		validate    func(*testing.T, *Config)
	}{
		"default values": {
			envVars: map[string]string{},
			validate: func(t *testing.T, c *Config) {
				assert.Equal(t, 9200, c.Server.Port)
				assert.Equal(t, 30*time.Second, c.HTTP.Timeout)
				assert.Equal(t, 3, c.Retry.MaxAttempts)
				assert.Equal(t, 5*time.Second, c.RateLimit.DefaultInterval)
				assert.Equal(t, "Mozilla/5.0 (compatible; AltBot/1.0; +https://alt.example.com/bot)", c.HTTP.UserAgent)
				assert.Equal(t, true, c.Metrics.Enabled)
			},
		},
		"custom values": {
			envVars: map[string]string{
				"SERVER_PORT":                 "8080",
				"HTTP_TIMEOUT":                "60s",
				"RETRY_MAX_ATTEMPTS":          "5",
				"RETRY_BACKOFF_FACTOR":        "3.0",
				"RATE_LIMIT_DEFAULT_INTERVAL": "10s",
				"METRICS_ENABLED":             "false",
			},
			validate: func(t *testing.T, c *Config) {
				assert.Equal(t, 8080, c.Server.Port)
				assert.Equal(t, 60*time.Second, c.HTTP.Timeout)
				assert.Equal(t, 5, c.Retry.MaxAttempts)
				assert.Equal(t, 3.0, c.Retry.BackoffFactor)
				assert.Equal(t, 10*time.Second, c.RateLimit.DefaultInterval)
				assert.Equal(t, false, c.Metrics.Enabled)
			},
		},
		"invalid port": {
			envVars: map[string]string{
				"SERVER_PORT": "70000",
			},
			expectError: true,
		},
		"invalid timeout": {
			envVars: map[string]string{
				"HTTP_TIMEOUT": "invalid",
			},
			expectError: true,
		},
		"invalid retry attempts": {
			envVars: map[string]string{
				"RETRY_MAX_ATTEMPTS": "-1",
			},
			expectError: true,
		},
		"invalid backoff factor": {
			envVars: map[string]string{
				"RETRY_BACKOFF_FACTOR": "0.5",
			},
			expectError: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// 環境変数設定
			for key, value := range tc.envVars {
				_ = os.Setenv(key, value)
				defer func(k string) {
					_ = os.Unsetenv(k)
				}(key)
			}

			config, err := LoadConfig()

			if tc.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, config)
			tc.validate(t, config)
		})
	}
}

// TDD RED PHASE: Test configuration validation
func TestValidateConfig(t *testing.T) {
	tests := map[string]struct {
		config      *Config
		expectError bool
		errorMsg    string
	}{
		"valid config": {
			config: &Config{
				Server: ServerConfig{Port: 9200},
				HTTP:   HTTPConfig{Timeout: 30 * time.Second},
				Retry: RetryConfig{
					MaxAttempts:   3,
					BackoffFactor: 2.0,
				},
				RateLimit: RateLimitConfig{DefaultInterval: 5 * time.Second},
				Metrics:   MetricsConfig{Port: 9201},
				NewsCreator: NewsCreatorConfig{
					Host:    "http://news-creator:11434",
					APIPath: "/api/generate",
					Model:   "gemma3:4b",
					Timeout: 60 * time.Second,
				},
				SummarizeQueue: SummarizeQueueConfig{
					WorkerInterval:  10 * time.Second,
					MaxRetries:      3,
					PollingInterval: 5 * time.Second,
				},
			},
			expectError: false,
		},
		"invalid port zero": {
			config: &Config{
				Server: ServerConfig{Port: 0},
			},
			expectError: true,
			errorMsg:    "invalid server port",
		},
		"invalid port high": {
			config: &Config{
				Server: ServerConfig{Port: 70000},
			},
			expectError: true,
			errorMsg:    "invalid server port",
		},
		"invalid timeout": {
			config: &Config{
				Server: ServerConfig{Port: 9200},
				HTTP:   HTTPConfig{Timeout: 0},
			},
			expectError: true,
			errorMsg:    "HTTP timeout must be positive",
		},
		"invalid retry attempts": {
			config: &Config{
				Server: ServerConfig{Port: 9200},
				HTTP:   HTTPConfig{Timeout: 30 * time.Second},
				Retry:  RetryConfig{MaxAttempts: 0},
			},
			expectError: true,
			errorMsg:    "retry max attempts must be positive",
		},
		"invalid backoff factor": {
			config: &Config{
				Server: ServerConfig{Port: 9200},
				HTTP:   HTTPConfig{Timeout: 30 * time.Second},
				Retry: RetryConfig{
					MaxAttempts:   3,
					BackoffFactor: 0.5,
				},
			},
			expectError: true,
			errorMsg:    "backoff factor must be greater than 1.0",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := validateConfig(tc.config)

			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TDD RED PHASE: Test ConfigManager functionality
func TestConfigManager(t *testing.T) {
	t.Run("should create config manager", func(t *testing.T) {
		config := &Config{
			Server:    ServerConfig{Port: 9200},
			HTTP:      HTTPConfig{Timeout: 30 * time.Second},
			RateLimit: RateLimitConfig{DefaultInterval: 5 * time.Second},
			Metrics:   MetricsConfig{Port: 9201},
		}

		manager := NewConfigManager(config, nil)

		assert.NotNil(t, manager)
		retrievedConfig := manager.GetConfig()
		assert.Equal(t, 9200, retrievedConfig.Server.Port)
		assert.Equal(t, 30*time.Second, retrievedConfig.HTTP.Timeout)
	})

	t.Run("should update config safely", func(t *testing.T) {
		originalConfig := &Config{
			Server:    ServerConfig{Port: 9200},
			HTTP:      HTTPConfig{Timeout: 30 * time.Second},
			Retry:     RetryConfig{MaxAttempts: 3, BackoffFactor: 2.0},
			RateLimit: RateLimitConfig{DefaultInterval: 5 * time.Second},
			Metrics:   MetricsConfig{Port: 9201},
			NewsCreator: NewsCreatorConfig{
				Host:    "http://news-creator:11434",
				APIPath: "/api/generate",
				Model:   "gemma3:4b",
				Timeout: 60 * time.Second,
			},
			SummarizeQueue: SummarizeQueueConfig{
				WorkerInterval:  10 * time.Second,
				MaxRetries:      3,
				PollingInterval: 5 * time.Second,
			},
		}

		manager := NewConfigManager(originalConfig, nil)

		newConfig := &Config{
			Server:    ServerConfig{Port: 8080},
			HTTP:      HTTPConfig{Timeout: 60 * time.Second},
			Retry:     RetryConfig{MaxAttempts: 5, BackoffFactor: 3.0},
			RateLimit: RateLimitConfig{DefaultInterval: 10 * time.Second},
			Metrics:   MetricsConfig{Port: 9202},
			NewsCreator: NewsCreatorConfig{
				Host:    "http://news-creator:11434",
				APIPath: "/api/generate",
				Model:   "gemma3:4b",
				Timeout: 60 * time.Second,
			},
			SummarizeQueue: SummarizeQueueConfig{
				WorkerInterval:  10 * time.Second,
				MaxRetries:      3,
				PollingInterval: 5 * time.Second,
			},
		}

		err := manager.UpdateConfig(newConfig)
		require.NoError(t, err)

		retrievedConfig := manager.GetConfig()
		assert.Equal(t, 8080, retrievedConfig.Server.Port)
		assert.Equal(t, 60*time.Second, retrievedConfig.HTTP.Timeout)
		assert.Equal(t, 5, retrievedConfig.Retry.MaxAttempts)
	})

	t.Run("should reject invalid config update", func(t *testing.T) {
		originalConfig := &Config{
			Server:    ServerConfig{Port: 9200},
			HTTP:      HTTPConfig{Timeout: 30 * time.Second},
			Retry:     RetryConfig{MaxAttempts: 3, BackoffFactor: 2.0},
			RateLimit: RateLimitConfig{DefaultInterval: 5 * time.Second},
			Metrics:   MetricsConfig{Port: 9201},
			NewsCreator: NewsCreatorConfig{
				Host:    "http://news-creator:11434",
				APIPath: "/api/generate",
				Model:   "gemma3:4b",
				Timeout: 60 * time.Second,
			},
		}

		manager := NewConfigManager(originalConfig, nil)

		invalidConfig := &Config{
			Server: ServerConfig{Port: 0}, // Invalid port
		}

		err := manager.UpdateConfig(invalidConfig)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "new config validation failed")

		// Original config should remain unchanged
		retrievedConfig := manager.GetConfig()
		assert.Equal(t, 9200, retrievedConfig.Server.Port)
	})
}

// TDD RED PHASE: Test environment variable parsing
func TestLoadFromEnv(t *testing.T) {
	t.Run("should handle missing environment variables", func(t *testing.T) {
		config := &Config{}

		err := loadFromEnv(config)
		require.NoError(t, err)

		// Should have default values
		assert.Equal(t, 9200, config.Server.Port)
		assert.Equal(t, 30*time.Second, config.HTTP.Timeout)
	})

	t.Run("should parse all supported environment variables", func(t *testing.T) {
		envVars := map[string]string{
			"SERVER_PORT":                 "8080",
			"SERVER_SHUTDOWN_TIMEOUT":     "45s",
			"HTTP_TIMEOUT":                "60s",
			"HTTP_MAX_IDLE_CONNS":         "20",
			"RETRY_MAX_ATTEMPTS":          "5",
			"RETRY_BASE_DELAY":            "2s",
			"RETRY_MAX_DELAY":             "60s",
			"RETRY_BACKOFF_FACTOR":        "3.0",
			"RETRY_JITTER_FACTOR":         "0.2",
			"RATE_LIMIT_DEFAULT_INTERVAL": "10s",
			"RATE_LIMIT_BURST_SIZE":       "2",
			"RATE_LIMIT_ENABLE_ADAPTIVE":  "true",
			"METRICS_ENABLED":             "false",
			"METRICS_PORT":                "9202",
			"METRICS_UPDATE_INTERVAL":     "5s",
		}

		for key, value := range envVars {
			_ = os.Setenv(key, value)
			defer func(k string) {
				_ = os.Unsetenv(k)
			}(key)
		}

		config := &Config{}
		err := loadFromEnv(config)
		require.NoError(t, err)

		assert.Equal(t, 8080, config.Server.Port)
		assert.Equal(t, 45*time.Second, config.Server.ShutdownTimeout)
		assert.Equal(t, 60*time.Second, config.HTTP.Timeout)
		assert.Equal(t, 20, config.HTTP.MaxIdleConns)
		assert.Equal(t, 5, config.Retry.MaxAttempts)
		assert.Equal(t, 2*time.Second, config.Retry.BaseDelay)
		assert.Equal(t, 60*time.Second, config.Retry.MaxDelay)
		assert.Equal(t, 3.0, config.Retry.BackoffFactor)
		assert.Equal(t, 0.2, config.Retry.JitterFactor)
		assert.Equal(t, 10*time.Second, config.RateLimit.DefaultInterval)
		assert.Equal(t, 2, config.RateLimit.BurstSize)
		assert.Equal(t, true, config.RateLimit.EnableAdaptive)
		assert.Equal(t, false, config.Metrics.Enabled)
		assert.Equal(t, 9202, config.Metrics.Port)
		assert.Equal(t, 5*time.Second, config.Metrics.UpdateInterval)
	})
}
