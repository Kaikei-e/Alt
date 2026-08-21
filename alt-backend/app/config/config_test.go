package config

import (
	"os"
	"testing"
	"time"
)

func TestNewConfig_WithDefaults(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected Config
	}{
		{
			name:    "all defaults",
			envVars: map[string]string{},
			expected: Config{
				Server: ServerConfig{
					Port:         9000,
					ConnectPort:  9101,
					ReadTimeout:  300 * time.Second,
					WriteTimeout: 300 * time.Second,
					IdleTimeout:  120 * time.Second,
				},
				Database: DatabaseConfig{
					MaxConnections:    25,
					ConnectionTimeout: 30 * time.Second,
				},
				RateLimit: RateLimitConfig{
					// 7.5s, deliberately not a whole number of seconds — see
					// external_api_interval_test.go for the floor and the
					// no-truncation guarantees that go with it.
					ExternalAPIInterval: 7500 * time.Millisecond,
					FeedFetchLimit:      100,
				},
				Cache: CacheConfig{
					FeedCacheExpiry:   300 * time.Second,
					SearchCacheExpiry: 900 * time.Second,
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "json",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment first
			clearTestEnv()

			// NewConfig refuses to start with the image proxy enabled (the
			// default) and an empty secret — see validateImageProxyConfig.
			t.Setenv("IMAGE_PROXY_SECRET", "test-image-proxy-secret")

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}
			defer clearTestEnv()

			config, err := NewConfig()
			if err != nil {
				t.Fatalf("NewConfig() failed: %v", err)
			}

			// Verify server config
			if config.Server.Port != tt.expected.Server.Port {
				t.Errorf("Server.Port = %d, want %d", config.Server.Port, tt.expected.Server.Port)
			}
			if config.Server.ReadTimeout != tt.expected.Server.ReadTimeout {
				t.Errorf("Server.ReadTimeout = %v, want %v", config.Server.ReadTimeout, tt.expected.Server.ReadTimeout)
			}

			// Verify database config
			if config.Database.MaxConnections != tt.expected.Database.MaxConnections {
				t.Errorf("Database.MaxConnections = %d, want %d", config.Database.MaxConnections, tt.expected.Database.MaxConnections)
			}

			// Verify rate limit config
			if config.RateLimit.ExternalAPIInterval != tt.expected.RateLimit.ExternalAPIInterval {
				t.Errorf("RateLimit.ExternalAPIInterval = %v, want %v", config.RateLimit.ExternalAPIInterval, tt.expected.RateLimit.ExternalAPIInterval)
			}
		})
	}
}

func TestNewConfig_WithEnvironmentOverrides(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		verify  func(*testing.T, *Config)
	}{
		{
			name: "override server port",
			envVars: map[string]string{
				"SERVER_PORT": "8080",
			},
			verify: func(t *testing.T, config *Config) {
				if config.Server.Port != 8080 {
					t.Errorf("Server.Port = %d, want 8080", config.Server.Port)
				}
			},
		},
		{
			// 10s must stay different from the shipping default (7.5s), or
			// this stops testing the override and starts testing the default.
			name: "override rate limit interval",
			envVars: map[string]string{
				"RATE_LIMIT_EXTERNAL_API_INTERVAL": "10s",
			},
			verify: func(t *testing.T, config *Config) {
				if config.RateLimit.ExternalAPIInterval != 10*time.Second {
					t.Errorf("RateLimit.ExternalAPIInterval = %v, want 10s", config.RateLimit.ExternalAPIInterval)
				}
			},
		},
		{
			name: "override logging level",
			envVars: map[string]string{
				"LOG_LEVEL": "debug",
			},
			verify: func(t *testing.T, config *Config) {
				if config.Logging.Level != "debug" {
					t.Errorf("Logging.Level = %s, want debug", config.Logging.Level)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment first
			clearTestEnv()

			// NewConfig refuses to start with the image proxy enabled (the
			// default) and an empty secret — see validateImageProxyConfig.
			t.Setenv("IMAGE_PROXY_SECRET", "test-image-proxy-secret")

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}
			defer clearTestEnv()

			config, err := NewConfig()
			if err != nil {
				t.Fatalf("NewConfig() failed: %v", err)
			}

			tt.verify(t, config)
		})
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantErr bool
		errMsg  string
	}{
		// SERVER_PORT / CONNECT_PORT range and uniqueness moved out of the
		// shared NewConfig path when alt-backend split into three binaries:
		// only cmd/backend opens those listeners. They are covered by
		// TestValidateBackendListeners in binary_test.go.
		{
			name: "invalid timeout - negative",
			envVars: map[string]string{
				"SERVER_READ_TIMEOUT": "-5s",
			},
			wantErr: true,
			errMsg:  "timeout values must be positive",
		},
		{
			name: "invalid log level",
			envVars: map[string]string{
				"LOG_LEVEL": "invalid",
			},
			wantErr: true,
			errMsg:  "log level must be one of: debug, info, warn, error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment first
			clearTestEnv()

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}
			defer clearTestEnv()

			_, err := NewConfig()
			if tt.wantErr {
				if err == nil {
					t.Error("NewConfig() expected error but got none")
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("NewConfig() error = %v, want to contain %s", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("NewConfig() unexpected error: %v", err)
				}
			}
		})
	}
}

func clearTestEnv() {
	envVars := []string{
		"SERVER_PORT", "SERVER_READ_TIMEOUT", "SERVER_WRITE_TIMEOUT", "SERVER_IDLE_TIMEOUT",
		"DB_MAX_CONNECTIONS", "DB_CONNECTION_TIMEOUT",
		"RATE_LIMIT_EXTERNAL_API_INTERVAL", "RATE_LIMIT_FEED_FETCH_LIMIT",
		"CACHE_FEED_EXPIRY", "CACHE_SEARCH_EXPIRY",
		"LOG_LEVEL", "LOG_FORMAT",
		"PRE_PROCESSOR_ENABLED",
		"APP_ENV",
		"IMAGE_PROXY_ENABLED", "IMAGE_PROXY_SECRET", "IMAGE_PROXY_SECRET_FILE",
	}

	for _, env := range envVars {
		os.Unsetenv(env)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && s[:len(substr)] == substr) ||
		(len(s) > len(substr) && s[len(s)-len(substr):] == substr) ||
		findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
