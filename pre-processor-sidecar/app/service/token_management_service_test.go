// TDD TEST FILE: TokenManagementService のテスト
// RED-GREEN-REFACTOR アプローチでサービス層の動作を定義

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
	"pre-processor-sidecar/repository"
)

// MockTokenRepository is a mock implementation of OAuth2TokenRepository
type MockTokenRepository struct {
	mock.Mock
}

func (m *MockTokenRepository) GetCurrentToken(ctx context.Context) (*models.OAuth2Token, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.OAuth2Token), args.Error(1)
}

func (m *MockTokenRepository) SaveToken(ctx context.Context, token *models.OAuth2Token) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

func (m *MockTokenRepository) UpdateToken(ctx context.Context, token *models.OAuth2Token) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

func (m *MockTokenRepository) DeleteToken(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// MockOAuth2Driver is a mock implementation of OAuth2Driver
type MockOAuth2Driver struct {
	mock.Mock
}

func (m *MockOAuth2Driver) RefreshToken(ctx context.Context, refreshToken string) (*models.InoreaderTokenResponse, error) {
	args := m.Called(ctx, refreshToken)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.InoreaderTokenResponse), args.Error(1)
}

func (m *MockOAuth2Driver) ValidateToken(ctx context.Context, accessToken string) (bool, error) {
	args := m.Called(ctx, accessToken)
	return args.Bool(0), args.Error(1)
}

func (m *MockOAuth2Driver) MakeAuthenticatedRequest(ctx context.Context, accessToken, endpoint string, params map[string]string) (map[string]interface{}, error) {
	args := m.Called(ctx, accessToken, endpoint, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

func (m *MockOAuth2Driver) MakeAuthenticatedRequestWithHeaders(ctx context.Context, accessToken, endpoint string, params map[string]string) (map[string]interface{}, map[string]string, error) {
	args := m.Called(ctx, accessToken, endpoint, params)
	if args.Get(0) == nil {
		return nil, nil, args.Error(2)
	}
	return args.Get(0).(map[string]interface{}), args.Get(1).(map[string]string), args.Error(2)
}

// TestTokenManagementService_EnsureValidToken tests the core token management logic
func TestTokenManagementService_EnsureValidToken(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	tests := map[string]struct {
		setupMocks    func(*MockTokenRepository, *MockOAuth2Driver)
		expectedError bool
		validateFunc  func(*testing.T, *models.OAuth2Token, error)
	}{
		"valid_token_no_refresh_needed": {
			setupMocks: func(repo *MockTokenRepository, client *MockOAuth2Driver) {
				validToken := &models.OAuth2Token{
					AccessToken:  "valid_access_token",
					RefreshToken: "valid_refresh_token",
					TokenType:    "Bearer",
					ExpiresIn:    3600,
					ExpiresAt:    time.Now().Add(1 * time.Hour), // Valid for 1 hour
					Scope:        "read",
					IssuedAt:     time.Now(),
				}
				repo.On("GetCurrentToken", mock.Anything).Return(validToken, nil)
				// No refresh should be needed
			},
			expectedError: false,
			validateFunc: func(t *testing.T, token *models.OAuth2Token, err error) {
				require.NoError(t, err)
				assert.NotNil(t, token)
				assert.Equal(t, "valid_access_token", token.AccessToken)
			},
		},
		"token_needs_refresh": {
			setupMocks: func(repo *MockTokenRepository, client *MockOAuth2Driver) {
				expiringSoonToken := &models.OAuth2Token{
					AccessToken:  "expiring_access_token",
					RefreshToken: "valid_refresh_token",
					TokenType:    "Bearer",
					ExpiresIn:    300,
					ExpiresAt:    time.Now().Add(2 * time.Minute), // Expires in 2 minutes (< 5 minute buffer)
					Scope:        "read",
					IssuedAt:     time.Now().Add(-1 * time.Hour),
				}

				refreshedTokenResponse := &models.InoreaderTokenResponse{
					AccessToken:  "new_access_token",
					TokenType:    "Bearer",
					ExpiresIn:    3600,
					RefreshToken: "", // Usually not provided in refresh response
					Scope:        "read",
				}

				repo.On("GetCurrentToken", mock.Anything).Return(expiringSoonToken, nil)
				client.On("RefreshToken", mock.Anything, "valid_refresh_token").Return(refreshedTokenResponse, nil)
				repo.On("UpdateToken", mock.Anything, mock.AnythingOfType("*models.OAuth2Token")).Return(nil)
			},
			expectedError: false,
			validateFunc: func(t *testing.T, token *models.OAuth2Token, err error) {
				require.NoError(t, err)
				assert.NotNil(t, token)
				assert.Equal(t, "new_access_token", token.AccessToken)
				assert.Equal(t, "valid_refresh_token", token.RefreshToken) // Should preserve refresh token
			},
		},
		"no_token_in_storage": {
			setupMocks: func(repo *MockTokenRepository, client *MockOAuth2Driver) {
				repo.On("GetCurrentToken", mock.Anything).Return(nil, repository.ErrTokenNotFound)
			},
			expectedError: true,
			validateFunc: func(t *testing.T, token *models.OAuth2Token, err error) {
				require.Error(t, err)
				assert.Nil(t, token)
				assert.Contains(t, err.Error(), "no OAuth2 token found")
			},
		},
		"refresh_token_fails": {
			setupMocks: func(repo *MockTokenRepository, client *MockOAuth2Driver) {
				expiredToken := &models.OAuth2Token{
					AccessToken:  "expired_access_token",
					RefreshToken: "invalid_refresh_token",
					TokenType:    "Bearer",
					ExpiresIn:    3600,
					ExpiresAt:    time.Now().Add(-1 * time.Hour), // Already expired
					Scope:        "read",
					IssuedAt:     time.Now().Add(-2 * time.Hour),
				}

				repo.On("GetCurrentToken", mock.Anything).Return(expiredToken, nil)
				client.On("RefreshToken", mock.Anything, "invalid_refresh_token").Return(nil, assert.AnError)
			},
			expectedError: true,
			validateFunc: func(t *testing.T, token *models.OAuth2Token, err error) {
				require.Error(t, err)
				assert.Nil(t, token)
				assert.Contains(t, err.Error(), "token refresh failed")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Setup mocks
			mockRepo := &MockTokenRepository{}
			mockClient := &MockOAuth2Driver{}
			tc.setupMocks(mockRepo, mockClient)

			// Create service
			service := NewTokenManagementService(mockRepo, mockClient, logger)
			ctx := context.Background()

			// Execute test
			token, err := service.EnsureValidToken(ctx)

			// Validate results
			tc.validateFunc(t, token, err)

			// Verify mock expectations
			mockRepo.AssertExpectations(t)
			mockClient.AssertExpectations(t)
		})
	}
}

// TestTokenManagementService_RefreshTokenProactively tests proactive token refresh
func TestTokenManagementService_RefreshTokenProactively(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	tests := map[string]struct {
		setupMocks    func(*MockTokenRepository, *MockOAuth2Driver)
		expectedError bool
	}{
		"token_needs_proactive_refresh": {
			setupMocks: func(repo *MockTokenRepository, client *MockOAuth2Driver) {
				soonExpiringToken := &models.OAuth2Token{
					AccessToken:  "soon_expiring_token",
					RefreshToken: "valid_refresh_token",
					TokenType:    "Bearer",
					ExpiresIn:    300,
					ExpiresAt:    time.Now().Add(3 * time.Minute), // Within 5-minute buffer
					Scope:        "read",
					IssuedAt:     time.Now(),
				}

				refreshResponse := &models.InoreaderTokenResponse{
					AccessToken: "proactively_refreshed_token",
					TokenType:   "Bearer",
					ExpiresIn:   3600,
					Scope:       "read",
				}

				repo.On("GetCurrentToken", mock.Anything).Return(soonExpiringToken, nil)
				client.On("RefreshToken", mock.Anything, "valid_refresh_token").Return(refreshResponse, nil)
				repo.On("UpdateToken", mock.Anything, mock.AnythingOfType("*models.OAuth2Token")).Return(nil)
			},
			expectedError: false,
		},
		"token_does_not_need_refresh": {
			setupMocks: func(repo *MockTokenRepository, client *MockOAuth2Driver) {
				validToken := &models.OAuth2Token{
					AccessToken:  "valid_token",
					RefreshToken: "valid_refresh_token",
					TokenType:    "Bearer",
					ExpiresIn:    3600,
					ExpiresAt:    time.Now().Add(1 * time.Hour), // Well within buffer
					Scope:        "read",
					IssuedAt:     time.Now(),
				}

				repo.On("GetCurrentToken", mock.Anything).Return(validToken, nil)
				// No refresh should be attempted
			},
			expectedError: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockRepo := &MockTokenRepository{}
			mockClient := &MockOAuth2Driver{}
			tc.setupMocks(mockRepo, mockClient)

			service := NewTokenManagementService(mockRepo, mockClient, logger)
			ctx := context.Background()

			err := service.RefreshTokenProactively(ctx)

			if tc.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			mockRepo.AssertExpectations(t)
			mockClient.AssertExpectations(t)
		})
	}
}

// TestTokenManagementService_ValidateAndRecoverToken tests token validation and recovery
func TestTokenManagementService_ValidateAndRecoverToken(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	tests := map[string]struct {
		setupMocks    func(*MockTokenRepository, *MockOAuth2Driver)
		expectedError bool
	}{
		"valid_token_no_recovery_needed": {
			setupMocks: func(repo *MockTokenRepository, client *MockOAuth2Driver) {
				validToken := &models.OAuth2Token{
					AccessToken:  "valid_token",
					RefreshToken: "valid_refresh_token",
					TokenType:    "Bearer",
					ExpiresIn:    600,
					ExpiresAt:    time.Now().Add(10 * time.Minute), // Close to expiry to trigger API validation
					Scope:        "read",
					IssuedAt:     time.Now(),
				}

				repo.On("GetCurrentToken", mock.Anything).Return(validToken, nil).Maybe()
				client.On("ValidateToken", mock.Anything, "valid_token").Return(true, nil).Once()
			},
			expectedError: false,
		},
		"invalid_token_requires_recovery": {
			setupMocks: func(repo *MockTokenRepository, client *MockOAuth2Driver) {
				invalidToken := &models.OAuth2Token{
					AccessToken:  "invalid_token",
					RefreshToken: "valid_refresh_token",
					TokenType:    "Bearer",
					ExpiresIn:    600,
					ExpiresAt:    time.Now().Add(10 * time.Minute), // Close to expiry to trigger API validation
					Scope:        "read",
					IssuedAt:     time.Now(),
				}

				recoveredTokenResponse := &models.InoreaderTokenResponse{
					AccessToken: "recovered_token",
					TokenType:   "Bearer",
					ExpiresIn:   3600,
					Scope:       "read",
				}

				// Allow multiple GetCurrentToken calls due to single-flight pattern
				repo.On("GetCurrentToken", mock.Anything).Return(invalidToken, nil).Maybe()
				client.On("ValidateToken", mock.Anything, "invalid_token").Return(false, nil).Once()
				// These calls may not happen due to single-flight caching
				client.On("RefreshToken", mock.Anything, "valid_refresh_token").Return(recoveredTokenResponse, nil).Maybe()
				repo.On("UpdateToken", mock.Anything, mock.AnythingOfType("*models.OAuth2Token")).Return(nil).Maybe()
			},
			expectedError: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockRepo := &MockTokenRepository{}
			mockClient := &MockOAuth2Driver{}
			tc.setupMocks(mockRepo, mockClient)

			service := NewTokenManagementService(mockRepo, mockClient, logger)
			ctx := context.Background()

			err := service.ValidateAndRecoverToken(ctx)

			if tc.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			mockRepo.AssertExpectations(t)
			mockClient.AssertExpectations(t)
		})
	}
}
