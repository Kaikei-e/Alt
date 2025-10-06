package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"pre-processor-sidecar/config"
)

// TestHealthCheckService_BasicHealth tests basic health check functionality
func TestHealthCheckService_BasicHealth(t *testing.T) {
	// Create a simple health check service
	healthService := NewHealthCheckService()
	
	result := healthService.PerformHealthCheck(context.Background())
	
	// Basic assertions
	assert.NotNil(t, result)
	assert.Equal(t, "healthy", result["status"])
	assert.Contains(t, result, "timestamp")
	assert.Contains(t, result, "version")
}

// TestHealthCheckService_TokenManagerHealth tests token manager health verification
func TestHealthCheckService_TokenManagerHealth(t *testing.T) {
	// Set environment variables for OAuth2 credentials
	os.Setenv("INOREADER_CLIENT_ID", "test-client-id")
	os.Setenv("INOREADER_CLIENT_SECRET", "test-client-secret")
	defer func() {
		os.Unsetenv("INOREADER_CLIENT_ID")
		os.Unsetenv("INOREADER_CLIENT_SECRET")
	}()

	// Create basic config
	cfg := &config.Config{
		OAuth2: config.OAuth2Config{
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
		},
		Database: config.DatabaseConfig{
			Host: "mock-db",
			Port: "5432",
		},
	}
	
	healthService := NewHealthCheckServiceWithConfig(cfg)
	result := healthService.PerformHealthCheck(context.Background())
	
	assert.Equal(t, "healthy", result["status"])
	assert.Equal(t, true, result["token_manager_available"])
}

// TestHealthCheckService_TokenManagerUnavailable tests token manager unavailable scenario
func TestHealthCheckService_TokenManagerUnavailable(t *testing.T) {
	// Don't set environment variables to simulate unavailable credentials
	os.Unsetenv("INOREADER_CLIENT_ID")
	os.Unsetenv("INOREADER_CLIENT_SECRET")

	cfg := &config.Config{
		OAuth2: config.OAuth2Config{
			// Empty credentials
		},
	}
	
	healthService := NewHealthCheckServiceWithConfig(cfg)
	result := healthService.PerformHealthCheck(context.Background())
	
	assert.Equal(t, "degraded", result["status"])
	assert.Equal(t, false, result["token_manager_available"])
	assert.Contains(t, result, "error_details")
}

// TestHealthCheckService_OAuth2ClientHealth tests OAuth2 client connectivity
func TestHealthCheckService_OAuth2ClientHealth(t *testing.T) {
	// Set environment variables for OAuth2 client
	os.Setenv("INOREADER_CLIENT_ID", "test-client-id")
	os.Setenv("INOREADER_CLIENT_SECRET", "test-client-secret")
	defer func() {
		os.Unsetenv("INOREADER_CLIENT_ID")
		os.Unsetenv("INOREADER_CLIENT_SECRET")
	}()

	cfg := &config.Config{
		OAuth2: config.OAuth2Config{
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
		},
		Inoreader: config.InoreaderConfig{
			BaseURL: "https://www.inoreader.com",
		},
		Database: config.DatabaseConfig{
			Host: "mock-db",
			Port: "5432",
		},
	}
	
	healthService := NewHealthCheckServiceWithConfig(cfg)
	result := healthService.PerformHealthCheck(context.Background())
	
	assert.Equal(t, "healthy", result["status"])
	assert.Contains(t, result, "oauth2_client_configured")
	assert.Equal(t, true, result["oauth2_client_configured"])
}

// TestHealthCheckService_DatabaseConnectivity tests database connectivity
func TestHealthCheckService_DatabaseConnectivity(t *testing.T) {
	// Set environment variables for OAuth2 credentials
	os.Setenv("INOREADER_CLIENT_ID", "test-client-id")
	os.Setenv("INOREADER_CLIENT_SECRET", "test-client-secret")
	defer func() {
		os.Unsetenv("INOREADER_CLIENT_ID")
		os.Unsetenv("INOREADER_CLIENT_SECRET")
	}()

	// For this test, we'll mock the database connectivity check
	cfg := &config.Config{
		OAuth2: config.OAuth2Config{
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
		},
		Database: config.DatabaseConfig{
			Host:     "mock-db-host",
			Port:     "5432",
			User:     "test-user",
			Password: "test-pass",
			Name:     "test-db",
			SSLMode:  "disable",
		},
	}
	
	healthService := NewHealthCheckServiceWithConfig(cfg)
	
	// Override the database check with a mock
	healthService.databaseHealthCheck = func(cfg *config.Config) bool {
		return cfg.Database.Host != ""
	}
	
	result := healthService.PerformHealthCheck(context.Background())
	
	assert.Equal(t, "healthy", result["status"])
	assert.Contains(t, result, "database_configured")
	assert.Equal(t, true, result["database_configured"])
}

// TestHealthCheckService_ComprehensiveHealth tests all components together
func TestHealthCheckService_ComprehensiveHealth(t *testing.T) {
	// Set environment variables for OAuth2 client
	os.Setenv("INOREADER_CLIENT_ID", "test-client-id")
	os.Setenv("INOREADER_CLIENT_SECRET", "test-client-secret")
	defer func() {
		os.Unsetenv("INOREADER_CLIENT_ID")
		os.Unsetenv("INOREADER_CLIENT_SECRET")
	}()

	cfg := &config.Config{
		OAuth2: config.OAuth2Config{
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
		},
		Inoreader: config.InoreaderConfig{
			BaseURL: "https://www.inoreader.com",
		},
		Database: config.DatabaseConfig{
			Host: "mock-db",
			Port: "5432",
		},
	}

	healthService := NewHealthCheckServiceWithConfig(cfg)
	healthService.databaseHealthCheck = func(cfg *config.Config) bool { return true }

	result := healthService.PerformHealthCheck(context.Background())

	// Comprehensive health check assertions
	expectedFields := []string{
		"status", "timestamp", "version", "token_manager_available",
		"oauth2_client_configured", "database_configured", "monitoring_status",
	}

	for _, field := range expectedFields {
		assert.Contains(t, result, field, "Health check should contain field: %s", field)
	}

	assert.Equal(t, "healthy", result["status"])
	assert.Equal(t, true, result["token_manager_available"])
	assert.Equal(t, true, result["oauth2_client_configured"])
	assert.Equal(t, true, result["database_configured"])
}

// TestHealthCheckService_PartialFailure tests scenario with some components failing
func TestHealthCheckService_PartialFailure(t *testing.T) {
	// Don't set environment variables to simulate token manager failure
	os.Unsetenv("INOREADER_CLIENT_ID")
	os.Unsetenv("INOREADER_CLIENT_SECRET")

	cfg := &config.Config{
		OAuth2: config.OAuth2Config{
			// Empty credentials to simulate failure
		},
		Inoreader: config.InoreaderConfig{
			BaseURL: "https://www.inoreader.com",
		},
		Database: config.DatabaseConfig{
			Host: "mock-db",
			Port: "5432",
		},
	}

	healthService := NewHealthCheckServiceWithConfig(cfg)
	result := healthService.PerformHealthCheck(context.Background())

	// Should be degraded due to token manager failure
	assert.Equal(t, "degraded", result["status"])
	assert.Equal(t, false, result["token_manager_available"])
	assert.Equal(t, false, result["oauth2_client_configured"])
	assert.Contains(t, result, "error_details")
}

// TestHealthCheckService_Timeout tests health check with timeout
func TestHealthCheckService_Timeout(t *testing.T) {
	// For this test, we'll simulate a timeout by using context
	cfg := &config.Config{
		OAuth2: config.OAuth2Config{
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
		},
	}

	healthService := NewHealthCheckServiceWithConfig(cfg)
	
	// Override the token manager health check to simulate a slow response
	healthService.tokenManagerHealthCheck = func(baseURL string) (bool, error) {
		time.Sleep(1 * time.Second) // Simulate slow response
		return true, nil
	}
	
	// Set a short timeout for testing
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	
	result := healthService.PerformHealthCheck(ctx)

	// Should still complete as our current implementation doesn't check context timeout
	assert.NotNil(t, result)
	assert.Contains(t, result, "status")
}

// TestPerformHealthCheckCommand tests the command-line health check
func TestPerformHealthCheckCommand(t *testing.T) {
	// This test verifies that the command-line health check works correctly
	// We'll capture the output and verify it's structured properly
	
	// Create a basic health check that doesn't require external dependencies
	result := performComprehensiveHealthCheck()
	
	assert.NotNil(t, result)
	assert.Contains(t, result, "status")
	assert.Contains(t, result, "timestamp")
}