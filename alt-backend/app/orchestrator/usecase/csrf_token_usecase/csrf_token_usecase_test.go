package csrf_token_usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockCSRFTokenGateway is a mock implementation of CSRF token gateway
type MockCSRFTokenGateway struct {
	mock.Mock
}

func (m *MockCSRFTokenGateway) GenerateToken(ctx context.Context) (string, error) {
	args := m.Called(ctx)
	return args.String(0), args.Error(1)
}

func (m *MockCSRFTokenGateway) ValidateToken(ctx context.Context, token string) (bool, error) {
	args := m.Called(ctx, token)
	return args.Bool(0), args.Error(1)
}

func (m *MockCSRFTokenGateway) ValidateHMACToken(ctx context.Context, token string, sessionID string, secret string) (bool, error) {
	args := m.Called(ctx, token, sessionID, secret)
	return args.Bool(0), args.Error(1)
}

func (m *MockCSRFTokenGateway) InvalidateToken(ctx context.Context, token string) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

func TestCSRFTokenUsecase_GenerateToken(t *testing.T) {
	tests := []struct {
		name           string
		setupMock      func(*MockCSRFTokenGateway)
		expectError    bool
		expectedToken  string
		expectedErrMsg string
	}{
		{
			name: "successful token generation",
			setupMock: func(m *MockCSRFTokenGateway) {
				m.On("GenerateToken", mock.Anything).Return("generated-csrf-token", nil)
			},
			expectError:   false,
			expectedToken: "generated-csrf-token",
		},
		{
			name: "token generation failure",
			setupMock: func(m *MockCSRFTokenGateway) {
				m.On("GenerateToken", mock.Anything).Return("", errors.New("token generation failed"))
			},
			expectError:    true,
			expectedToken:  "",
			expectedErrMsg: "failed to generate CSRF token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock gateway
			mockGateway := &MockCSRFTokenGateway{}
			tt.setupMock(mockGateway)

			// Create usecase
			usecase := NewCSRFTokenUsecase(mockGateway)

			// Execute test
			ctx := context.Background()
			token, err := usecase.GenerateToken(ctx)

			// Verify results
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrMsg)
				assert.Empty(t, token)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedToken, token)
			}

			// Verify mock expectations
			mockGateway.AssertExpectations(t)
		})
	}
}

func TestCSRFTokenUsecase_ValidateToken(t *testing.T) {
	tests := []struct {
		name           string
		inputToken     string
		setupMock      func(*MockCSRFTokenGateway)
		expectValid    bool
		expectError    bool
		expectedErrMsg string
	}{
		{
			name:       "valid token validation",
			inputToken: "valid-csrf-token",
			setupMock: func(m *MockCSRFTokenGateway) {
				m.On("ValidateToken", mock.Anything, "valid-csrf-token").Return(true, nil)
			},
			expectValid: true,
			expectError: false,
		},
		{
			name:       "invalid token validation",
			inputToken: "invalid-csrf-token",
			setupMock: func(m *MockCSRFTokenGateway) {
				m.On("ValidateToken", mock.Anything, "invalid-csrf-token").Return(false, nil)
			},
			expectValid: false,
			expectError: false,
		},
		{
			name:       "empty token validation",
			inputToken: "",
			setupMock: func(m *MockCSRFTokenGateway) {
				// Empty token should be rejected without calling gateway
			},
			expectValid: false,
			expectError: false,
		},
		{
			name:       "token validation error",
			inputToken: "some-token",
			setupMock: func(m *MockCSRFTokenGateway) {
				m.On("ValidateToken", mock.Anything, "some-token").Return(false, errors.New("validation error"))
			},
			expectValid:    false,
			expectError:    true,
			expectedErrMsg: "failed to validate CSRF token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock gateway
			mockGateway := &MockCSRFTokenGateway{}
			tt.setupMock(mockGateway)

			// Create usecase
			usecase := NewCSRFTokenUsecase(mockGateway)

			// Execute test
			ctx := context.Background()
			valid, err := usecase.ValidateToken(ctx, tt.inputToken)

			// Verify results
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrMsg)
				assert.False(t, valid)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectValid, valid)
			}

			// Verify mock expectations
			mockGateway.AssertExpectations(t)
		})
	}
}

func TestCSRFTokenUsecase_InvalidateToken(t *testing.T) {
	tests := []struct {
		name           string
		inputToken     string
		setupMock      func(*MockCSRFTokenGateway)
		expectError    bool
		expectedErrMsg string
	}{
		{
			name:       "successful token invalidation",
			inputToken: "valid-csrf-token",
			setupMock: func(m *MockCSRFTokenGateway) {
				m.On("InvalidateToken", mock.Anything, "valid-csrf-token").Return(nil)
			},
			expectError: false,
		},
		{
			name:       "token invalidation error",
			inputToken: "some-token",
			setupMock: func(m *MockCSRFTokenGateway) {
				m.On("InvalidateToken", mock.Anything, "some-token").Return(errors.New("invalidation error"))
			},
			expectError:    true,
			expectedErrMsg: "failed to invalidate CSRF token",
		},
		{
			name:       "empty token invalidation",
			inputToken: "",
			setupMock: func(m *MockCSRFTokenGateway) {
				// Empty token should be rejected without calling gateway
			},
			expectError: false, // Should not error for empty token, just ignore
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock gateway
			mockGateway := &MockCSRFTokenGateway{}
			tt.setupMock(mockGateway)

			// Create usecase
			usecase := NewCSRFTokenUsecase(mockGateway)

			// Execute test
			ctx := context.Background()
			err := usecase.InvalidateToken(ctx, tt.inputToken)

			// Verify results
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrMsg)
			} else {
				assert.NoError(t, err)
			}

			// Verify mock expectations
			mockGateway.AssertExpectations(t)
		})
	}
}

func TestCSRFTokenUsecase_TokenExpiration(t *testing.T) {
	tests := []struct {
		name           string
		inputToken     string
		setupMock      func(*MockCSRFTokenGateway)
		expectValid    bool
		expectError    bool
		expectedErrMsg string
	}{
		{
			name:       "expired token should be invalid",
			inputToken: "expired-csrf-token",
			setupMock: func(m *MockCSRFTokenGateway) {
				m.On("ValidateToken", mock.Anything, "expired-csrf-token").Return(false, nil)
			},
			expectValid: false,
			expectError: false,
		},
		{
			name:       "valid non-expired token",
			inputToken: "valid-csrf-token",
			setupMock: func(m *MockCSRFTokenGateway) {
				m.On("ValidateToken", mock.Anything, "valid-csrf-token").Return(true, nil)
			},
			expectValid: true,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock gateway
			mockGateway := &MockCSRFTokenGateway{}
			tt.setupMock(mockGateway)

			// Create usecase
			usecase := NewCSRFTokenUsecase(mockGateway)

			// Execute test
			ctx := context.Background()
			valid, err := usecase.ValidateToken(ctx, tt.inputToken)

			// Verify results
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrMsg)
				assert.False(t, valid)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectValid, valid)
			}

			// Verify mock expectations
			mockGateway.AssertExpectations(t)
		})
	}
}
