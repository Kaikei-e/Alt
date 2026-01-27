// ABOUTME: This file tests OAuth2 token models and validation logic
// ABOUTME: Ensures proper token expiration checking and refresh logic

package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOAuth2Token(t *testing.T) {
	tests := map[string]struct {
		response             InoreaderTokenResponse
		existingRefreshToken string
		validate             func(t *testing.T, token *OAuth2Token)
	}{
		"full_response_with_refresh_token": {
			response: InoreaderTokenResponse{
				AccessToken:  "new_access_token",
				TokenType:    "Bearer",
				ExpiresIn:    3600,
				RefreshToken: "new_refresh_token",
				Scope:        "read write",
			},
			existingRefreshToken: "existing_refresh_token",
			validate: func(t *testing.T, token *OAuth2Token) {
				assert.Equal(t, "new_access_token", token.AccessToken)
				assert.Equal(t, "Bearer", token.TokenType)
				assert.Equal(t, 3600, token.ExpiresIn)
				assert.Equal(t, "new_refresh_token", token.RefreshToken) // Should use new one
				assert.Equal(t, "read write", token.Scope)
				assert.True(t, token.ExpiresAt.After(time.Now()))
				assert.True(t, token.IssuedAt.Before(time.Now().Add(time.Second)))
			},
		},
		"response_without_refresh_token": {
			response: InoreaderTokenResponse{
				AccessToken: "new_access_token",
				TokenType:   "Bearer",
				ExpiresIn:   3600,
				Scope:       "read",
			},
			existingRefreshToken: "existing_refresh_token",
			validate: func(t *testing.T, token *OAuth2Token) {
				assert.Equal(t, "new_access_token", token.AccessToken)
				assert.Equal(t, "existing_refresh_token", token.RefreshToken) // Should use existing
				assert.Equal(t, "read", token.Scope)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			token := NewOAuth2Token(tc.response, tc.existingRefreshToken)
			require.NotNil(t, token)
			if tc.validate != nil {
				tc.validate(t, token)
			}
		})
	}
}

func TestOAuth2Token_IsExpired(t *testing.T) {
	tests := map[string]struct {
		expiresAt time.Time
		expected  bool
	}{
		"not_expired": {
			expiresAt: time.Now().Add(1 * time.Hour),
			expected:  false,
		},
		"expired": {
			expiresAt: time.Now().Add(-1 * time.Hour),
			expected:  true,
		},
		"just_expired": {
			expiresAt: time.Now().Add(-1 * time.Second),
			expected:  true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			token := &OAuth2Token{
				AccessToken: "test_token",
				ExpiresAt:   tc.expiresAt,
			}

			result := token.IsExpired()
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestOAuth2Token_NeedsRefresh(t *testing.T) {
	tests := map[string]struct {
		expiresAt time.Time
		buffer    time.Duration
		expected  bool
	}{
		"needs_refresh_within_buffer": {
			expiresAt: time.Now().Add(3 * time.Minute),
			buffer:    5 * time.Minute,
			expected:  true,
		},
		"does_not_need_refresh": {
			expiresAt: time.Now().Add(10 * time.Minute),
			buffer:    5 * time.Minute,
			expected:  false,
		},
		"expired_needs_refresh": {
			expiresAt: time.Now().Add(-1 * time.Minute),
			buffer:    5 * time.Minute,
			expected:  true,
		},
		"exactly_at_buffer": {
			expiresAt: time.Now().Add(5 * time.Minute),
			buffer:    5 * time.Minute,
			expected:  true, // Should refresh at exactly buffer time
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			token := &OAuth2Token{
				AccessToken: "test_token",
				ExpiresAt:   tc.expiresAt,
			}

			result := token.NeedsRefresh(tc.buffer)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestOAuth2Token_IsValid(t *testing.T) {
	tests := map[string]struct {
		token    *OAuth2Token
		expected bool
	}{
		"valid_token": {
			token: &OAuth2Token{
				AccessToken: "valid_token",
				ExpiresAt:   time.Now().Add(1 * time.Hour),
			},
			expected: true,
		},
		"empty_access_token": {
			token: &OAuth2Token{
				AccessToken: "",
				ExpiresAt:   time.Now().Add(1 * time.Hour),
			},
			expected: false,
		},
		"expired_token": {
			token: &OAuth2Token{
				AccessToken: "expired_token",
				ExpiresAt:   time.Now().Add(-1 * time.Hour),
			},
			expected: false,
		},
		"empty_and_expired": {
			token: &OAuth2Token{
				AccessToken: "",
				ExpiresAt:   time.Now().Add(-1 * time.Hour),
			},
			expected: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := tc.token.IsValid()
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestOAuth2Token_UpdateFromRefresh(t *testing.T) {
	// Create initial token
	token := &OAuth2Token{
		AccessToken:  "old_access_token",
		RefreshToken: "original_refresh_token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		IssuedAt:     time.Now().Add(-2 * time.Hour),
	}

	// Update with refresh response
	refreshResponse := InoreaderTokenResponse{
		AccessToken: "new_access_token",
		TokenType:   "Bearer",
		ExpiresIn:   7200, // 2 hours
		Scope:       "read write",
	}

	beforeUpdate := time.Now()
	token.UpdateFromRefresh(refreshResponse)
	afterUpdate := time.Now()

	// Verify updates
	assert.Equal(t, "new_access_token", token.AccessToken)
	assert.Equal(t, "original_refresh_token", token.RefreshToken) // Should keep original
	assert.Equal(t, "Bearer", token.TokenType)
	assert.Equal(t, 7200, token.ExpiresIn)
	assert.Equal(t, "read write", token.Scope)

	// Verify times
	assert.True(t, token.IssuedAt.After(beforeUpdate) || token.IssuedAt.Equal(beforeUpdate))
	assert.True(t, token.IssuedAt.Before(afterUpdate) || token.IssuedAt.Equal(afterUpdate))
	assert.True(t, token.ExpiresAt.After(time.Now().Add(1*time.Hour))) // Should be ~2 hours from now

	// Test with new refresh token in response
	refreshResponseWithToken := InoreaderTokenResponse{
		AccessToken:  "newer_access_token",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		RefreshToken: "new_refresh_token",
		Scope:        "read",
	}

	token.UpdateFromRefresh(refreshResponseWithToken)

	assert.Equal(t, "newer_access_token", token.AccessToken)
	assert.Equal(t, "new_refresh_token", token.RefreshToken) // Should use new one
	assert.Equal(t, "read", token.Scope)
}

func TestOAuth2Token_TimeUntilExpiry(t *testing.T) {
	futureTime := time.Now().Add(30 * time.Minute)
	token := &OAuth2Token{
		AccessToken: "test_token",
		ExpiresAt:   futureTime,
	}

	duration := token.TimeUntilExpiry()

	// Should be approximately 30 minutes (allowing some test execution time)
	assert.True(t, duration > 29*time.Minute)
	assert.True(t, duration <= 30*time.Minute)

	// Test with past expiry time
	pastTime := time.Now().Add(-10 * time.Minute)
	expiredToken := &OAuth2Token{
		AccessToken: "expired_token",
		ExpiresAt:   pastTime,
	}

	expiredDuration := expiredToken.TimeUntilExpiry()
	assert.True(t, expiredDuration < 0) // Should be negative
}
