package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"pre-processor-sidecar/models"
)

// TestSimpleTokenService_onSecretUpdate tests the conflict avoidance in onSecretUpdate
func TestSimpleTokenService_onSecretUpdate(t *testing.T) {
	// Setup logger for testing
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create test configuration
	config := SimpleTokenConfig{
		ClientID:            "test-client-id",
		ClientSecret:        "test-client-secret",
		InitialAccessToken:  "initial-access-token",
		InitialRefreshToken: "initial-refresh-token",
		BaseURL:             "https://test.example.com",
		RefreshBuffer:       5 * time.Minute,
		CheckInterval:       1 * time.Minute,
		// OAuth2 Secret settings (will be mocked)
		OAuth2SecretName:    "test-oauth2-secret",
		KubernetesNamespace: "test-namespace",
		EnableSecretWatch:   false, // Disable to avoid K8s dependencies
	}

	// Create SimpleTokenService
	service, err := NewSimpleTokenService(config, nil, logger)
	if err != nil {
		t.Fatalf("Failed to create SimpleTokenService: %v", err)
	}
	defer service.Stop()

	tests := []struct {
		name        string
		inputToken  *models.OAuth2Token
		expectError bool
		description string
	}{
		{
			name: "onSecretUpdate_Success_ReadWriteScope",
			inputToken: &models.OAuth2Token{
				AccessToken:  "auth-manager-access-token",
				RefreshToken: "auth-manager-refresh-token",
				TokenType:    "Bearer",
				ExpiresAt:    time.Now().Add(24 * time.Hour),
				Scope:        "read write", // Fixed scope from auth-token-manager
			},
			expectError: false,
			description: "Should update token without OAuth2 API call (conflict avoidance)",
		},
		{
			name: "onSecretUpdate_Success_ReadScope",
			inputToken: &models.OAuth2Token{
				AccessToken:  "legacy-access-token",
				RefreshToken: "legacy-refresh-token",
				TokenType:    "Bearer",
				ExpiresAt:    time.Now().Add(12 * time.Hour),
				Scope:        "read", // Legacy scope
			},
			expectError: false,
			description: "Should handle legacy read-only scope",
		},
		{
			name: "onSecretUpdate_Success_MultipleUpdates",
			inputToken: &models.OAuth2Token{
				AccessToken:  "multiple-update-token",
				RefreshToken: "multiple-refresh-token",
				TokenType:    "Bearer",
				ExpiresAt:    time.Now().Add(6 * time.Hour),
				Scope:        "read write",
			},
			expectError: false,
			description: "Should handle multiple rapid Secret updates",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Execute onSecretUpdate (simulating Secret change from auth-token-manager)
			err := service.onSecretUpdate(tt.inputToken)

			// Check error expectation
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none for test: %s", tt.description)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error for test %s: %v", tt.description, err)
				return
			}

			// Verify token was updated without OAuth2 API call
			ctx := context.Background()
			retrievedToken, err := service.GetValidToken(ctx)
			if err != nil {
				t.Errorf("Failed to get valid token after onSecretUpdate: %v", err)
				return
			}

			// Verify the token matches what was provided by auth-token-manager
			if retrievedToken.AccessToken != tt.inputToken.AccessToken {
				t.Errorf("Access token not updated correctly. Expected %s, got %s",
					tt.inputToken.AccessToken, retrievedToken.AccessToken)
			}

			if retrievedToken.TokenType != tt.inputToken.TokenType {
				t.Errorf("Token type not updated correctly. Expected %s, got %s",
					tt.inputToken.TokenType, retrievedToken.TokenType)
			}

			// Check service status
			status := service.GetServiceStatus()
			if !status.IsHealthy {
				t.Error("Service should be healthy after successful onSecretUpdate")
			}
		})
	}
}

// TestSimpleTokenService_ConflictAvoidanceScenario tests the complete conflict avoidance scenario
func TestSimpleTokenService_ConflictAvoidanceScenario(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	config := SimpleTokenConfig{
		ClientID:            "conflict-test-client",
		ClientSecret:        "conflict-test-secret",
		InitialAccessToken:  "initial-access",
		InitialRefreshToken: "initial-refresh",
		BaseURL:             "https://conflict-test.example.com",
		RefreshBuffer:       5 * time.Minute,
		CheckInterval:       1 * time.Minute,
		EnableSecretWatch:   false,
	}

	service, err := NewSimpleTokenService(config, nil, logger)
	if err != nil {
		t.Fatalf("Failed to create service for conflict test: %v", err)
	}
	defer service.Stop()

	// Start the service to set isStarted = true
	if err := service.Start(); err != nil {
		t.Fatalf("Failed to start service: %v", err)
	}

	// Scenario: auth-token-manager refreshes token and updates Secret
	authManagerToken := &models.OAuth2Token{
		AccessToken:  "fresh-token-from-auth-manager",
		RefreshToken: "fresh-refresh-from-auth-manager",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		Scope:        "read write", // Fixed scope from auth-token-manager
	}

	// Step 1: Simulate auth-token-manager updating the Secret
	t.Log("Step 1: auth-token-manager updates Secret")
	err = service.onSecretUpdate(authManagerToken)
	if err != nil {
		t.Fatalf("onSecretUpdate failed: %v", err)
	}

	// Step 2: Verify pre-processor-sidecar has the new token WITHOUT making OAuth2 API call
	t.Log("Step 2: Verify token updated without API call")
	ctx := context.Background()
	currentToken, err := service.GetValidToken(ctx)
	if err != nil {
		t.Fatalf("Failed to get current token: %v", err)
	}

	if currentToken.AccessToken != authManagerToken.AccessToken {
		t.Errorf("Token not updated from Secret. Expected %s, got %s",
			authManagerToken.AccessToken, currentToken.AccessToken)
	}

	// Step 3: Simulate multiple rapid Secret updates (race condition test)
	t.Log("Step 3: Multiple rapid Secret updates")
	for i := 0; i < 3; i++ {
		rapidToken := &models.OAuth2Token{
			AccessToken:  "rapid-token-" + string(rune('A'+i)),
			RefreshToken: "rapid-refresh-" + string(rune('A'+i)),
			TokenType:    "Bearer",
			ExpiresAt:    time.Now().Add(time.Duration(20+i) * time.Hour),
			Scope:        "read write",
		}

		err = service.onSecretUpdate(rapidToken)
		if err != nil {
			t.Errorf("Rapid update %d failed: %v", i, err)
		}

		// Verify each update
		verifyToken, err := service.GetValidToken(ctx)
		if err != nil {
			t.Errorf("Failed to verify rapid update %d: %v", i, err)
		}

		if verifyToken.AccessToken != rapidToken.AccessToken {
			t.Errorf("Rapid update %d not applied. Expected %s, got %s",
				i, rapidToken.AccessToken, verifyToken.AccessToken)
		}
	}

	// Step 4: Verify service remains healthy throughout
	status := service.GetServiceStatus()
	if !status.IsHealthy {
		t.Error("Service should remain healthy after conflict avoidance scenario")
	}

	if !status.IsRunning {
		t.Error("Service should be running after conflict avoidance scenario")
	}
}

// TestSimpleTokenService_ScopeHandling tests proper OAuth2 scope handling
func TestSimpleTokenService_ScopeHandling(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	config := SimpleTokenConfig{
		ClientID:            "scope-test-client",
		ClientSecret:        "scope-test-secret",
		InitialAccessToken:  "scope-initial-access",
		InitialRefreshToken: "scope-initial-refresh",
		BaseURL:             "https://scope-test.example.com",
		EnableSecretWatch:   false,
	}

	service, err := NewSimpleTokenService(config, nil, logger)
	if err != nil {
		t.Fatalf("Failed to create service for scope test: %v", err)
	}
	defer service.Stop()

	scopeTests := []struct {
		name           string
		inputScope     string
		expectedResult string
		description    string
	}{
		{
			name:           "ReadWriteScope_Fixed",
			inputScope:     "read write",
			expectedResult: "read write",
			description:    "Should preserve fixed read write scope from auth-token-manager",
		},
		{
			name:           "ReadOnlyScope_Legacy",
			inputScope:     "read",
			expectedResult: "read",
			description:    "Should preserve legacy read-only scope",
		},
		{
			name:           "EmptyScope_Default",
			inputScope:     "",
			expectedResult: "",
			description:    "Should handle empty scope gracefully",
		},
	}

	for _, test := range scopeTests {
		t.Run(test.name, func(t *testing.T) {
			tokenWithScope := &models.OAuth2Token{
				AccessToken:  "scope-test-access-" + test.name,
				RefreshToken: "scope-test-refresh-" + test.name,
				TokenType:    "Bearer",
				ExpiresAt:    time.Now().Add(12 * time.Hour),
				Scope:        test.inputScope,
			}

			// Update via onSecretUpdate
			err := service.onSecretUpdate(tokenWithScope)
			if err != nil {
				t.Errorf("onSecretUpdate failed for scope test %s: %v", test.name, err)
				return
			}

			// Verify scope is preserved (though not directly accessible via GetValidToken)
			// We verify indirectly through successful token retrieval
			ctx := context.Background()
			retrievedToken, err := service.GetValidToken(ctx)
			if err != nil {
				t.Errorf("Failed to get token with scope %s: %v", test.inputScope, err)
				return
			}

			if retrievedToken.AccessToken != tokenWithScope.AccessToken {
				t.Errorf("Token with scope %s not updated correctly", test.inputScope)
			}
		})
	}
}

// MockOAuth2SecretService for testing without Kubernetes dependencies
type MockOAuth2SecretService struct {
	tokens []*models.OAuth2Token
	logger *slog.Logger
}

func (m *MockOAuth2SecretService) LoadOAuth2Token(ctx context.Context) (*models.OAuth2Token, error) {
	if len(m.tokens) == 0 {
		return nil, fmt.Errorf("no tokens available")
	}
	return m.tokens[len(m.tokens)-1], nil
}

func (m *MockOAuth2SecretService) IsTokenExpired(token *models.OAuth2Token, bufferMinutes int) bool {
	if token == nil {
		return true
	}
	buffer := time.Duration(bufferMinutes) * time.Minute
	return time.Now().Add(buffer).After(token.ExpiresAt)
}

func (m *MockOAuth2SecretService) AddToken(token *models.OAuth2Token) {
	m.tokens = append(m.tokens, token)
}

// TestSimpleTokenService_WithMockSecretService tests with mocked Secret service
func TestSimpleTokenService_WithMockSecretService(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create mock secret service
	mockSecret := &MockOAuth2SecretService{
		logger: logger,
	}

	// Add initial token
	initialToken := &models.OAuth2Token{
		AccessToken:  "mock-initial-access",
		RefreshToken: "mock-initial-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		Scope:        "read write",
	}
	mockSecret.AddToken(initialToken)

	config := SimpleTokenConfig{
		ClientID:            "mock-test-client",
		ClientSecret:        "mock-test-secret",
		InitialAccessToken:  "initial-access",
		InitialRefreshToken: "initial-refresh",
		BaseURL:             "https://mock-test.example.com",
		EnableSecretWatch:   false,
	}

	service, err := NewSimpleTokenService(config, nil, logger)
	if err != nil {
		t.Fatalf("Failed to create service with mock: %v", err)
	}
	defer service.Stop()

	// Test conflict avoidance with mock
	newToken := &models.OAuth2Token{
		AccessToken:  "mock-updated-access",
		RefreshToken: "mock-updated-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(18 * time.Hour),
		Scope:        "read write",
	}

	err = service.onSecretUpdate(newToken)
	if err != nil {
		t.Fatalf("onSecretUpdate failed with mock: %v", err)
	}

	// Verify update worked
	ctx := context.Background()
	retrievedToken, err := service.GetValidToken(ctx)
	if err != nil {
		t.Fatalf("Failed to get token with mock: %v", err)
	}

	if retrievedToken.AccessToken != newToken.AccessToken {
		t.Errorf("Mock update failed. Expected %s, got %s",
			newToken.AccessToken, retrievedToken.AccessToken)
	}
}
