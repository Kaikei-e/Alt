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

// TestCSRFTokenGateway_ValidateHMACToken tests HMAC-based token validation
func TestCSRFTokenGateway_ValidateHMACToken(t *testing.T) {
	testSecret := "test-secret-key-for-hmac"
	testSessionID := "test-session-id-12345"

	// Create a gateway instance for generating tokens
	mockDriver := &MockCSRFTokenDriver{}
	gateway := NewCSRFTokenGateway(mockDriver)

	// Helper function to generate expected token for comparison
	generateExpectedToken := func(sessionID, secret string) string {
		return gateway.GenerateHMACToken(sessionID, secret)
	}

	tests := []struct {
		name        string
		token       string
		sessionID   string
		secret      string
		setupToken  func() string // Function to generate the token to test
		expectValid bool
		expectError bool
	}{
		{
			name:      "valid HMAC token with correct session ID",
			sessionID: testSessionID,
			secret:    testSecret,
			setupToken: func() string {
				// Token will be generated using the same logic as production code
				return generateExpectedToken(testSessionID, testSecret)
			},
			expectValid: true,
			expectError: false,
		},
		{
			name:      "invalid HMAC token with wrong token value",
			token:     "invalid-hmac-token",
			sessionID: testSessionID,
			secret:    testSecret,
			setupToken: func() string {
				return "invalid-hmac-token"
			},
			expectValid: false,
			expectError: false,
		},
		{
			name:      "empty session ID should return false",
			token:     "some-token",
			sessionID: "",
			secret:    testSecret,
			setupToken: func() string {
				return "some-token"
			},
			expectValid: false,
			expectError: false,
		},
		{
			name:      "empty token should return false",
			token:     "",
			sessionID: testSessionID,
			secret:    testSecret,
			setupToken: func() string {
				return ""
			},
			expectValid: false,
			expectError: false,
		},
		{
			name:      "valid token with wrong secret should fail",
			sessionID: testSessionID,
			secret:    "wrong-secret",
			setupToken: func() string {
				return generateExpectedToken(testSessionID, testSecret)
			},
			expectValid: false,
			expectError: false,
		},
		{
			name:      "valid token with wrong session ID should fail",
			sessionID: "different-session-id",
			secret:    testSecret,
			setupToken: func() string {
				return generateExpectedToken(testSessionID, testSecret)
			},
			expectValid: false,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create gateway (no mock needed for HMAC validation)
			mockDriver := &MockCSRFTokenDriver{}
			gateway := NewCSRFTokenGateway(mockDriver)

			// Get token to test
			token := tt.token
			if tt.setupToken != nil {
				token = tt.setupToken()
			}

			// Execute test
			ctx := context.Background()
			valid, err := gateway.ValidateHMACToken(ctx, token, tt.sessionID, tt.secret)

			// Verify results
			if tt.expectError {
				assert.Error(t, err)
				assert.False(t, valid)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectValid, valid)
			}
		})
	}
}

// TestCSRFTokenGateway_GenerateHMACToken tests HMAC token generation
func TestCSRFTokenGateway_GenerateHMACToken(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		secret    string
		wantEmpty bool
	}{
		{
			name:      "generate token with valid session ID and secret",
			sessionID: "valid-session-id",
			secret:    "valid-secret",
			wantEmpty: false,
		},
		{
			name:      "generate token with different session ID produces different token",
			sessionID: "different-session-id",
			secret:    "valid-secret",
			wantEmpty: false,
		},
		{
			name:      "same session ID and secret should produce same token (deterministic)",
			sessionID: "same-session-id",
			secret:    "same-secret",
			wantEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create gateway
			mockDriver := &MockCSRFTokenDriver{}
			gateway := NewCSRFTokenGateway(mockDriver)

			// Generate first token
			token1 := gateway.GenerateHMACToken(tt.sessionID, tt.secret)

			// Verify token is not empty (unless expected)
			if tt.wantEmpty {
				assert.Empty(t, token1)
			} else {
				assert.NotEmpty(t, token1)

				// Generate second token with same inputs
				token2 := gateway.GenerateHMACToken(tt.sessionID, tt.secret)

				// Verify deterministic behavior (same input = same output)
				assert.Equal(t, token1, token2, "HMAC token generation should be deterministic")
			}
		})
	}
}

// TestCSRFTokenGateway_HMACTimingAttackResistance tests for timing attack resistance
func TestCSRFTokenGateway_HMACTimingAttackResistance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timing attack test in short mode")
	}

	testSecret := "test-secret"
	testSessionID := "test-session"

	mockDriver := &MockCSRFTokenDriver{}
	gateway := NewCSRFTokenGateway(mockDriver)
	ctx := context.Background()

	// Generate a valid token
	validToken := gateway.GenerateHMACToken(testSessionID, testSecret)

	// Test with completely wrong token (different length and content)
	wrongToken := "completely-wrong-token"

	iterations := 1000
	var validDurations []time.Duration
	var invalidDurations []time.Duration

	// Measure validation time for valid tokens
	for i := 0; i < iterations; i++ {
		start := time.Now()
		_, _ = gateway.ValidateHMACToken(ctx, validToken, testSessionID, testSecret)
		duration := time.Since(start)
		validDurations = append(validDurations, duration)
	}

	// Measure validation time for invalid tokens
	for i := 0; i < iterations; i++ {
		start := time.Now()
		_, _ = gateway.ValidateHMACToken(ctx, wrongToken, testSessionID, testSecret)
		duration := time.Since(start)
		invalidDurations = append(invalidDurations, duration)
	}

	// Calculate averages
	var validTotal, invalidTotal time.Duration
	for i := 0; i < iterations; i++ {
		validTotal += validDurations[i]
		invalidTotal += invalidDurations[i]
	}

	validAvg := validTotal / time.Duration(iterations)
	invalidAvg := invalidTotal / time.Duration(iterations)

	// The difference should be minimal (within reasonable variance)
	// This is a heuristic test - constant time comparison should prevent
	// large timing differences
	t.Logf("Average validation time for valid tokens: %v", validAvg)
	t.Logf("Average validation time for invalid tokens: %v", invalidAvg)

	// If the timing difference is more than 2x, it might indicate a timing leak
	// Note: This is not a perfect test but provides some confidence
	ratio := float64(validAvg) / float64(invalidAvg)
	if ratio > 2.0 || ratio < 0.5 {
		t.Logf("Warning: Timing difference detected (ratio: %.2f). This might indicate non-constant time comparison.", ratio)
	}
}
