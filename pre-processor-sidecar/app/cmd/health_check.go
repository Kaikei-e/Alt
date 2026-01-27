package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"pre-processor-sidecar/config"
)

// HealthCheckService provides health check functionality for the pre-processor-sidecar
type HealthCheckService struct {
	config                  *config.Config
	logger                  *slog.Logger
	databaseHealthCheck     func(*config.Config) bool
	tokenManagerHealthCheck func(string) (bool, error)
	oauth2HealthCheck       func(*config.Config) (bool, error)
}

// NewHealthCheckService creates a new health check service with defaults
func NewHealthCheckService() *HealthCheckService {
	return &HealthCheckService{
		logger:                  slog.Default(),
		databaseHealthCheck:     defaultDatabaseHealthCheck,
		tokenManagerHealthCheck: defaultTokenManagerHealthCheck,
		oauth2HealthCheck:       defaultOAuth2HealthCheck,
	}
}

// NewHealthCheckServiceWithConfig creates a new health check service with configuration
func NewHealthCheckServiceWithConfig(cfg *config.Config) *HealthCheckService {
	return &HealthCheckService{
		config:                  cfg,
		logger:                  slog.Default(),
		databaseHealthCheck:     defaultDatabaseHealthCheck,
		tokenManagerHealthCheck: defaultTokenManagerHealthCheck,
		oauth2HealthCheck:       defaultOAuth2HealthCheck,
	}
}

// PerformHealthCheck performs a comprehensive health check
func (hcs *HealthCheckService) PerformHealthCheck(ctx context.Context) map[string]interface{} {
	result := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version":   getServiceVersion(),
	}

	errors := []string{}

	// Check token manager if config is available
	if hcs.config != nil {
		// Check OAuth2 client configuration
		oauth2Configured, err := hcs.oauth2HealthCheck(hcs.config)
		result["oauth2_client_configured"] = oauth2Configured
		if err != nil {
			errors = append(errors, fmt.Sprintf("oauth2_check: %v", err))
		}

		// Check database configuration
		dbConfigured := hcs.databaseHealthCheck(hcs.config)
		result["database_configured"] = dbConfigured
		if !dbConfigured {
			errors = append(errors, "database not configured")
		}

		// Check token manager availability (simulated for now)
		tokenManagerAvailable, err := hcs.tokenManagerHealthCheck("")
		result["token_manager_available"] = tokenManagerAvailable
		if err != nil {
			errors = append(errors, fmt.Sprintf("token_manager: %v", err))
		}
	}

	// Check monitoring status (basic implementation)
	result["monitoring_status"] = "active"

	// Set overall status based on errors
	if len(errors) > 0 {
		result["status"] = "degraded"
		result["error_details"] = errors
	}

	return result
}

// Default health check implementations

func defaultDatabaseHealthCheck(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	return cfg.Database.Host != "" && cfg.Database.Port != ""
}

func defaultTokenManagerHealthCheck(baseURL string) (bool, error) {
	// Check for OAuth2 credentials from file or environment variable
	// Uses GetSecretOrEnv to support both file-based secrets (Docker/K8s) and env vars
	clientID := config.GetSecretOrEnv("INOREADER_CLIENT_ID_FILE", "INOREADER_CLIENT_ID")
	clientSecret := config.GetSecretOrEnv("INOREADER_CLIENT_SECRET_FILE", "INOREADER_CLIENT_SECRET")

	if clientID == "" || clientSecret == "" {
		return false, fmt.Errorf("OAuth2 credentials not configured")
	}

	return true, nil
}

func defaultOAuth2HealthCheck(cfg *config.Config) (bool, error) {
	if cfg == nil {
		return false, fmt.Errorf("no configuration provided")
	}

	// Check if OAuth2 configuration exists
	if cfg.OAuth2.ClientID == "" || cfg.OAuth2.ClientSecret == "" {
		// Try file-based secrets or environment variables as fallback
		clientID := config.GetSecretOrEnv("INOREADER_CLIENT_ID_FILE", "INOREADER_CLIENT_ID")
		clientSecret := config.GetSecretOrEnv("INOREADER_CLIENT_SECRET_FILE", "INOREADER_CLIENT_SECRET")

		if clientID == "" || clientSecret == "" {
			return false, fmt.Errorf("OAuth2 credentials not configured")
		}
	}

	return true, nil
}

func getServiceVersion() string {
	version := os.Getenv("SERVICE_VERSION")
	if version == "" {
		version = "unknown"
	}
	return version
}

// performComprehensiveHealthCheck performs a comprehensive health check for command line use
func performComprehensiveHealthCheck() map[string]interface{} {
	// Load configuration if available
	cfg, err := config.LoadConfig()
	var healthService *HealthCheckService

	if err != nil {
		// Create without config if loading fails
		healthService = NewHealthCheckService()
	} else {
		healthService = NewHealthCheckServiceWithConfig(cfg)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return healthService.PerformHealthCheck(ctx)
}

// performHealthCheckWithOutput performs health check and outputs JSON
func performHealthCheckWithOutput() {
	result := performComprehensiveHealthCheck()

	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Printf(`{"status": "error", "error": "failed to marshal health check result: %v"}`, err)
		os.Exit(1)
	}

	fmt.Println(string(output))

	// Exit with error code if not healthy
	if status, ok := result["status"]; ok && status != "healthy" {
		os.Exit(1)
	}
}
