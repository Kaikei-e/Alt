// ABOUTME: This file tests the admin API handler functionality
// ABOUTME: Following TDD principles with comprehensive test coverage for HTTP endpoints

package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"pre-processor-sidecar/models"
	"pre-processor-sidecar/security"
	"pre-processor-sidecar/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock implementations for testing
type MockTokenManager struct{}

func (m *MockTokenManager) UpdateRefreshToken(ctx context.Context, refreshToken string, clientID, clientSecret string) error {
	return nil
}

func (m *MockTokenManager) GetTokenStatus() service.TokenStatus {
	expiresAt, _ := time.Parse(time.RFC3339, "2024-01-01T00:00:00Z")
	return service.TokenStatus{
		HasAccessToken:  true,
		HasRefreshToken: true,
		ExpiresAt:       expiresAt,
		TokenType:       "bearer",
	}
}

func (m *MockTokenManager) GetValidToken(ctx context.Context) (*service.TokenInfo, error) {
	expiresAt, _ := time.Parse(time.RFC3339, "2024-01-01T00:00:00Z")
	return &service.TokenInfo{
		AccessToken:  "test_token",
		RefreshToken: "test_refresh_token",
		ExpiresAt:    expiresAt,
		TokenType:    "bearer",
	}, nil
}

type MockAdminAuthenticator struct{}

func (m *MockAdminAuthenticator) ValidateKubernetesServiceAccountToken(token string) (*security.ServiceAccountInfo, error) {
	return &security.ServiceAccountInfo{
		Subject:   "system:serviceaccount:test:test",
		Namespace: "test",
		Name:      "test",
		UID:       "test-uid",
	}, nil
}

func (m *MockAdminAuthenticator) HasAdminPermissions(info *security.ServiceAccountInfo) bool {
	return true
}

type MockRateLimiter struct{}

func (m *MockRateLimiter) IsAllowed(clientIP string, endpoint string) bool {
	return true
}

func (m *MockRateLimiter) RecordRequest(clientIP string, endpoint string) {}

type MockInputValidator struct{}

func (m *MockInputValidator) ValidateTokenUpdateRequest(req *models.TokenUpdateRequest) error {
	return nil
}

func (m *MockInputValidator) SanitizeString(input string) string {
	return input
}

type MockAdminAPIMetricsCollector struct{}

func (m *MockAdminAPIMetricsCollector) IncrementAdminAPIRequest(method, endpoint, status string) {}
func (m *MockAdminAPIMetricsCollector) RecordAdminAPIRequestDuration(method, endpoint string, duration time.Duration) {
}
func (m *MockAdminAPIMetricsCollector) IncrementAdminAPIRateLimitHit()                        {}
func (m *MockAdminAPIMetricsCollector) IncrementAdminAPIAuthenticationError(errorType string) {}

func TestAdminAPIHandler_HandleTokenStatus(t *testing.T) {
	tests := map[string]struct {
		method        string
		expectStatus  int
		checkResponse func(t *testing.T, body []byte)
	}{
		"successful_token_status": {
			method:       http.MethodGet,
			expectStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Contains(t, response, "status")
				assert.Equal(t, "success", response["status"])
			},
		},
		"post_method_not_allowed": {
			method:       http.MethodPost,
			expectStatus: http.StatusMethodNotAllowed,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// Create mock dependencies
			mockTokenManager := &MockTokenManager{}
			mockAuth := &MockAdminAuthenticator{}
			mockRateLimit := &MockRateLimiter{}
			mockValidator := &MockInputValidator{}
			mockMetrics := &MockAdminAPIMetricsCollector{}

			// Create handler
			handler := NewAdminAPIHandler(
				mockTokenManager,
				mockAuth,
				mockRateLimit,
				mockValidator,
				slog.Default(),
				mockMetrics,
			)

			// Create request
			req := httptest.NewRequest(tc.method, "/admin/token/status", nil)
			req.Header.Set("Authorization", "Bearer test_token")

			// Create response recorder
			recorder := httptest.NewRecorder()

			// Call handler method directly
			handler.HandleTokenStatus(recorder, req)

			// Check status code
			assert.Equal(t, tc.expectStatus, recorder.Code)

			// Check response if checker provided
			if tc.checkResponse != nil {
				tc.checkResponse(t, recorder.Body.Bytes())
			}
		})
	}
}

func TestAdminAPIHandler_HandleRefreshTokenUpdate(t *testing.T) {
	// Basic test for refresh token update handler
	mockTokenManager := &MockTokenManager{}
	mockAuth := &MockAdminAuthenticator{}
	mockRateLimit := &MockRateLimiter{}
	mockValidator := &MockInputValidator{}
	mockMetrics := &MockAdminAPIMetricsCollector{}

	handler := NewAdminAPIHandler(
		mockTokenManager,
		mockAuth,
		mockRateLimit,
		mockValidator,
		slog.Default(),
		mockMetrics,
	)

	// Test GET method not allowed
	req := httptest.NewRequest(http.MethodGet, "/admin/oauth2/refresh-token", nil)
	recorder := httptest.NewRecorder()

	handler.HandleRefreshTokenUpdate(recorder, req)

	assert.Equal(t, http.StatusMethodNotAllowed, recorder.Code)
}
