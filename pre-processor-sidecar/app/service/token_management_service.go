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

// performTokenRefresh performs the actual token refresh operation
func (s *TokenManagementService) performTokenRefresh(ctx context.Context, token *models.OAuth2Token) (*models.OAuth2Token, error) {
	// Call OAuth2 client to refresh token
	refreshResponse, err := s.oauth2Client.RefreshToken(ctx, token.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("OAuth2 refresh API call failed: %w", err)
	}

	// Create new token from response
	refreshedToken := models.NewOAuth2Token(*refreshResponse, token.RefreshToken)

	// Update token in storage
	err = s.tokenRepo.UpdateToken(ctx, refreshedToken)
	if err != nil {
		s.logger.Error("Failed to save refreshed token to storage", "error", err)
		// Token refresh succeeded but storage failed - log but continue
		// The refreshed token is still valid for this execution
	} else {
		s.logger.Info("Refreshed token saved to storage successfully")
	}

	return refreshedToken, nil
}

// GetTokenStatus returns current token status for monitoring
func (s *TokenManagementService) GetTokenStatus(ctx context.Context) (*TokenStatus, error) {
	token, err := s.loadTokenFromStorage(ctx)
	if err != nil {
		return &TokenStatus{
			Exists:        false,
			IsValid:       false,
			IsExpired:     true,
			NeedsRefresh:  true,
			ErrorMessage:  err.Error(),
		}, nil
	}

	return &TokenStatus{
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

// TokenStatus represents the current status of OAuth2 token
type TokenStatus struct {
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