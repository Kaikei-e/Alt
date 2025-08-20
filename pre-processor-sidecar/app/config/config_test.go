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
                                // Ensure global HTTPS_PROXY doesn't override default
                                "HTTPS_PROXY":                        "",
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

// createValidConfig creates a valid configuration for testing
func createValidConfig() *Config {
	return &Config{
		// Service configuration - required by validation
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
		Environment:    "development",
		LogLevel:       "info",

		// HTTP Server configuration - required by validation
		HTTPPort:    8080,
		ReadTimeout: 30 * time.Second,

		// HTTP Client configuration - required by validation
		HTTPClient: HTTPClientConfig{
			Timeout:               30 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 15 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConns:          10,
			MaxIdleConnsPerHost:   2,
		},

		// Circuit Breaker configuration - required by validation
		CircuitBreaker: CircuitBreakerConfig{
			FailureThreshold: 3,
			SuccessThreshold: 2,
			Timeout:          60 * time.Second,
			MaxRequests:      1,
		},

		// Retry configuration - required by validation
		Retry: RetryConfig{
			MaxRetries:   3,
			InitialDelay: 5 * time.Second,
			MaxDelay:     30 * time.Second,
			Multiplier:   2.0,
		},

		// Database configuration - required by validation
		Database: DatabaseConfig{
			Host:     "localhost",
			Port:     "5432",
			Name:     "test_db",
			User:     "test_user",
			Password: "test_password",
			SSLMode:  "disable",
		},

		// Inoreader configuration - required by validation
		Inoreader: InoreaderConfig{
			BaseURL:               "https://www.inoreader.com/reader/api/0",
			ClientID:              "test_client_id",
			ClientSecret:          "test_client_secret",
			RefreshToken:          "test_refresh_token",
			MaxArticlesPerRequest: 100,
			TokenRefreshBuffer:    5 * time.Minute,
		},

		// Proxy configuration - required by validation
		Proxy: ProxyConfig{
			HTTPSProxy: "http://proxy:8081",
			NoProxy:    "localhost,127.0.0.1",
		},

		// Rate limiting configuration
		RateLimit: RateLimitConfig{
			DailyLimit:   100,
			SyncInterval: 30 * time.Minute,
		},

		// OAuth2 configuration
		OAuth2: OAuth2Config{
			ClientID:      "test_client_id",
			ClientSecret:  "test_client_secret",
			RefreshToken:  "test_refresh_token",
			RefreshBuffer: 5 * time.Minute,
		},

		// Monitoring configuration
		Monitoring: MonitoringConfig{
			EnableMetrics:     true,
			EnableTracing:     true,
			MetricsBatchSize:  100,
			FlushInterval:     30 * time.Second,
			RetentionDuration: 24 * time.Hour,
		},

		// Feature flags
		EnableScheduleMode: false,
		EnableDebugMode:    false,
		EnableHealthCheck:  true,

		// Token storage
		TokenStoragePath: "/tmp/test_token.env",
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := map[string]struct {
		config      *Config
		expectError bool
		errorMsg    string
	}{
		"valid_config": {
			config:      createValidConfig(),
			expectError: false,
		},
		"missing_db_password": {
			config: func() *Config {
				cfg := createValidConfig()
				cfg.Database.Password = ""
				return cfg
			}(),
			expectError: true,
			errorMsg:    "PRE_PROCESSOR_SIDECAR_DB_PASSWORD is required",
		},
		"missing_oauth_client_id": {
			config: func() *Config {
				cfg := createValidConfig()
				cfg.Inoreader.ClientID = ""
				return cfg
			}(),
			expectError: true,
			errorMsg:    "INOREADER_CLIENT_ID is required",
		},
		"missing_oauth_client_secret": {
			config: func() *Config {
				cfg := createValidConfig()
				cfg.Inoreader.ClientSecret = ""
				return cfg
			}(),
			expectError: true,
			errorMsg:    "INOREADER_CLIENT_SECRET is required",
		},
		"missing_refresh_token": {
			config: func() *Config {
				cfg := createValidConfig()
				cfg.Inoreader.RefreshToken = ""
				return cfg
			}(),
			expectError: false, // RefreshToken now optional - managed by auth-token-manager
		},
		"missing_https_proxy": {
			config: func() *Config {
				cfg := createValidConfig()
				cfg.Proxy.HTTPSProxy = ""
				return cfg
			}(),
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

// TDD Phase 3 - REFACTOR: Enhanced Configuration Management Tests
func TestEnhancedConfigurationManagement(t *testing.T) {
	tests := map[string]struct {
		envVars     map[string]string
		expectError bool
		validate    func(t *testing.T, cfg *Config)
	}{
		"enhanced_config_with_all_features": {
			envVars: map[string]string{
				"SERVICE_NAME":                            "test-sidecar",
				"SERVICE_VERSION":                         "2.0.0",
				"ENVIRONMENT":                             "production",
				"HTTP_PORT":                               "9090",
				"PRE_PROCESSOR_SIDECAR_DB_PASSWORD":       "test_password",
				"INOREADER_CLIENT_ID":                     "test_client_id",
				"INOREADER_CLIENT_SECRET":                 "test_client_secret",
				"INOREADER_REFRESH_TOKEN":                 "test_refresh_token",
				"HTTP_CLIENT_TIMEOUT":                     "90s",
				"RETRY_MAX_RETRIES":                       "5",
				"RETRY_INITIAL_DELAY":                     "10s",
				"CIRCUIT_BREAKER_FAILURE_THRESHOLD":       "5",
				"CIRCUIT_BREAKER_SUCCESS_THRESHOLD":       "3",
				"MONITORING_ENABLE_METRICS":               "false",
				"MONITORING_ENABLE_TRACING":               "false",
				"ENABLE_SCHEDULE_MODE":                    "true",
				"ENABLE_DEBUG_MODE":                       "true",
			},
			expectError: false,
			validate: func(t *testing.T, cfg *Config) {
				assert.Equal(t, "test-sidecar", cfg.ServiceName)
				assert.Equal(t, "2.0.0", cfg.ServiceVersion)
				assert.Equal(t, "production", cfg.Environment)
				assert.Equal(t, 9090, cfg.HTTPPort)
				assert.Equal(t, 90*time.Second, cfg.HTTPClient.Timeout)
				assert.Equal(t, 5, cfg.Retry.MaxRetries)
				assert.Equal(t, 10*time.Second, cfg.Retry.InitialDelay)
				assert.Equal(t, 5, cfg.CircuitBreaker.FailureThreshold)
				assert.Equal(t, 3, cfg.CircuitBreaker.SuccessThreshold)
				assert.False(t, cfg.Monitoring.EnableMetrics)
				assert.False(t, cfg.Monitoring.EnableTracing)
				assert.True(t, cfg.EnableScheduleMode)
				assert.True(t, cfg.EnableDebugMode)
				assert.True(t, cfg.IsProduction())
				assert.False(t, cfg.IsDevelopment())
			},
		},
		"invalid_configuration_validation": {
			envVars: map[string]string{
				"SERVICE_NAME":                            "", // Invalid: empty service name
				"SERVICE_VERSION":                         "1.0.0",
				"HTTP_PORT":                               "99999", // Invalid: port out of range
				"PRE_PROCESSOR_SIDECAR_DB_PASSWORD":       "test_password",
				"INOREADER_CLIENT_ID":                     "test_client_id",
				"INOREADER_CLIENT_SECRET":                 "test_client_secret",
				"INOREADER_REFRESH_TOKEN":                 "test_refresh_token",
				"RETRY_MULTIPLIER":                        "0.5", // Invalid: multiplier <= 1.0
				"CIRCUIT_BREAKER_FAILURE_THRESHOLD":       "-1",  // Invalid: negative threshold
			},
			expectError: true,
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

// TestConfigHelperFunctions tests the new helper functions
func TestConfigHelperFunctions(t *testing.T) {
	tests := []struct {
		name     string
		setup    func()
		teardown func()
		test     func(t *testing.T)
	}{
		{
			name: "getEnvOrDefaultInt",
			setup: func() {
				os.Setenv("TEST_INT", "42")
				os.Setenv("TEST_INVALID_INT", "not_a_number")
			},
			teardown: func() {
				os.Unsetenv("TEST_INT")
				os.Unsetenv("TEST_INVALID_INT")
			},
			test: func(t *testing.T) {
				assert.Equal(t, 42, getEnvOrDefaultInt("TEST_INT", 10))
				assert.Equal(t, 10, getEnvOrDefaultInt("TEST_INVALID_INT", 10))
				assert.Equal(t, 10, getEnvOrDefaultInt("TEST_MISSING_INT", 10))
			},
		},
		{
			name: "getEnvOrDefaultDuration",
			setup: func() {
				os.Setenv("TEST_DURATION", "5m")
				os.Setenv("TEST_INVALID_DURATION", "not_a_duration")
			},
			teardown: func() {
				os.Unsetenv("TEST_DURATION")
				os.Unsetenv("TEST_INVALID_DURATION")
			},
			test: func(t *testing.T) {
				assert.Equal(t, 5*time.Minute, getEnvOrDefaultDuration("TEST_DURATION", time.Hour))
				assert.Equal(t, time.Hour, getEnvOrDefaultDuration("TEST_INVALID_DURATION", time.Hour))
				assert.Equal(t, time.Hour, getEnvOrDefaultDuration("TEST_MISSING_DURATION", time.Hour))
			},
		},
		{
			name: "getEnvOrDefaultBool",
			setup: func() {
				os.Setenv("TEST_BOOL_TRUE", "true")
				os.Setenv("TEST_BOOL_FALSE", "false")
				os.Setenv("TEST_INVALID_BOOL", "not_a_bool")
			},
			teardown: func() {
				os.Unsetenv("TEST_BOOL_TRUE")
				os.Unsetenv("TEST_BOOL_FALSE")
				os.Unsetenv("TEST_INVALID_BOOL")
			},
			test: func(t *testing.T) {
				assert.True(t, getEnvOrDefaultBool("TEST_BOOL_TRUE", false))
				assert.False(t, getEnvOrDefaultBool("TEST_BOOL_FALSE", true))
				assert.True(t, getEnvOrDefaultBool("TEST_INVALID_BOOL", true))
				assert.False(t, getEnvOrDefaultBool("TEST_MISSING_BOOL", false))
			},
		},
		{
			name: "getEnvOrDefaultFloat",
			setup: func() {
				os.Setenv("TEST_FLOAT", "3.14")
				os.Setenv("TEST_INVALID_FLOAT", "not_a_float")
			},
			teardown: func() {
				os.Unsetenv("TEST_FLOAT")
				os.Unsetenv("TEST_INVALID_FLOAT")
			},
			test: func(t *testing.T) {
				assert.Equal(t, 3.14, getEnvOrDefaultFloat("TEST_FLOAT", 2.71))
				assert.Equal(t, 2.71, getEnvOrDefaultFloat("TEST_INVALID_FLOAT", 2.71))
				assert.Equal(t, 2.71, getEnvOrDefaultFloat("TEST_MISSING_FLOAT", 2.71))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}
			if tt.teardown != nil {
				defer tt.teardown()
			}
			tt.test(t)
		})
	}
}

// TestConfigDatabaseConnectionString tests database connection string generation
func TestConfigDatabaseConnectionString(t *testing.T) {
	cfg := &Config{
		Database: DatabaseConfig{
			Host:     "localhost",
			Port:     "5432",
			Name:     "testdb",
			User:     "testuser",
			Password: "testpass",
			SSLMode:  "disable",
		},
	}

	expected := "host=localhost port=5432 dbname=testdb user=testuser password=testpass sslmode=disable"
	actual := cfg.GetDatabaseConnectionString()
	
	assert.Equal(t, expected, actual)
}