// ABOUTME: This file implements comprehensive OAuth2 token lifecycle management
// ABOUTME: Handles token validation, refresh, error recovery with 5-minute buffer strategy

package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"pre-processor-sidecar/driver"
	"pre-processor-sidecar/models"
	"pre-processor-sidecar/repository"

	"golang.org/x/sync/singleflight"
)

// TokenManagementService handles complete OAuth2 token lifecycle management
type TokenManagementService struct {
	tokenRepo        repository.OAuth2TokenRepository
	oauth2Client     OAuth2Driver
	logger           *slog.Logger
	refreshBuffer    time.Duration
	maxRetryAttempts int

	// API call optimization
	validationThreshold time.Duration // Only validate via API when token expires within this time

	// Single-flight group prevents concurrent refresh operations
	refreshGroup *singleflight.Group

	// Monitoring and metrics. metricsMu guards every field of metrics: the
	// singleflight leader's closure and every waiter that shares its result
	// (plus a subsequent, non-overlapping refresh call) can update/read
	// these counters concurrently.
	metricsMu sync.Mutex
	metrics   *TokenManagementMetrics
}

// TokenManagementMetrics tracks token management operations
type TokenManagementMetrics struct {
	TotalRefreshAttempts     int64         `json:"total_refresh_attempts"`
	SuccessfulRefreshes      int64         `json:"successful_refreshes"`
	FailedRefreshes          int64         `json:"failed_refresh_count"`
	NonRetryableFailures     int64         `json:"non_retryable_failures"`
	RateLimitFailures        int64         `json:"rate_limit_failures"`
	LastRefreshTime          time.Time     `json:"last_refresh_time"`
	LastRefreshDuration      time.Duration `json:"last_refresh_duration"`
	AverageRefreshDuration   time.Duration `json:"average_refresh_duration"`
	SingleFlightHits         int64         `json:"singleflight_hits"`
	ConcurrentRefreshBlocked int64         `json:"concurrent_refresh_blocked"`
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
		tokenRepo:           tokenRepo,
		oauth2Client:        oauth2Client,
		logger:              logger,
		refreshBuffer:       30 * time.Minute, // Increased to 30-minute buffer for API optimization
		validationThreshold: 2 * time.Hour,    // Only validate via API when < 2 hours to expiry (API optimized)
		maxRetryAttempts:    3,
		refreshGroup:        &singleflight.Group{}, // Initialize single-flight group
		metrics:             &TokenManagementMetrics{},
	}
}

// NewTokenManagementServiceWithBuffer creates a token management service with custom refresh buffer
func NewTokenManagementServiceWithBuffer(
	tokenRepo repository.OAuth2TokenRepository,
	oauth2Client OAuth2Driver,
	logger *slog.Logger,
	refreshBuffer time.Duration,
) *TokenManagementService {
	if logger == nil {
		logger = slog.Default()
	}

	return &TokenManagementService{
		tokenRepo:           tokenRepo,
		oauth2Client:        oauth2Client,
		logger:              logger,
		refreshBuffer:       refreshBuffer,
		validationThreshold: 2 * time.Hour, // Only validate via API when < 2 hours to expiry (API optimized)
		maxRetryAttempts:    3,
		refreshGroup:        &singleflight.Group{}, // Initialize single-flight group
		metrics:             &TokenManagementMetrics{},
	}
}

// NewTokenManagementServiceWithValidationThreshold creates a token management service with custom validation threshold
func NewTokenManagementServiceWithValidationThreshold(
	tokenRepo repository.OAuth2TokenRepository,
	oauth2Client OAuth2Driver,
	logger *slog.Logger,
	validationThreshold time.Duration,
) *TokenManagementService {
	if logger == nil {
		logger = slog.Default()
	}

	return &TokenManagementService{
		tokenRepo:           tokenRepo,
		oauth2Client:        oauth2Client,
		logger:              logger,
		refreshBuffer:       30 * time.Minute, // Consistent 30-minute buffer
		validationThreshold: validationThreshold,
		maxRetryAttempts:    3,
		refreshGroup:        &singleflight.Group{},
		metrics:             &TokenManagementMetrics{},
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

	// Optimization: Check local expiry first to avoid unnecessary API calls
	timeUntilExpiry := token.TimeUntilExpiry()

	// If token is expired, refresh immediately without API validation
	if token.IsExpired() {
		s.logger.Warn("Token is expired, attempting recovery",
			"expires_at", token.ExpiresAt,
			"expired_since", -timeUntilExpiry)

		_, err := s.refreshTokenWithRetry(ctx, token)
		if err != nil {
			return fmt.Errorf("token recovery failed: %w", err)
		}

		s.logger.Info("Token recovery completed successfully")
		return nil
	}

	// If token has plenty of time left, skip API validation to save API calls
	if timeUntilExpiry > s.validationThreshold {
		s.logger.Debug("Token has sufficient time remaining, skipping API validation",
			"time_until_expiry", timeUntilExpiry,
			"validation_threshold", s.validationThreshold)
		return nil
	}

	// Only validate via API when close to expiry
	s.logger.Info("Token close to expiry, performing API validation",
		"time_until_expiry", timeUntilExpiry,
		"validation_threshold", s.validationThreshold)

	isValid, err := s.oauth2Client.ValidateToken(ctx, token.AccessToken)
	if err != nil {
		s.logger.Warn("Token validation API call failed", "error", err)
	}

	if !isValid {
		s.logger.Warn("Token is invalid, attempting recovery",
			"api_valid", isValid,
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
	if token == nil {
		s.logger.Warn("Token is nil, refresh required")
		return true
	}

	needsRefresh := token.NeedsRefresh(s.refreshBuffer)

	// Enhanced logging based on token status
	if token.IsExpired() {
		s.logger.Warn("Token is already expired - CRITICAL",
			"expired_at", token.ExpiresAt.Format(time.RFC3339),
			"current_time", time.Now().Format(time.RFC3339),
			"expired_for", time.Since(token.ExpiresAt).String(),
			"impact", "api_calls_will_fail")
	} else if needsRefresh {
		timeUntilExpiry := token.TimeUntilExpiry()
		s.logger.Info("Token expires within refresh buffer - proactive refresh",
			"expires_at", token.ExpiresAt.Format(time.RFC3339),
			"time_until_expiry", timeUntilExpiry.String(),
			"refresh_buffer", s.refreshBuffer.String(),
			"refresh_reason", "proactive_maintenance")
	} else {
		timeUntilExpiry := token.TimeUntilExpiry()
		percentageRemaining := float64(timeUntilExpiry) / float64(s.refreshBuffer+timeUntilExpiry) * 100
		s.logger.Debug("Token is valid and does not need refresh",
			"expires_at", token.ExpiresAt.Format(time.RFC3339),
			"time_until_expiry", timeUntilExpiry.String(),
			"percentage_remaining", fmt.Sprintf("%.1f%%", percentageRemaining),
			"status", "healthy")
	}

	return needsRefresh
}

// refreshTokenWithRetry attempts token refresh with retry logic using single-flight pattern
func (s *TokenManagementService) refreshTokenWithRetry(ctx context.Context, token *models.OAuth2Token) (*models.OAuth2Token, error) {
	// Use single-flight pattern to prevent concurrent refresh operations
	refreshKey := "token_refresh"

	startTime := time.Now()
	s.metricsMu.Lock()
	s.metrics.TotalRefreshAttempts++
	s.metricsMu.Unlock()

	result, err, shared := s.refreshGroup.Do(refreshKey, func() (interface{}, error) {
		s.logger.Info("Executing token refresh (single-flight protected)")

		// Re-check token validity in case another goroutine already refreshed it
		currentToken, err := s.loadTokenFromStorage(ctx)
		if err == nil && !s.tokenNeedsRefresh(currentToken) {
			s.logger.Info("Token was already refreshed by another operation")
			return currentToken, nil
		}

		// Perform actual refresh with retry logic
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

				// Check if this is a non-retryable error
				if errors.Is(err, driver.ErrInvalidRefreshToken) ||
					errors.Is(err, driver.ErrTokenRevoked) ||
					errors.Is(err, driver.ErrInvalidGrant) {
					s.logger.Error("Token refresh failed with non-retryable error", "error", err)
					s.metricsMu.Lock()
					s.metrics.NonRetryableFailures++
					s.metrics.FailedRefreshes++
					s.metricsMu.Unlock()
					return nil, fmt.Errorf("non-retryable token refresh error: %w", err)
				}

				// For rate limiting, wait longer
				if errors.Is(err, driver.ErrRateLimited) {
					s.metricsMu.Lock()
					s.metrics.RateLimitFailures++
					s.metricsMu.Unlock()
					if attempt < s.maxRetryAttempts {
						backoffDuration := time.Duration(attempt) * 30 * time.Second // Longer backoff for rate limiting
						s.logger.Warn("Rate limited, waiting longer before retry", "backoff", backoffDuration)
						select {
						case <-ctx.Done():
							return nil, ctx.Err()
						case <-time.After(backoffDuration):
						}
						continue
					}
				}

				if attempt < s.maxRetryAttempts {
					// Exponential backoff for temporary failures
					backoffDuration := time.Duration(attempt) * 2 * time.Second
					if errors.Is(err, driver.ErrTemporaryFailure) {
						backoffDuration = time.Duration(attempt) * 10 * time.Second // Longer backoff for server failures
					}
					s.logger.Info("Retrying token refresh after backoff", "backoff", backoffDuration)
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-time.After(backoffDuration):
					}
					continue
				}
			} else {
				s.logger.Info("Token refresh successful", "attempt", attempt)
				s.metricsMu.Lock()
				s.metrics.SuccessfulRefreshes++
				s.metricsMu.Unlock()
				return refreshedToken, nil
			}
		}

		s.metricsMu.Lock()
		s.metrics.FailedRefreshes++
		s.metricsMu.Unlock()
		return nil, fmt.Errorf("token refresh failed after %d attempts: %w", s.maxRetryAttempts, lastErr)
	})

	// Update metrics
	duration := time.Since(startTime)
	s.metricsMu.Lock()
	s.metrics.LastRefreshTime = startTime
	s.metrics.LastRefreshDuration = duration

	// Update average duration
	if s.metrics.AverageRefreshDuration == 0 {
		s.metrics.AverageRefreshDuration = duration
	} else {
		s.metrics.AverageRefreshDuration = (s.metrics.AverageRefreshDuration + duration) / 2
	}
	if shared {
		s.metrics.SingleFlightHits++
		s.metrics.ConcurrentRefreshBlocked++
	}
	totalAttempts := s.metrics.TotalRefreshAttempts
	successfulRefreshes := s.metrics.SuccessfulRefreshes
	singleFlightHits := s.metrics.SingleFlightHits
	s.metricsMu.Unlock()

	if err != nil {
		s.logger.Error("Token refresh failed",
			"error", err,
			"duration", duration,
			"total_attempts", totalAttempts,
			"success_rate", float64(successfulRefreshes)/float64(totalAttempts))
		return nil, err
	}

	if shared {
		s.logger.Info("Token refresh result shared from concurrent operation",
			"duration", duration,
			"singleflight_hits", singleFlightHits)
	}

	s.logger.Info("Token refresh completed successfully",
		"duration", duration,
		"shared_result", shared,
		"total_refreshes", successfulRefreshes,
		"success_rate", float64(successfulRefreshes)/float64(totalAttempts))

	return result.(*models.OAuth2Token), nil
}

// performTokenRefresh performs the actual token refresh operation with rotation support
func (s *TokenManagementService) performTokenRefresh(ctx context.Context, token *models.OAuth2Token) (*models.OAuth2Token, error) {
	oldRefreshToken := token.RefreshToken

	// Never log any part of the refresh token — length only.
	s.logger.Info("Performing token refresh",
		"old_refresh_token_length", len(oldRefreshToken),
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
			"old_refresh_token_length", len(oldRefreshToken),
			"new_refresh_token_length", len(refreshResponse.RefreshToken),
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

// GetTokenStatus returns current token status for monitoring
func (s *TokenManagementService) GetTokenStatus(ctx context.Context) (*OldTokenStatus, error) {
	token, err := s.loadTokenFromStorage(ctx)
	if err != nil {
		return &OldTokenStatus{
			Exists:       false,
			IsValid:      false,
			IsExpired:    true,
			NeedsRefresh: true,
			ErrorMessage: err.Error(),
		}, nil
	}

	return &OldTokenStatus{
		Exists:       true,
		IsValid:      token.IsValid(),
		IsExpired:    token.IsExpired(),
		NeedsRefresh: s.tokenNeedsRefresh(token),
		ExpiresAt:    token.ExpiresAt,
		TimeToExpiry: token.TimeUntilExpiry(),
		TokenType:    token.TokenType,
		Scope:        token.Scope,
	}, nil
}

// OldTokenStatus represents the current status of OAuth2 token from old service
type OldTokenStatus struct {
	Exists       bool          `json:"exists"`
	IsValid      bool          `json:"is_valid"`
	IsExpired    bool          `json:"is_expired"`
	NeedsRefresh bool          `json:"needs_refresh"`
	ExpiresAt    time.Time     `json:"expires_at,omitempty"`
	TimeToExpiry time.Duration `json:"time_to_expiry,omitempty"`
	TokenType    string        `json:"token_type,omitempty"`
	Scope        string        `json:"scope,omitempty"`
	ErrorMessage string        `json:"error_message,omitempty"`
}

// GetMetrics returns a snapshot copy of the current metrics for token
// management operations. A copy — not the live pointer — is returned so
// callers can't observe a struct mid-update from a concurrent refresh.
func (s *TokenManagementService) GetMetrics() *TokenManagementMetrics {
	s.metricsMu.Lock()
	defer s.metricsMu.Unlock()
	snapshot := *s.metrics
	return &snapshot
}

// ResetMetrics resets all metrics counters (useful for testing)
func (s *TokenManagementService) ResetMetrics() {
	s.metricsMu.Lock()
	defer s.metricsMu.Unlock()
	s.metrics = &TokenManagementMetrics{}
}

// GetHealthMetrics returns a comprehensive health report including metrics
func (s *TokenManagementService) GetHealthMetrics(ctx context.Context) map[string]interface{} {
	// Get current token status
	tokenStatus, _ := s.GetTokenStatus(ctx)

	metrics := s.GetMetrics()

	// Calculate success rate
	var successRate float64
	if metrics.TotalRefreshAttempts > 0 {
		successRate = float64(metrics.SuccessfulRefreshes) / float64(metrics.TotalRefreshAttempts)
	}

	return map[string]interface{}{
		"token_status": tokenStatus,
		"metrics":      metrics,
		"calculated_metrics": map[string]interface{}{
			"success_rate_percentage":  successRate * 100,
			"failure_rate_percentage":  (1 - successRate) * 100,
			"single_flight_efficiency": float64(metrics.ConcurrentRefreshBlocked) / float64(metrics.TotalRefreshAttempts+metrics.ConcurrentRefreshBlocked) * 100,
			"average_refresh_time_ms":  metrics.AverageRefreshDuration.Milliseconds(),
			"last_refresh_time_ms":     metrics.LastRefreshDuration.Milliseconds(),
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
}
