package service

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"pre-processor-sidecar/driver"
	"pre-processor-sidecar/models"
)

// TestUpdateTokenDirectly tests the UpdateTokenDirectly method that avoids OAuth2 API conflicts
func TestUpdateTokenDirectly(t *testing.T) {
	// Setup logger for testing
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create OAuth2 client for testing
	oauth2Client := driver.NewOAuth2Client("test-client-id", "test-client-secret", "https://test.example.com")

	// Create test token manager
	config := InMemoryTokenManagerConfig{
		ClientID:         "test-client-id",
		ClientSecret:     "test-client-secret",
		AccessToken:      "initial-access-token",
		RefreshToken:     "initial-refresh-token",
		RefreshBuffer:    5 * time.Minute,
		CheckInterval:    1 * time.Minute,
		OAuth2Client:     oauth2Client,
		Logger:           logger,
		MetricsCollector: &NoOpMetricsCollector{},
	}

	manager, err := NewInMemoryTokenManager(config)
	if err != nil {
		t.Fatalf("Failed to create token manager: %v", err)
	}
	defer manager.Stop()

	tests := []struct {
		name        string
		token       *models.OAuth2Token
		expectError bool
		description string
	}{
		{
			name: "UpdateTokenDirectly_Success_WithReadWriteScope",
			token: &models.OAuth2Token{
				AccessToken:  "new-access-token-from-auth-token-manager",
				RefreshToken: "new-refresh-token-from-auth-token-manager",
				TokenType:    "Bearer",
				ExpiresAt:    time.Now().Add(24 * time.Hour),
				Scope:        "read write", // Fixed scope from auth-token-manager
			},
			expectError: false,
			description: "Should successfully update token without OAuth2 API call",
		},
		{
			name: "UpdateTokenDirectly_Success_WithReadScope",
			token: &models.OAuth2Token{
				AccessToken:  "another-access-token",
				RefreshToken: "another-refresh-token",
				TokenType:    "Bearer",
				ExpiresAt:    time.Now().Add(12 * time.Hour),
				Scope:        "read", // Legacy scope
			},
			expectError: false,
			description: "Should handle legacy read-only scope",
		},
		{
			name: "UpdateTokenDirectly_Success_EmptyTokenType",
			token: &models.OAuth2Token{
				AccessToken:  "token-without-type",
				RefreshToken: "refresh-without-type",
				TokenType:    "", // Empty token type should default to Bearer
				ExpiresAt:    time.Now().Add(6 * time.Hour),
				Scope:        "read write",
			},
			expectError: false,
			description: "Should default empty token type to Bearer",
		},
		{
			name: "UpdateTokenDirectly_Failure_EmptyAccessToken",
			token: &models.OAuth2Token{
				AccessToken:  "", // Empty access token should fail
				RefreshToken: "valid-refresh-token",
				TokenType:    "Bearer",
				ExpiresAt:    time.Now().Add(1 * time.Hour),
				Scope:        "read write",
			},
			expectError: true,
			description: "Should fail with empty access token",
		},
		{
			name: "UpdateTokenDirectly_Failure_EmptyRefreshToken",
			token: &models.OAuth2Token{
				AccessToken:  "valid-access-token",
				RefreshToken: "", // Empty refresh token should fail
				TokenType:    "Bearer",
				ExpiresAt:    time.Now().Add(1 * time.Hour),
				Scope:        "read write",
			},
			expectError: true,
			description: "Should fail with empty refresh token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			
			// Execute UpdateTokenDirectly
			err := manager.UpdateTokenDirectly(ctx, tt.token)

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

			// Verify token was updated correctly
			status := manager.GetTokenStatus()
			
			// Check basic status fields
			if !status.HasAccessToken {
				t.Error("Token manager should have access token after UpdateTokenDirectly")
			}
			if !status.HasRefreshToken {
				t.Error("Token manager should have refresh token after UpdateTokenDirectly")
			}

			// Check expiry time was updated
			expectedExpiry := tt.token.ExpiresAt.Truncate(time.Second)
			actualExpiry := status.ExpiresAt.Truncate(time.Second)
			if !expectedExpiry.Equal(actualExpiry) {
				t.Errorf("Expected expires_at %v, got %v", expectedExpiry, actualExpiry)
			}

			// Check token type (should default to Bearer if empty)
			expectedTokenType := tt.token.TokenType
			if expectedTokenType == "" {
				expectedTokenType = "Bearer"
			}
			if status.TokenType != expectedTokenType {
				t.Errorf("Expected token_type %s, got %s", expectedTokenType, status.TokenType)
			}

			// Verify we can retrieve a valid token (this tests internal encryption/decryption)
			retrievedToken, err := manager.GetValidToken(ctx)
			if err != nil {
				t.Errorf("Failed to retrieve token after UpdateTokenDirectly: %v", err)
				return
			}

			if retrievedToken.AccessToken != tt.token.AccessToken {
				t.Errorf("Retrieved access token doesn't match. Expected %s, got %s", 
					tt.token.AccessToken, retrievedToken.AccessToken)
			}

			if retrievedToken.TokenType != expectedTokenType {
				t.Errorf("Retrieved token type doesn't match. Expected %s, got %s", 
					expectedTokenType, retrievedToken.TokenType)
			}
		})
	}
}

// TestUpdateTokenDirectly_ConflictAvoidance tests the core conflict avoidance functionality
func TestUpdateTokenDirectly_ConflictAvoidance(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	oauth2Client := driver.NewOAuth2Client("test-client", "test-secret", "https://test.example.com")

	config := InMemoryTokenManagerConfig{
		ClientID:         "test-client",
		ClientSecret:     "test-secret",
		AccessToken:      "initial-token",
		RefreshToken:     "initial-refresh",
		OAuth2Client:     oauth2Client,
		Logger:           logger,
		MetricsCollector: &NoOpMetricsCollector{},
	}

	manager, err := NewInMemoryTokenManager(config)
	if err != nil {
		t.Fatalf("Failed to create token manager: %v", err)
	}
	defer manager.Stop()

	// Simulate auth-token-manager updating the Secret
	authManagerToken := &models.OAuth2Token{
		AccessToken:  "auth-manager-access-token",
		RefreshToken: "auth-manager-refresh-token", 
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		Scope:        "read write", // Fixed scope from auth-token-manager
	}

	ctx := context.Background()

	// Test 1: UpdateTokenDirectly should succeed without OAuth2 API call
	err = manager.UpdateTokenDirectly(ctx, authManagerToken)
	if err != nil {
		t.Fatalf("UpdateTokenDirectly failed: %v", err)
	}

	// Test 2: Verify the token was updated correctly
	retrievedToken, err := manager.GetValidToken(ctx)
	if err != nil {
		t.Fatalf("Failed to get valid token after UpdateTokenDirectly: %v", err)
	}

	if retrievedToken.AccessToken != authManagerToken.AccessToken {
		t.Errorf("Access token not updated. Expected %s, got %s", 
			authManagerToken.AccessToken, retrievedToken.AccessToken)
	}

	// Test 3: Multiple rapid updates (simulating rapid Secret changes)
	for i := 0; i < 5; i++ {
		rapidUpdateToken := &models.OAuth2Token{
			AccessToken:  "rapid-update-token-" + string(rune('A'+i)),
			RefreshToken: "rapid-refresh-token-" + string(rune('A'+i)),
			TokenType:    "Bearer",
			ExpiresAt:    time.Now().Add(time.Duration(20+i) * time.Hour),
			Scope:        "read write",
		}

		err = manager.UpdateTokenDirectly(ctx, rapidUpdateToken)
		if err != nil {
			t.Errorf("Rapid update %d failed: %v", i, err)
		}

		// Verify each update
		currentToken, err := manager.GetValidToken(ctx)
		if err != nil {
			t.Errorf("Failed to get token after rapid update %d: %v", i, err)
		}

		if currentToken.AccessToken != rapidUpdateToken.AccessToken {
			t.Errorf("Rapid update %d: token not updated correctly", i)
		}
	}
}

// TestUpdateTokenDirectly_ThreadSafety tests concurrent access safety
func TestUpdateTokenDirectly_ThreadSafety(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError})) // Reduce log noise
	oauth2Client := driver.NewOAuth2Client("test-client", "test-secret", "https://test.example.com")

	config := InMemoryTokenManagerConfig{
		ClientID:         "test-client",
		ClientSecret:     "test-secret",
		AccessToken:      "initial-token",
		RefreshToken:     "initial-refresh",
		OAuth2Client:     oauth2Client,
		Logger:           logger,
		MetricsCollector: &NoOpMetricsCollector{},
	}

	manager, err := NewInMemoryTokenManager(config)
	if err != nil {
		t.Fatalf("Failed to create token manager: %v", err)
	}
	defer manager.Stop()

	// Concurrent updates test
	numGoroutines := 10
	done := make(chan bool, numGoroutines)
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()

			token := &models.OAuth2Token{
				AccessToken:  "concurrent-token-" + string(rune('0'+id)),
				RefreshToken: "concurrent-refresh-" + string(rune('0'+id)),
				TokenType:    "Bearer",
				ExpiresAt:    time.Now().Add(time.Duration(id+1) * time.Hour),
				Scope:        "read write",
			}

			ctx := context.Background()
			if err := manager.UpdateTokenDirectly(ctx, token); err != nil {
				errors <- err
				return
			}

			// Also test GetValidToken concurrently
			if _, err := manager.GetValidToken(ctx); err != nil {
				errors <- err
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		select {
		case <-done:
			// Good
		case err := <-errors:
			t.Errorf("Concurrent access error: %v", err)
		case <-time.After(10 * time.Second):
			t.Fatal("Concurrent test timed out")
		}
	}

	// Check for any remaining errors
	select {
	case err := <-errors:
		t.Errorf("Additional concurrent access error: %v", err)
	default:
		// No additional errors
	}
}