// ABOUTME: This file implements comprehensive OAuth2 token lifecycle management
// ABOUTME: Handles token validation, refresh, error recovery with 5-minute buffer strategy

package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"pre-processor-sidecar/models"
	"pre-processor-sidecar/repository"
)

// TokenManagementService handles complete OAuth2 token lifecycle management
type TokenManagementService struct {
	tokenRepo        repository.OAuth2TokenRepository
	oauth2Client     OAuth2Driver
	logger           *slog.Logger
	refreshBuffer    time.Duration
	maxRetryAttempts int
}

// NewTokenManagementService creates a new token management service
func NewTokenManagementService(
	tokenRepo repository.OAuth2TokenRepository,
	oauth2Client OAuth2Driver,
	logger *slog.Logger,
) *TokenManagementService {
	if logger == nil {
		logger = slog.Default()
	}

	return &TokenManagementService{
		tokenRepo:        tokenRepo,
		oauth2Client:     oauth2Client,
		logger:           logger,
		refreshBuffer:    5 * time.Minute, // 5-minute buffer before expiry
		maxRetryAttempts: 3,
	}
}

// EnsureValidToken ensures we have a valid OAuth2 token, refreshing if necessary
func (s *TokenManagementService) EnsureValidToken(ctx context.Context) (*models.OAuth2Token, error) {
	s.logger.Info("Ensuring valid OAuth2 token")

	// Step 1: Load current token from storage
	token, err := s.loadTokenFromStorage(ctx)
	if err != nil {
		s.logger.Error("Failed to load token from storage", "error", err)
		return nil, fmt.Errorf("token storage access failed: %w", err)
	}

	// Step 2: Check if token needs refresh
	if s.tokenNeedsRefresh(token) {
		s.logger.Info("Token needs refresh", 
			"expires_at", token.ExpiresAt, 
			"time_until_expiry", token.TimeUntilExpiry(),
			"refresh_buffer", s.refreshBuffer)

		refreshedToken, err := s.refreshTokenWithRetry(ctx, token)
		if err != nil {
			s.logger.Error("Token refresh failed", "error", err)
			return nil, fmt.Errorf("token refresh failed: %w", err)
		}

		token = refreshedToken
	}

	// Step 3: Final validation
	if !token.IsValid() {
		s.logger.Error("Token is invalid after refresh attempts",
			"access_token_empty", token.AccessToken == "",
			"is_expired", token.IsExpired())
		return nil, fmt.Errorf("token is invalid after refresh attempts")
	}

	s.logger.Info("Valid OAuth2 token ensured",
		"expires_at", token.ExpiresAt,
		"time_until_expiry", token.TimeUntilExpiry())

	return token, nil
}

// RefreshTokenProactively proactively refreshes token if it's close to expiry
func (s *TokenManagementService) RefreshTokenProactively(ctx context.Context) error {
	s.logger.Info("Proactively checking token refresh status")

	token, err := s.loadTokenFromStorage(ctx)
	if err != nil {
		return fmt.Errorf("failed to load token for proactive refresh: %w", err)
	}

	if s.tokenNeedsRefresh(token) {
		s.logger.Info("Proactively refreshing token",
			"expires_at", token.ExpiresAt,
			"time_until_expiry", token.TimeUntilExpiry())

		_, err := s.refreshTokenWithRetry(ctx, token)
		if err != nil {
			return fmt.Errorf("proactive token refresh failed: %w", err)
		}

		s.logger.Info("Proactive token refresh completed successfully")
	} else {
		s.logger.Debug("Token does not need proactive refresh",
			"expires_at", token.ExpiresAt,
			"time_until_expiry", token.TimeUntilExpiry())
	}

	return nil
}

// ValidateAndRecoverToken validates current token and attempts recovery if invalid
func (s *TokenManagementService) ValidateAndRecoverToken(ctx context.Context) error {
	s.logger.Info("Validating and recovering OAuth2 token")

	token, err := s.loadTokenFromStorage(ctx)
	if err != nil {
		return fmt.Errorf("token validation failed - storage access error: %w", err)
	}

	// Check if token is valid by making a test API call
	isValid, err := s.oauth2Client.ValidateToken(ctx, token.AccessToken)
	if err != nil {
		s.logger.Warn("Token validation API call failed", "error", err)
	}

	if !isValid || token.IsExpired() {
		s.logger.Warn("Token is invalid, attempting recovery",
			"api_valid", isValid,
			"is_expired", token.IsExpired(),
			"expires_at", token.ExpiresAt)

		// Attempt token refresh for recovery
		_, err := s.refreshTokenWithRetry(ctx, token)
		if err != nil {
			return fmt.Errorf("token recovery failed: %w", err)
		}

		s.logger.Info("Token recovery completed successfully")
	} else {
		s.logger.Info("Token is valid, no recovery needed")
	}

	return nil
}

// loadTokenFromStorage loads the current token from repository with error handling
func (s *TokenManagementService) loadTokenFromStorage(ctx context.Context) (*models.OAuth2Token, error) {
	token, err := s.tokenRepo.GetCurrentToken(ctx)
	if err != nil {
		if err == repository.ErrTokenNotFound {
			return nil, fmt.Errorf("no OAuth2 token found in storage - run oauth-init tool first")
		}
		return nil, fmt.Errorf("storage access error: %w", err)
	}

	return token, nil
}

// tokenNeedsRefresh checks if token needs refresh using buffer time
func (s *TokenManagementService) tokenNeedsRefresh(token *models.OAuth2Token) bool {
	return token.NeedsRefresh(s.refreshBuffer)
}

// refreshTokenWithRetry attempts token refresh with retry logic
func (s *TokenManagementService) refreshTokenWithRetry(ctx context.Context, token *models.OAuth2Token) (*models.OAuth2Token, error) {
	var lastErr error

	for attempt := 1; attempt <= s.maxRetryAttempts; attempt++ {
		s.logger.Info("Attempting token refresh",
			"attempt", attempt,
			"max_attempts", s.maxRetryAttempts)

		refreshedToken, err := s.performTokenRefresh(ctx, token)
		if err != nil {
			lastErr = err
			s.logger.Warn("Token refresh attempt failed",
				"attempt", attempt,
				"error", err)

			if attempt < s.maxRetryAttempts {
				backoffDuration := time.Duration(attempt) * 2 * time.Second
				s.logger.Info("Retrying token refresh after backoff", "backoff", backoffDuration)
				time.Sleep(backoffDuration)
				continue
			}
		} else {
			s.logger.Info("Token refresh successful", "attempt", attempt)
			return refreshedToken, nil
		}
	}

	return nil, fmt.Errorf("token refresh failed after %d attempts: %w", s.maxRetryAttempts, lastErr)
}

// performTokenRefresh performs the actual token refresh operation with rotation support
func (s *TokenManagementService) performTokenRefresh(ctx context.Context, token *models.OAuth2Token) (*models.OAuth2Token, error) {
	oldRefreshToken := token.RefreshToken
	
	s.logger.Info("Performing token refresh",
		"old_refresh_token_prefix", oldRefreshToken[:min(8, len(oldRefreshToken))],
		"expires_at", token.ExpiresAt)

	// Call OAuth2 client to refresh token
	refreshResponse, err := s.oauth2Client.RefreshToken(ctx, token.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("OAuth2 refresh API call failed: %w", err)
	}

	// Create new token from response, preserving existing refresh token as fallback
	refreshedToken := models.NewOAuth2Token(*refreshResponse, token.RefreshToken)

	// Check if refresh token was rotated (new refresh token provided in response)
	refreshTokenRotated := false
	if refreshResponse.RefreshToken != "" && refreshResponse.RefreshToken != token.RefreshToken {
		refreshTokenRotated = true
		refreshedToken.RefreshToken = refreshResponse.RefreshToken
		
		s.logger.Warn("Refresh token rotation detected",
			"old_refresh_token_prefix", oldRefreshToken[:min(8, len(oldRefreshToken))],
			"new_refresh_token_prefix", refreshResponse.RefreshToken[:min(8, len(refreshResponse.RefreshToken))],
			"rotation_required", true)
	}

	// Update token in storage with enhanced error handling for rotation
	err = s.updateTokenWithRotation(ctx, refreshedToken, oldRefreshToken, refreshTokenRotated)
	if err != nil {
		s.logger.Error("Failed to save refreshed token to storage", 
			"error", err,
			"refresh_token_rotated", refreshTokenRotated)
		
		// For token rotation, storage failure is critical
		if refreshTokenRotated {
			return nil, fmt.Errorf("critical: refresh token rotated but storage failed: %w", err)
		}
		
		// For normal refresh, warn but continue with in-memory token
		s.logger.Warn("Token refresh succeeded but storage failed - using in-memory token", "error", err)
	} else {
		s.logger.Info("Refreshed token saved to storage successfully",
			"refresh_token_rotated", refreshTokenRotated)
	}

	return refreshedToken, nil
}

// updateTokenWithRotation updates token with rotation awareness
func (s *TokenManagementService) updateTokenWithRotation(
	ctx context.Context, 
	token *models.OAuth2Token, 
	oldRefreshToken string, 
	rotated bool,
) error {
	// Check if repository supports rotation-aware updates
	if rotationRepo, ok := s.tokenRepo.(interface {
		UpdateWithRefreshRotation(ctx context.Context, token *models.OAuth2Token, oldRefreshToken string) error
	}); ok {
		// Use rotation-aware update if available
		return rotationRepo.UpdateWithRefreshRotation(ctx, token, oldRefreshToken)
	}
	
	// Fallback to standard update
	return s.tokenRepo.UpdateToken(ctx, token)
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// GetTokenStatus returns current token status for monitoring
func (s *TokenManagementService) GetTokenStatus(ctx context.Context) (*OldTokenStatus, error) {
	token, err := s.loadTokenFromStorage(ctx)
	if err != nil {
		return &OldTokenStatus{
			Exists:        false,
			IsValid:       false,
			IsExpired:     true,
			NeedsRefresh:  true,
			ErrorMessage:  err.Error(),
		}, nil
	}

	return &OldTokenStatus{
		Exists:        true,
		IsValid:       token.IsValid(),
		IsExpired:     token.IsExpired(),
		NeedsRefresh:  s.tokenNeedsRefresh(token),
		ExpiresAt:     token.ExpiresAt,
		TimeToExpiry:  token.TimeUntilExpiry(),
		TokenType:     token.TokenType,
		Scope:         token.Scope,
	}, nil
}

// OldTokenStatus represents the current status of OAuth2 token from old service
type OldTokenStatus struct {
	Exists        bool          `json:"exists"`
	IsValid       bool          `json:"is_valid"`
	IsExpired     bool          `json:"is_expired"`
	NeedsRefresh  bool          `json:"needs_refresh"`
	ExpiresAt     time.Time     `json:"expires_at,omitempty"`
	TimeToExpiry  time.Duration `json:"time_to_expiry,omitempty"`
	TokenType     string        `json:"token_type,omitempty"`
	Scope         string        `json:"scope,omitempty"`
	ErrorMessage  string        `json:"error_message,omitempty"`
}