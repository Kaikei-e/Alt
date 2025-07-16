package auth_gateway

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"alt/domain"
	"alt/mocks"
	"alt/port/auth_port"
)

func TestAuthGateway_ValidateSession(t *testing.T) {
	tests := []struct {
		name         string
		sessionToken string
		tenantID     string
		setupMock    func(*mocks.MockAuthClient)
		wantValid    bool
		wantUserID   string
		wantError    bool
	}{
		{
			name:         "valid session with context",
			sessionToken: "valid-session-token",
			tenantID:     "tenant-123",
			setupMock: func(mockClient *mocks.MockAuthClient) {
				userContext := &domain.UserContext{
					UserID:    uuid.New(),
					Email:     "test@example.com",
					Role:      domain.UserRoleUser,
					TenantID:  uuid.New(),
					SessionID: "valid-session-token",
					ExpiresAt: time.Now().Add(time.Hour),
				}
				mockClient.EXPECT().
					ValidateSession(gomock.Any(), "valid-session-token", "tenant-123").
					Return(&auth_port.SessionValidationResponse{
						Valid:   true,
						UserID:  userContext.UserID.String(),
						Email:   "test@example.com",
						Role:    "user",
						Context: userContext,
					}, nil)
			},
			wantValid:  true,
			wantUserID: "",
			wantError:  false,
		},
		{
			name:         "valid session without context",
			sessionToken: "valid-session-token",
			tenantID:     "tenant-123",
			setupMock: func(mockClient *mocks.MockAuthClient) {
				mockClient.EXPECT().
					ValidateSession(gomock.Any(), "valid-session-token", "tenant-123").
					Return(&auth_port.SessionValidationResponse{
						Valid:   true,
						UserID:  "user-123",
						Email:   "test@example.com",
						Role:    "user",
						Context: nil,
					}, nil)
			},
			wantValid:  true,
			wantUserID: "user-123",
			wantError:  false,
		},
		{
			name:         "invalid session",
			sessionToken: "invalid-session-token",
			tenantID:     "tenant-123",
			setupMock: func(mockClient *mocks.MockAuthClient) {
				mockClient.EXPECT().
					ValidateSession(gomock.Any(), "invalid-session-token", "tenant-123").
					Return(&auth_port.SessionValidationResponse{
						Valid: false,
					}, nil)
			},
			wantValid:  false,
			wantUserID: "",
			wantError:  false,
		},
		{
			name:         "auth service error",
			sessionToken: "test-token",
			tenantID:     "tenant-123",
			setupMock: func(mockClient *mocks.MockAuthClient) {
				mockClient.EXPECT().
					ValidateSession(gomock.Any(), "test-token", "tenant-123").
					Return(nil, errors.New("auth service unavailable"))
			},
			wantValid:  false,
			wantUserID: "",
			wantError:  true,
		},
		{
			name:         "empty session token",
			sessionToken: "",
			tenantID:     "tenant-123",
			setupMock: func(mockClient *mocks.MockAuthClient) {
				// No mock expectations - should fail before calling auth client
			},
			wantValid:  false,
			wantUserID: "",
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mocks.NewMockAuthClient(ctrl)
			tt.setupMock(mockClient)

			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			gateway := NewAuthGateway(mockClient, logger)

			ctx := context.Background()
			result, err := gateway.ValidateSession(ctx, tt.sessionToken, tt.tenantID)

			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.wantValid, result.Valid)
				if tt.wantUserID != "" {
					assert.Equal(t, tt.wantUserID, result.UserID)
				}
				if result.Valid {
					assert.NotNil(t, result.Context)
				}
			}
		})
	}
}

func TestAuthGateway_GenerateCSRFToken(t *testing.T) {
	tests := []struct {
		name         string
		sessionToken string
		setupMock    func(*mocks.MockAuthClient)
		wantToken    string
		wantError    bool
	}{
		{
			name:         "successful token generation",
			sessionToken: "valid-session-token",
			setupMock: func(mockClient *mocks.MockAuthClient) {
				mockClient.EXPECT().
					GenerateCSRFToken(gomock.Any(), "valid-session-token").
					Return(&auth_port.CSRFTokenResponse{
						Token:     "csrf-token-123",
						ExpiresAt: time.Now().Add(time.Hour).Format(time.RFC3339),
					}, nil)
			},
			wantToken: "csrf-token-123",
			wantError: false,
		},
		{
			name:         "auth service error",
			sessionToken: "valid-session-token",
			setupMock: func(mockClient *mocks.MockAuthClient) {
				mockClient.EXPECT().
					GenerateCSRFToken(gomock.Any(), "valid-session-token").
					Return(nil, errors.New("auth service error"))
			},
			wantToken: "",
			wantError: true,
		},
		{
			name:         "empty session token",
			sessionToken: "",
			setupMock: func(mockClient *mocks.MockAuthClient) {
				// No mock expectations - should fail before calling auth client
			},
			wantToken: "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mocks.NewMockAuthClient(ctrl)
			tt.setupMock(mockClient)

			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			gateway := NewAuthGateway(mockClient, logger)

			ctx := context.Background()
			result, err := gateway.GenerateCSRFToken(ctx, tt.sessionToken)

			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.wantToken, result.Token)
			}
		})
	}
}

func TestAuthGateway_ValidateCSRFToken(t *testing.T) {
	tests := []struct {
		name         string
		token        string
		sessionToken string
		setupMock    func(*mocks.MockAuthClient)
		wantValid    bool
		wantError    bool
	}{
		{
			name:         "valid CSRF token",
			token:        "valid-csrf-token",
			sessionToken: "valid-session-token",
			setupMock: func(mockClient *mocks.MockAuthClient) {
				mockClient.EXPECT().
					ValidateCSRFToken(gomock.Any(), "valid-csrf-token", "valid-session-token").
					Return(&auth_port.CSRFValidationResponse{
						Valid: true,
					}, nil)
			},
			wantValid: true,
			wantError: false,
		},
		{
			name:         "invalid CSRF token",
			token:        "invalid-csrf-token",
			sessionToken: "valid-session-token",
			setupMock: func(mockClient *mocks.MockAuthClient) {
				mockClient.EXPECT().
					ValidateCSRFToken(gomock.Any(), "invalid-csrf-token", "valid-session-token").
					Return(&auth_port.CSRFValidationResponse{
						Valid: false,
					}, nil)
			},
			wantValid: false,
			wantError: false,
		},
		{
			name:         "auth service error",
			token:        "test-token",
			sessionToken: "test-session",
			setupMock: func(mockClient *mocks.MockAuthClient) {
				mockClient.EXPECT().
					ValidateCSRFToken(gomock.Any(), "test-token", "test-session").
					Return(nil, errors.New("auth service error"))
			},
			wantValid: false,
			wantError: true,
		},
		{
			name:         "empty CSRF token",
			token:        "",
			sessionToken: "valid-session-token",
			setupMock: func(mockClient *mocks.MockAuthClient) {
				// No mock expectations - should fail before calling auth client
			},
			wantValid: false,
			wantError: true,
		},
		{
			name:         "empty session token",
			token:        "valid-csrf-token",
			sessionToken: "",
			setupMock: func(mockClient *mocks.MockAuthClient) {
				// No mock expectations - should fail before calling auth client
			},
			wantValid: false,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mocks.NewMockAuthClient(ctrl)
			tt.setupMock(mockClient)

			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			gateway := NewAuthGateway(mockClient, logger)

			ctx := context.Background()
			result, err := gateway.ValidateCSRFToken(ctx, tt.token, tt.sessionToken)

			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.wantValid, result.Valid)
			}
		})
	}
}

func TestAuthGateway_HealthCheck(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func(*mocks.MockAuthClient)
		wantError bool
	}{
		{
			name: "healthy service",
			setupMock: func(mockClient *mocks.MockAuthClient) {
				mockClient.EXPECT().
					HealthCheck(gomock.Any()).
					Return(nil)
			},
			wantError: false,
		},
		{
			name: "unhealthy service",
			setupMock: func(mockClient *mocks.MockAuthClient) {
				mockClient.EXPECT().
					HealthCheck(gomock.Any()).
					Return(errors.New("service unavailable"))
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mocks.NewMockAuthClient(ctrl)
			tt.setupMock(mockClient)

			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			gateway := NewAuthGateway(mockClient, logger)

			ctx := context.Background()
			err := gateway.HealthCheck(ctx)

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
