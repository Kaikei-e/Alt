// ABOUTME: Shared OAuth2 token value types used across the TokenManager /
// ABOUTME: TokenProvider interfaces (RemoteTokenService, admin API handler).
package service

import "time"

// TokenInfo represents a point-in-time OAuth2 token snapshot.
type TokenInfo struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	TokenType    string
}

// TokenStatus is the admin-facing token status summary returned by
// TokenManager.GetTokenStatus().
type TokenStatus struct {
	HasAccessToken   bool      `json:"has_access_token"`
	HasRefreshToken  bool      `json:"has_refresh_token"`
	ExpiresAt        time.Time `json:"expires_at"`
	ExpiresInSeconds int64     `json:"expires_in_seconds"`
	TokenType        string    `json:"token_type"`
	NeedsRefresh     bool      `json:"needs_refresh"`
	IsAutoRefreshing bool      `json:"is_auto_refreshing"`
}
