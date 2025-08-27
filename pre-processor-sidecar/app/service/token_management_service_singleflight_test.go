// ABOUTME: Tests for Single-Flight pattern and enhanced error handling in TokenManagementService
// ABOUTME: Verifies concurrent refresh protection and proper error categorization

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"pre-processor-sidecar/driver"
	"pre-processor-sidecar/models"
	"pre-processor-sidecar/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenManagementService_SingleFlight_ConcurrentRefresh(t *testing.T) {
	// Setup mock OAuth2 server
	refreshCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth2/token" {
			refreshCount++
			
			// Simulate slow response to ensure concurrent calls
			time.Sleep(100 * time.Millisecond)
			
			// Return successful token refresh
			response := map[string]interface{}{
				"access_token":  fmt.Sprintf("new-access-token-%d", refreshCount),
				"token_type":    "Bearer",
				"expires_in":    3600,
				"refresh_token": "new-refresh-token",
				"scope":         "read",
			}
			
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			return
		}
		
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Setup components
	logger := slog.Default()
	repo := repository.NewInMemoryTokenRepository()
	
	// Create expired token to force refresh
	expiredToken := &models.OAuth2Token{
		AccessToken:  "expired-token",
		RefreshToken: "valid-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-1 * time.Hour), // Expired
		IssuedAt:     time.Now().Add(-2 * time.Hour),
	}
	
	err := repo.SaveToken(context.Background(), expiredToken)
	require.NoError(t, err)

	// Create OAuth2 client pointing to mock server
	oauth2Client := driver.NewOAuth2Client("test-client", "test-secret", server.URL, logger)
	
	// Create token management service
	tokenService := NewTokenManagementService(repo, oauth2Client, logger)

	// Test: Launch 5 concurrent refresh requests
	const numConcurrent = 5
	var wg sync.WaitGroup
	results := make(chan *models.OAuth2Token, numConcurrent)
	errors := make(chan error, numConcurrent)

	for i := 0; i < numConcurrent; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			token, err := tokenService.EnsureValidToken(context.Background())
			if err != nil {
				errors <- fmt.Errorf("goroutine %d: %w", id, err)
			} else {
				results <- token
			}
		}(i)
	}

	wg.Wait()
	close(results)
	close(errors)

	// Assertions
	// Check that no errors occurred
	for err := range errors {
		t.Errorf("Unexpected error: %v", err)
	}

	// Check that all goroutines got valid tokens
	tokens := make([]*models.OAuth2Token, 0, numConcurrent)
	for token := range results {
		tokens = append(tokens, token)
	}

	assert.Len(t, tokens, numConcurrent, "All goroutines should get tokens")
	
	// All tokens should be identical (same refresh operation)
	for i := 1; i < len(tokens); i++ {
		assert.Equal(t, tokens[0].AccessToken, tokens[i].AccessToken, "All tokens should be identical")
		assert.Equal(t, tokens[0].RefreshToken, tokens[i].RefreshToken, "All refresh tokens should be identical")
	}

	// Most importantly: Only ONE refresh should have occurred
	assert.Equal(t, 1, refreshCount, "Single-flight should ensure only one refresh call")
}

func TestTokenManagementService_ErrorHandling_InvalidRefreshToken(t *testing.T) {
	// Setup mock OAuth2 server that returns 401 with invalid_grant
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth2/token" {
			w.WriteHeader(http.StatusUnauthorized)
			response := map[string]interface{}{
				"error":             "invalid_grant",
				"error_description": "The refresh token is invalid or expired",
			}
			json.NewEncoder(w).Encode(response)
			return
		}
		
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Setup components
	logger := slog.Default()
	repo := repository.NewInMemoryTokenRepository()
	
	// Create expired token
	expiredToken := &models.OAuth2Token{
		AccessToken:  "expired-token",
		RefreshToken: "invalid-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-1 * time.Hour),
		IssuedAt:     time.Now().Add(-2 * time.Hour),
	}
	
	err := repo.SaveToken(context.Background(), expiredToken)
	require.NoError(t, err)

	// Create OAuth2 client and service
	oauth2Client := driver.NewOAuth2Client("test-client", "test-secret", server.URL, logger)
	tokenService := NewTokenManagementService(repo, oauth2Client, logger)

	// Test: Should fail immediately without retries
	_, err = tokenService.EnsureValidToken(context.Background())
	
	// Assertions
	assert.Error(t, err)
	assert.True(t, errors.Is(err, driver.ErrInvalidRefreshToken), "Should return ErrInvalidRefreshToken")
	assert.Contains(t, err.Error(), "non-retryable", "Error should indicate it's non-retryable")
}

func TestTokenManagementService_ErrorHandling_RateLimited(t *testing.T) {
	// Setup mock OAuth2 server that returns 429 rate limit
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth2/token" {
			attemptCount++
			
			if attemptCount <= 2 {
				// First two attempts are rate limited
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				fmt.Fprintf(w, "Rate limit exceeded")
				return
			} else {
				// Third attempt succeeds
				response := map[string]interface{}{
					"access_token":  "new-access-token",
					"token_type":    "Bearer",
					"expires_in":    3600,
					"refresh_token": "new-refresh-token",
					"scope":         "read",
				}
				
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
				return
			}
		}
		
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Setup components
	logger := slog.Default()
	repo := repository.NewInMemoryTokenRepository()
	
	expiredToken := &models.OAuth2Token{
		AccessToken:  "expired-token",
		RefreshToken: "valid-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-1 * time.Hour),
		IssuedAt:     time.Now().Add(-2 * time.Hour),
	}
	
	err := repo.SaveToken(context.Background(), expiredToken)
	require.NoError(t, err)

	// Create OAuth2 client and service
	oauth2Client := driver.NewOAuth2Client("test-client", "test-secret", server.URL, logger)
	tokenService := NewTokenManagementService(repo, oauth2Client, logger)

	// Test: Should retry and eventually succeed
	startTime := time.Now()
	token, err := tokenService.EnsureValidToken(context.Background())
	duration := time.Since(startTime)

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, token)
	assert.Equal(t, "new-access-token", token.AccessToken)
	assert.Equal(t, 3, attemptCount, "Should make 3 attempts (2 failures + 1 success)")
	
	// Should have taken some time due to backoff (at least 30 seconds for first retry)
	// But we'll make it shorter for test speed
	assert.True(t, duration > 100*time.Millisecond, "Should have some backoff delay")
}

func TestTokenManagementService_ClockSkewTolerance(t *testing.T) {
	// Test that clock skew tolerance affects token expiration checks
	
	// First, set clock skew to 10 seconds for baseline test
	t.Setenv("OAUTH2_CLOCK_SKEW_SECONDS", "10")
	
	// Create token that expires in 30 seconds (should not be expired with 10s skew)
	token := &models.OAuth2Token{
		AccessToken:  "test-token",
		RefreshToken: "refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(30 * time.Second),
		IssuedAt:     time.Now(),
	}

	// With 10-second clock skew, token expiring in 30 seconds should not be expired
	assert.False(t, token.IsExpired(), "Token should not be expired with 10s clock skew")

	// Now set clock skew environment variable to 60 seconds
	t.Setenv("OAUTH2_CLOCK_SKEW_SECONDS", "60")
	
	// With 60-second clock skew, token expiring in 30 seconds should be considered expired
	assert.True(t, token.IsExpired(), "Token should be expired with 60s clock skew tolerance")
	
	// Test NeedsRefresh with different buffer scenarios
	buffer := 5 * time.Minute
	assert.True(t, token.NeedsRefresh(buffer), "Token should need refresh with buffer + clock skew")
	
	// Test with very long token (2 hours) - should not be expired even with clock skew
	longToken := &models.OAuth2Token{
		AccessToken:  "long-token",
		RefreshToken: "refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(2 * time.Hour),
		IssuedAt:     time.Now(),
	}
	
	assert.False(t, longToken.IsExpired(), "Long-lived token should not be expired even with clock skew")
}