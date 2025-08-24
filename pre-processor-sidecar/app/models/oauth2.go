// ABOUTME: This file defines domain models for OAuth2 token management
// ABOUTME: Handles access token, refresh token, and expiration logic

package models

import (
	"os"
	"strconv"
	"time"
)

// OAuth2Token represents an OAuth2 access token with metadata
type OAuth2Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int       `json:"expires_in"`    // Seconds until expiration
	ExpiresAt    time.Time `json:"expires_at"`    // Calculated expiration time
	Scope        string    `json:"scope"`
	IssuedAt     time.Time `json:"issued_at"`     // When token was issued
}

// getClockSkewTolerance returns the clock skew tolerance from environment variable or default
func getClockSkewTolerance() time.Duration {
	if skewStr := os.Getenv("OAUTH2_CLOCK_SKEW_SECONDS"); skewStr != "" {
		if skewSeconds, err := strconv.Atoi(skewStr); err == nil {
			return time.Duration(skewSeconds) * time.Second
		}
	}
	// Default to 60 seconds clock skew tolerance
	return 60 * time.Second
}

// InoreaderTokenResponse represents the OAuth2 token response from Inoreader API
type InoreaderTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"` // May not be present in refresh responses
	Scope        string `json:"scope"`
}

// NewOAuth2Token creates a new OAuth2Token from Inoreader API response
func NewOAuth2Token(response InoreaderTokenResponse, existingRefreshToken string) *OAuth2Token {
	now := time.Now()
	expiresAt := now.Add(time.Duration(response.ExpiresIn) * time.Second)

	// Use existing refresh token if not provided in response
	refreshToken := response.RefreshToken
	if refreshToken == "" {
		refreshToken = existingRefreshToken
	}

	return &OAuth2Token{
		AccessToken:  response.AccessToken,
		RefreshToken: refreshToken,
		TokenType:    response.TokenType,
		ExpiresIn:    response.ExpiresIn,
		ExpiresAt:    expiresAt,
		Scope:        response.Scope,
		IssuedAt:     now,
	}
}

// IsExpired checks if the token is expired with clock skew tolerance
func (t *OAuth2Token) IsExpired() bool {
	// Add clock skew tolerance to account for time differences between servers
	clockSkew := getClockSkewTolerance()
	return time.Now().Add(clockSkew).After(t.ExpiresAt)
}

// NeedsRefresh checks if the token needs to be refreshed based on buffer time with clock skew tolerance
func (t *OAuth2Token) NeedsRefresh(buffer time.Duration) bool {
	// Add both buffer time and clock skew tolerance
	clockSkew := getClockSkewTolerance()
	totalBuffer := buffer + clockSkew
	return time.Now().Add(totalBuffer).After(t.ExpiresAt)
}

// TimeUntilExpiry returns the duration until token expiry
func (t *OAuth2Token) TimeUntilExpiry() time.Duration {
	return time.Until(t.ExpiresAt)
}

// IsValid checks if the token is valid and not expired
func (t *OAuth2Token) IsValid() bool {
	return t.AccessToken != "" && !t.IsExpired()
}

// UpdateFromRefresh updates the token with new access token information
func (t *OAuth2Token) UpdateFromRefresh(response InoreaderTokenResponse) {
	now := time.Now()
	
	t.AccessToken = response.AccessToken
	t.TokenType = response.TokenType
	t.ExpiresIn = response.ExpiresIn
	t.ExpiresAt = now.Add(time.Duration(response.ExpiresIn) * time.Second)
	t.Scope = response.Scope
	t.IssuedAt = now

	// Update refresh token if provided (usually not provided in refresh responses)
	if response.RefreshToken != "" {
		t.RefreshToken = response.RefreshToken
	}
}