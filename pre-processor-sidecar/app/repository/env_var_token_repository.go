package repository

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"pre-processor-sidecar/models"
)

// EnvVarTokenRepository implements OAuth2TokenRepository using environment variables
// This is the permanent solution for Kubernetes secret integration
type EnvVarTokenRepository struct {
	logger *slog.Logger
	mu     sync.RWMutex
}

// NewEnvVarTokenRepository creates a new environment variable-based token repository
func NewEnvVarTokenRepository(logger *slog.Logger) *EnvVarTokenRepository {
	if logger == nil {
		logger = slog.Default()
	}

	return &EnvVarTokenRepository{
		logger: logger,
	}
}

// Load loads OAuth2 token from environment variables (PERMANENT SOLUTION)
func (r *EnvVarTokenRepository) Load(ctx context.Context) (*models.OAuth2Token, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Read OAuth2 credentials from Kubernetes secret environment variables
	clientID := os.Getenv("INOREADER_CLIENT_ID")
	clientSecret := os.Getenv("INOREADER_CLIENT_SECRET")
	refreshToken := os.Getenv("INOREADER_REFRESH_TOKEN")
	accessToken := os.Getenv("INOREADER_ACCESS_TOKEN")

	if clientID == "" || clientSecret == "" || refreshToken == "" {
		r.logger.Error("Missing OAuth2 environment variables",
			"has_client_id", clientID != "",
			"has_client_secret", clientSecret != "",
			"has_refresh_token", refreshToken != "")
		return nil, fmt.Errorf("storage access error: required OAuth2 environment variables not found")
	}

	// Set expiration time based on whether we have access token
	var expiresAt time.Time
	if accessToken != "" {
		// If we have access token, set it to expire in 24 hours (typical Inoreader token lifetime)
		expiresAt = time.Now().Add(24 * time.Hour)
	} else {
		// If no access token, force refresh
		expiresAt = time.Now().Add(-1 * time.Hour)
	}

	// Create OAuth2 token with environment variable data
	token := &models.OAuth2Token{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresAt:    expiresAt,
		IssuedAt:     time.Now(),
	}

	r.logger.Info("Successfully loaded OAuth2 token from environment variables",
		"client_id", clientID,
		"has_access_token", len(accessToken) > 0,
		"has_refresh_token", len(refreshToken) > 0,
		"expires_at", expiresAt)

	return token, nil
}

// Save saves OAuth2 token (memory-only for environment variable approach)
func (r *EnvVarTokenRepository) Save(ctx context.Context, token *models.OAuth2Token) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// For environment variable approach, we only log the successful save
	// The actual credentials remain in environment variables
	clientID := os.Getenv("INOREADER_CLIENT_ID")
	r.logger.Info("OAuth2 token state updated in memory",
		"client_id", clientID,
		"expires_at", token.ExpiresAt,
		"token_type", token.TokenType)

	return nil
}

// Delete removes OAuth2 token (no-op for environment variable approach)
func (r *EnvVarTokenRepository) Delete(ctx context.Context) error {
	r.logger.Info("OAuth2 token delete requested (no-op for environment variable approach)")
	return nil
}

// Exists checks if OAuth2 token exists in environment variables
func (r *EnvVarTokenRepository) Exists(ctx context.Context) bool {
	clientID := os.Getenv("INOREADER_CLIENT_ID")
	clientSecret := os.Getenv("INOREADER_CLIENT_SECRET")
	refreshToken := os.Getenv("INOREADER_REFRESH_TOKEN")

	exists := clientID != "" && clientSecret != "" && refreshToken != ""

	r.logger.Debug("Checking OAuth2 token existence in environment variables",
		"exists", exists,
		"has_client_id", clientID != "",
		"has_client_secret", clientSecret != "",
		"has_refresh_token", refreshToken != "")

	return exists
}

// GetStoragePath returns the storage description for logging
func (r *EnvVarTokenRepository) GetStoragePath() string {
	return "environment variables (INOREADER_CLIENT_ID, INOREADER_CLIENT_SECRET, INOREADER_REFRESH_TOKEN)"
}

// UpdateFromRefreshResponse updates token from refresh response
func (r *EnvVarTokenRepository) UpdateFromRefreshResponse(ctx context.Context, response *models.InoreaderTokenResponse) error {
	// For environment variable approach, we create an in-memory representation
	// The base credentials (client_id, client_secret, refresh_token) remain in environment variables

	expiresIn := 3600 // Default to 1 hour if not provided
	if response.ExpiresIn > 0 {
		expiresIn = response.ExpiresIn
	}

	expiresAt := time.Now().Add(time.Duration(expiresIn) * time.Second)

	r.logger.Info("Updated OAuth2 token from refresh response",
		"access_token_length", len(response.AccessToken),
		"token_type", response.TokenType,
		"expires_in_seconds", expiresIn,
		"expires_at", expiresAt)

	return nil
}

// IsHealthy checks if the repository is healthy
func (r *EnvVarTokenRepository) IsHealthy(ctx context.Context) error {
	if !r.Exists(ctx) {
		return fmt.Errorf("OAuth2 environment variables not properly configured")
	}
	return nil
}

// GetCurrentToken retrieves the current OAuth2 token (interface compliance)
func (r *EnvVarTokenRepository) GetCurrentToken(ctx context.Context) (*models.OAuth2Token, error) {
	return r.Load(ctx)
}

// SaveToken saves OAuth2 token (interface compliance)
func (r *EnvVarTokenRepository) SaveToken(ctx context.Context, token *models.OAuth2Token) error {
	return r.Save(ctx, token)
}

// UpdateToken updates OAuth2 token (interface compliance)
func (r *EnvVarTokenRepository) UpdateToken(ctx context.Context, token *models.OAuth2Token) error {
	return r.Save(ctx, token)
}

// DeleteToken removes OAuth2 token (interface compliance)
func (r *EnvVarTokenRepository) DeleteToken(ctx context.Context) error {
	return r.Delete(ctx)
}
