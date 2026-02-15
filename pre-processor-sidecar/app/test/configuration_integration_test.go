// TDD Phase 3 - REFACTOR: Configuration Integration Test
package test

import (
	"os"
	"testing"
	"time"

	"pre-processor-sidecar/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfigurationIntegration tests the complete configuration management system
func TestConfigurationIntegration(t *testing.T) {
	// Backup original environment
	originalEnv := make(map[string]string)
	testEnvVars := map[string]string{
		"SERVICE_NAME":                      "pre-processor-sidecar",
		"SERVICE_VERSION":                   "2.0.0",
		"ENVIRONMENT":                       "production",
		"HTTP_PORT":                         "8080",
		"PRE_PROCESSOR_SIDECAR_DB_PASSWORD": "secure_password",
		"INOREADER_CLIENT_ID":               "production_client_id",
		"INOREADER_CLIENT_SECRET":           "production_client_secret",
		"INOREADER_REFRESH_TOKEN":           "production_refresh_token",
		"HTTP_CLIENT_TIMEOUT":               "90s",
		"RETRY_MAX_RETRIES":                 "5",
		"RETRY_INITIAL_DELAY":               "10s",
		"CIRCUIT_BREAKER_FAILURE_THRESHOLD": "5",
		"CIRCUIT_BREAKER_SUCCESS_THRESHOLD": "3",
		"MONITORING_ENABLE_METRICS":         "true",
		"MONITORING_ENABLE_TRACING":         "true",
		"ENABLE_SCHEDULE_MODE":              "true",
		"ENABLE_DEBUG_MODE":                 "false",
		"DB_HOST":                          "postgres.alt-database.svc.cluster.local",
		"DB_NAME":                          "alt",
		"PRE_PROCESSOR_SIDECAR_DB_USER":    "pre_processor_sidecar_user",
	}

	for key := range testEnvVars {
		originalEnv[key] = os.Getenv(key)
	}

	// Set test environment variables
	for key, value := range testEnvVars {
		os.Setenv(key, value)
	}

	// Restore original environment after test
	defer func() {
		for key := range testEnvVars {
			if originalValue, exists := originalEnv[key]; exists {
				os.Setenv(key, originalValue)
			} else {
				os.Unsetenv(key)
			}
		}
	}()

	// Load configuration
	cfg, err := config.LoadConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify service configuration
	assert.Equal(t, "pre-processor-sidecar", cfg.ServiceName)
	assert.Equal(t, "2.0.0", cfg.ServiceVersion)
	assert.Equal(t, "production", cfg.Environment)
	assert.True(t, cfg.IsProduction())
	assert.False(t, cfg.IsDevelopment())

	// Verify HTTP configuration
	assert.Equal(t, 8080, cfg.HTTPPort)
	assert.Equal(t, 90*time.Second, cfg.HTTPClient.Timeout)

	// Verify retry configuration
	assert.Equal(t, 5, cfg.Retry.MaxRetries)
	assert.Equal(t, 10*time.Second, cfg.Retry.InitialDelay)

	// Verify circuit breaker configuration
	assert.Equal(t, 5, cfg.CircuitBreaker.FailureThreshold)
	assert.Equal(t, 3, cfg.CircuitBreaker.SuccessThreshold)

	// Verify monitoring configuration
	assert.True(t, cfg.Monitoring.EnableMetrics)
	assert.True(t, cfg.Monitoring.EnableTracing)

	// Verify feature flags
	assert.True(t, cfg.EnableScheduleMode)
	assert.False(t, cfg.EnableDebugMode)

	// Verify database connection string generation
	expectedConnStr := "host=postgres.alt-database.svc.cluster.local port=5432 dbname=alt user=pre_processor_sidecar_user password=secure_password sslmode=disable"
	actualConnStr := cfg.GetDatabaseConnectionString()
	assert.Equal(t, expectedConnStr, actualConnStr)

	t.Logf("Configuration loaded and validated successfully")
	t.Logf("Service: %s v%s (%s)", cfg.ServiceName, cfg.ServiceVersion, cfg.Environment)
	t.Logf("HTTP Client Timeout: %v", cfg.HTTPClient.Timeout)
	t.Logf("Circuit Breaker: %d failures -> OPEN, %d successes -> CLOSED",
		cfg.CircuitBreaker.FailureThreshold, cfg.CircuitBreaker.SuccessThreshold)
	t.Logf("Monitoring: metrics=%v, tracing=%v",
		cfg.Monitoring.EnableMetrics, cfg.Monitoring.EnableTracing)
}

// TestConfigurationValidationEdgeCases tests edge cases in configuration validation
func TestConfigurationValidationEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		modifier    func(*config.Config)
		expectError bool
		errorMsg    string
	}{
		{
			name: "invalid_http_port_zero",
			modifier: func(cfg *config.Config) {
				cfg.HTTPPort = 0
			},
			expectError: true,
			errorMsg:    "HTTP_PORT must be between 1 and 65535",
		},
		{
			name: "invalid_http_port_too_high",
			modifier: func(cfg *config.Config) {
				cfg.HTTPPort = 99999
			},
			expectError: true,
			errorMsg:    "HTTP_PORT must be between 1 and 65535",
		},
		{
			name: "invalid_retry_multiplier",
			modifier: func(cfg *config.Config) {
				cfg.Retry.Multiplier = 1.0
			},
			expectError: true,
			errorMsg:    "RETRY_MULTIPLIER must be greater than 1.0",
		},
		{
			name: "invalid_initial_delay_greater_than_max",
			modifier: func(cfg *config.Config) {
				cfg.Retry.InitialDelay = 60 * time.Second
				cfg.Retry.MaxDelay = 30 * time.Second
			},
			expectError: true,
			errorMsg:    "RETRY_INITIAL_DELAY must be less than or equal to RETRY_MAX_DELAY",
		},
		{
			name: "valid_boundary_values",
			modifier: func(cfg *config.Config) {
				cfg.HTTPPort = 65535
				cfg.Retry.Multiplier = 1.1
				cfg.CircuitBreaker.FailureThreshold = 1
				cfg.CircuitBreaker.SuccessThreshold = 1
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a valid config
			cfg := &config.Config{
				ServiceName:    "test-service",
				ServiceVersion: "1.0.0",
				HTTPPort:       8080,
				HTTPClient: config.HTTPClientConfig{
					Timeout: 30 * time.Second,
				},
				CircuitBreaker: config.CircuitBreakerConfig{
					FailureThreshold: 3,
					SuccessThreshold: 2,
					Timeout:          60 * time.Second,
				},
				Retry: config.RetryConfig{
					MaxRetries:   3,
					InitialDelay: 5 * time.Second,
					MaxDelay:     30 * time.Second,
					Multiplier:   2.0,
				},
				Database: config.DatabaseConfig{
					Password: "test_password",
				},
				Inoreader: config.InoreaderConfig{
					ClientID:     "test_client_id",
					ClientSecret: "test_client_secret",
					RefreshToken: "test_refresh_token",
				},
				Proxy: config.ProxyConfig{
					HTTPSProxy: "http://proxy:8081",
				},
			}

			// Apply modification
			tt.modifier(cfg)

			// Validate
			err := cfg.Validate()

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
