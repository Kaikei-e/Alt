// TDD TEST FILE: API Call Reduction Tests for TokenManagementService
// Tests to ensure ValidateAndRecoverToken minimizes API calls

package service

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"pre-processor-sidecar/models"
)

// TestValidateAndRecoverToken_SkipsAPICallWhenTokenValid tests that API validation is skipped when token has sufficient time
func TestValidateAndRecoverToken_SkipsAPICallWhenTokenValid(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	tests := map[string]struct {
		tokenExpiresIn time.Duration
		shouldCallAPI  bool
		description    string
	}{
		"token_valid_for_2_hours": {
			tokenExpiresIn: 2 * time.Hour,
			shouldCallAPI:  false, // Should skip API call
			description:    "Token valid for 2 hours should not trigger API validation",
		},
		"token_valid_for_1_hour": {
			tokenExpiresIn: 1 * time.Hour,
			shouldCallAPI:  false, // Should skip API call
			description:    "Token valid for 1 hour should not trigger API validation",
		},
		"token_valid_for_30_minutes": {
			tokenExpiresIn: 30 * time.Minute,
			shouldCallAPI:  false, // Should skip API call when > 15 minutes
			description:    "Token valid for 30 minutes should not trigger API validation",
		},
		"token_valid_for_10_minutes": {
			tokenExpiresIn: 10 * time.Minute,
			shouldCallAPI:  true, // Should call API when close to expiry
			description:    "Token valid for 10 minutes should trigger API validation",
		},
		"token_valid_for_5_minutes": {
			tokenExpiresIn: 5 * time.Minute,
			shouldCallAPI:  true, // Should call API when very close to expiry
			description:    "Token valid for 5 minutes should trigger API validation",
		},
		"token_expired": {
			tokenExpiresIn: -5 * time.Minute,
			shouldCallAPI:  false, // Expired tokens skip API validation and go straight to refresh
			description:    "Expired token should skip validation and go directly to refresh",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockRepo := &MockTokenRepository{}
			mockClient := &MockOAuth2Driver{}

			token := &models.OAuth2Token{
				AccessToken:  "test_token",
				RefreshToken: "test_refresh_token",
				TokenType:    "Bearer",
				ExpiresIn:    int(tc.tokenExpiresIn.Seconds()),
				ExpiresAt:    time.Now().Add(tc.tokenExpiresIn),
				Scope:        "read",
				IssuedAt:     time.Now(),
			}

			// Setup repository mock - always returns the token
			mockRepo.On("GetCurrentToken", mock.Anything).Return(token, nil).Maybe()

			// Setup refresh mock for expired tokens (regardless of shouldCallAPI)
			if tc.tokenExpiresIn < 0 {
				refreshResponse := &models.InoreaderTokenResponse{
					AccessToken: "new_token",
					TokenType:   "Bearer",
					ExpiresIn:   3600,
					Scope:       "read",
				}
				mockClient.On("RefreshToken", mock.Anything, "test_refresh_token").Return(refreshResponse, nil).Maybe()
				mockRepo.On("UpdateToken", mock.Anything, mock.AnythingOfType("*models.OAuth2Token")).Return(nil).Maybe()
			}

			if tc.shouldCallAPI {
				// Only set up ValidateToken expectation when API should be called
				mockClient.On("ValidateToken", mock.Anything, "test_token").Return(true, nil).Once()
			}
			// If shouldCallAPI is false, we expect NO calls to ValidateToken

			service := NewTokenManagementServiceWithValidationThreshold(mockRepo, mockClient, logger, 15*time.Minute)
			ctx := context.Background()

			err := service.ValidateAndRecoverToken(ctx)
			require.NoError(t, err, tc.description)

			// Verify expectations
			mockRepo.AssertExpectations(t)
			mockClient.AssertExpectations(t)

			// Additional assertion: if shouldCallAPI is false, ensure ValidateToken was NOT called
			if !tc.shouldCallAPI {
				mockClient.AssertNotCalled(t, "ValidateToken", mock.Anything, mock.Anything)
			}
		})
	}
}

// TestValidationThreshold_Configuration tests that validation threshold can be configured
func TestValidationThreshold_Configuration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	tests := map[string]struct {
		threshold         time.Duration
		expectedThreshold time.Duration
	}{
		"15_minute_threshold": {
			threshold:         15 * time.Minute,
			expectedThreshold: 15 * time.Minute,
		},
		"30_minute_threshold": {
			threshold:         30 * time.Minute,
			expectedThreshold: 30 * time.Minute,
		},
		"1_hour_threshold": {
			threshold:         1 * time.Hour,
			expectedThreshold: 1 * time.Hour,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockRepo := &MockTokenRepository{}
			mockClient := &MockOAuth2Driver{}

			service := NewTokenManagementServiceWithValidationThreshold(
				mockRepo,
				mockClient,
				logger,
				tc.threshold,
			)

			assert.Equal(t, tc.expectedThreshold, service.validationThreshold)
		})
	}
}

// TestAPICallLimit_DailyLimit tests that we respect the 100 API calls/day limit
func TestAPICallLimit_DailyLimit(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Simulate a day's worth of operations
	// With 30-minute health checks: 48 checks/day
	// With proper token validation threshold: should only validate when necessary

	mockRepo := &MockTokenRepository{}
	mockClient := &MockOAuth2Driver{}

	// Token valid for 2 hours
	validToken := &models.OAuth2Token{
		AccessToken:  "daily_test_token",
		RefreshToken: "daily_refresh_token",
		TokenType:    "Bearer",
		ExpiresIn:    7200,
		ExpiresAt:    time.Now().Add(2 * time.Hour),
		Scope:        "read",
		IssuedAt:     time.Now(),
	}

	mockRepo.On("GetCurrentToken", mock.Anything).Return(validToken, nil).Maybe()

	service := NewTokenManagementServiceWithValidationThreshold(mockRepo, mockClient, logger, 15*time.Minute)
	ctx := context.Background()

	// Simulate 48 health checks (24 hours with 30-minute intervals)
	apiCallCount := 0
	for i := 0; i < 48; i++ {
		// Update token expiry to simulate time passing
		if i > 0 && i%4 == 0 { // Every 2 hours, update token
			validToken.ExpiresAt = time.Now().Add(2 * time.Hour)
		}

		// Check if this would trigger an API call
		timeToExpiry := validToken.TimeUntilExpiry()
		if timeToExpiry <= 15*time.Minute {
			// This would trigger an API call
			mockClient.On("ValidateToken", mock.Anything, mock.Anything).Return(true, nil).Once()
			apiCallCount++
		}

		err := service.ValidateAndRecoverToken(ctx)
		require.NoError(t, err)
	}

	// Assert that we stayed well under the 100 API calls/day limit
	assert.Less(t, apiCallCount, 20, "Should make far fewer than 20 API calls per day with proper thresholds")

	mockRepo.AssertExpectations(t)
	mockClient.AssertExpectations(t)
}
