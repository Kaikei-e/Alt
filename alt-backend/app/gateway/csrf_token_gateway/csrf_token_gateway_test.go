package csrf_token_gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockCSRFTokenDriver is a mock implementation of CSRF token driver
type MockCSRFTokenDriver struct {
	mock.Mock
}

func (m *MockCSRFTokenDriver) StoreToken(ctx context.Context, token string, expiration time.Time) error {
	args := m.Called(ctx, token, expiration)
	return args.Error(0)
}

func (m *MockCSRFTokenDriver) GetToken(ctx context.Context, token string) (time.Time, error) {
	args := m.Called(ctx, token)
	return args.Get(0).(time.Time), args.Error(1)
}

func (m *MockCSRFTokenDriver) DeleteToken(ctx context.Context, token string) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

func (m *MockCSRFTokenDriver) GenerateRandomToken() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func TestCSRFTokenGateway_GenerateToken(t *testing.T) {
	tests := []struct {
		name           string
		setupMock      func(*MockCSRFTokenDriver)
		expectError    bool
		expectedToken  string
		expectedErrMsg string
	}{
		{
			name: "successful token generation and storage",
			setupMock: func(m *MockCSRFTokenDriver) {
				m.On("GenerateRandomToken").Return("random-csrf-token", nil)
				m.On("StoreToken", mock.Anything, "random-csrf-token", mock.AnythingOfType("time.Time")).Return(nil)
			},
			expectError:   false,
			expectedToken: "random-csrf-token",
		},
		{
			name: "token generation failure",
			setupMock: func(m *MockCSRFTokenDriver) {
				m.On("GenerateRandomToken").Return("", errors.New("random generation failed"))
			},
			expectError:    true,
			expectedToken:  "",
			expectedErrMsg: "failed to generate random token",
		},
		{
			name: "token storage failure",
			setupMock: func(m *MockCSRFTokenDriver) {
				m.On("GenerateRandomToken").Return("random-csrf-token", nil)
				m.On("StoreToken", mock.Anything, "random-csrf-token", mock.AnythingOfType("time.Time")).Return(errors.New("storage failed"))
			},
			expectError:    true,
			expectedToken:  "",
			expectedErrMsg: "failed to store CSRF token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock driver
			mockDriver := &MockCSRFTokenDriver{}
			tt.setupMock(mockDriver)

			// Create gateway
			gateway := NewCSRFTokenGateway(mockDriver)

			// Execute test
			ctx := context.Background()
			token, err := gateway.GenerateToken(ctx)

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
			mockDriver.AssertExpectations(t)
		})
	}
}

func TestCSRFTokenGateway_ValidateToken(t *testing.T) {
	tests := []struct {
		name           string
		inputToken     string
		setupMock      func(*MockCSRFTokenDriver)
		expectValid    bool
		expectError    bool
		expectedErrMsg string
	}{
		{
			name:       "valid token within expiration",
			inputToken: "valid-csrf-token",
			setupMock: func(m *MockCSRFTokenDriver) {
				futureTime := time.Now().Add(1 * time.Hour)
				m.On("GetToken", mock.Anything, "valid-csrf-token").Return(futureTime, nil)
			},
			expectValid: true,
			expectError: false,
		},
		{
			name:       "expired token",
			inputToken: "expired-csrf-token",
			setupMock: func(m *MockCSRFTokenDriver) {
				pastTime := time.Now().Add(-1 * time.Hour)
				m.On("GetToken", mock.Anything, "expired-csrf-token").Return(pastTime, nil)
				m.On("DeleteToken", mock.Anything, "expired-csrf-token").Return(nil)
			},
			expectValid: false,
			expectError: false,
		},
		{
			name:       "token not found",
			inputToken: "nonexistent-csrf-token",
			setupMock: func(m *MockCSRFTokenDriver) {
				m.On("GetToken", mock.Anything, "nonexistent-csrf-token").Return(time.Time{}, errors.New("token not found"))
			},
			expectValid: false,
			expectError: false,
		},
		{
			name:       "driver error during validation",
			inputToken: "some-token",
			setupMock: func(m *MockCSRFTokenDriver) {
				m.On("GetToken", mock.Anything, "some-token").Return(time.Time{}, errors.New("driver error"))
			},
			expectValid: false,
			expectError: false, // Gateway should handle driver errors gracefully
		},
		{
			name:       "empty token",
			inputToken: "",
			setupMock: func(m *MockCSRFTokenDriver) {
				// Empty token should be rejected without calling driver
			},
			expectValid: false,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock driver
			mockDriver := &MockCSRFTokenDriver{}
			tt.setupMock(mockDriver)

			// Create gateway
			gateway := NewCSRFTokenGateway(mockDriver)

			// Execute test
			ctx := context.Background()
			valid, err := gateway.ValidateToken(ctx, tt.inputToken)

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
			mockDriver.AssertExpectations(t)
		})
	}
}

func TestCSRFTokenGateway_InvalidateToken(t *testing.T) {
	tests := []struct {
		name           string
		inputToken     string
		setupMock      func(*MockCSRFTokenDriver)
		expectError    bool
		expectedErrMsg string
	}{
		{
			name:       "successful token invalidation",
			inputToken: "valid-csrf-token",
			setupMock: func(m *MockCSRFTokenDriver) {
				m.On("DeleteToken", mock.Anything, "valid-csrf-token").Return(nil)
			},
			expectError: false,
		},
		{
			name:       "token deletion failure",
			inputToken: "some-token",
			setupMock: func(m *MockCSRFTokenDriver) {
				m.On("DeleteToken", mock.Anything, "some-token").Return(errors.New("deletion failed"))
			},
			expectError:    true,
			expectedErrMsg: "failed to delete CSRF token",
		},
		{
			name:       "empty token invalidation",
			inputToken: "",
			setupMock: func(m *MockCSRFTokenDriver) {
				// Empty token should be rejected without calling driver
			},
			expectError: false, // Should not error for empty token
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock driver
			mockDriver := &MockCSRFTokenDriver{}
			tt.setupMock(mockDriver)

			// Create gateway
			gateway := NewCSRFTokenGateway(mockDriver)

			// Execute test
			ctx := context.Background()
			err := gateway.InvalidateToken(ctx, tt.inputToken)

			// Verify results
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrMsg)
			} else {
				assert.NoError(t, err)
			}

			// Verify mock expectations
			mockDriver.AssertExpectations(t)
		})
	}
}

func TestCSRFTokenGateway_TokenExpiration(t *testing.T) {
	tests := []struct {
		name          string
		tokenAge      time.Duration
		expectValid   bool
		expectDeleted bool
	}{
		{
			name:          "token within 1 hour should be valid",
			tokenAge:      30 * time.Minute,
			expectValid:   true,
			expectDeleted: false,
		},
		{
			name:          "token exactly 1 hour should be valid",
			tokenAge:      1 * time.Hour,
			expectValid:   true,
			expectDeleted: false,
		},
		{
			name:          "token older than 1 hour should be invalid and deleted",
			tokenAge:      -1 * time.Hour,
			expectValid:   false,
			expectDeleted: true,
		},
		{
			name:          "token much older should be invalid and deleted",
			tokenAge:      -24 * time.Hour,
			expectValid:   false,
			expectDeleted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock driver
			mockDriver := &MockCSRFTokenDriver{}

			tokenTime := time.Now().Add(tt.tokenAge)
			mockDriver.On("GetToken", mock.Anything, "test-token").Return(tokenTime, nil)

			if tt.expectDeleted {
				mockDriver.On("DeleteToken", mock.Anything, "test-token").Return(nil)
			}

			// Create gateway
			gateway := NewCSRFTokenGateway(mockDriver)

			// Execute test
			ctx := context.Background()
			valid, err := gateway.ValidateToken(ctx, "test-token")

			// Verify results
			assert.NoError(t, err)
			assert.Equal(t, tt.expectValid, valid)

			// Verify mock expectations
			mockDriver.AssertExpectations(t)
		})
	}
}
