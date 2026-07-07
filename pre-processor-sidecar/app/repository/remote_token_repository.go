// ABOUTME: This file implements the RemoteTokenRepository which fetches OAuth2 tokens from auth-token-manager service
// ABOUTME: It acts as a read-only client to the centralized token management service

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"pre-processor-sidecar/models"
	"time"
)

// RemoteTokenRepository fetches tokens from the auth-token-manager API
type RemoteTokenRepository struct {
	managerURL        string
	internalAuthToken string
	client            *http.Client
	logger            *slog.Logger
}

// TokenResponse represents the JSON response from auth-token-manager
type TokenResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"`
	Scope        string    `json:"scope"`
}

// NewRemoteTokenRepository creates a new remote token repository. internalAuthToken is sent
// as the X-Internal-Auth header on every request — auth-token-manager fails closed (401/503)
// without a matching token, so an empty value here means the token source starves silently.
func NewRemoteTokenRepository(managerURL string, internalAuthToken string, logger *slog.Logger) *RemoteTokenRepository {
	return &RemoteTokenRepository{
		managerURL:        managerURL,
		internalAuthToken: internalAuthToken,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// GetCurrentToken retrieves the current OAuth2 token from the remote service
func (r *RemoteTokenRepository) GetCurrentToken(ctx context.Context) (*models.OAuth2Token, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", r.managerURL+"/api/token", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("X-Internal-Auth", r.internalAuthToken)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch token from auth-token-manager: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrTokenNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("auth-token-manager returned status: %d", resp.StatusCode)
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return nil, ErrTokenNotFound
	}

	return &models.OAuth2Token{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresAt:    tokenResp.ExpiresAt,
		TokenType:    tokenResp.TokenType,
		Scope:        tokenResp.Scope,
	}, nil
}

// SaveToken is a no-op for the remote repository (read-only)
func (r *RemoteTokenRepository) SaveToken(ctx context.Context, token *models.OAuth2Token) error {
	r.logger.Warn("SaveToken called on RemoteTokenRepository (read-only), ignoring")
	return nil
}

// UpdateToken is a no-op for the remote repository (read-only)
func (r *RemoteTokenRepository) UpdateToken(ctx context.Context, token *models.OAuth2Token) error {
	r.logger.Warn("UpdateToken called on RemoteTokenRepository (read-only), ignoring")
	return nil
}

// DeleteToken is a no-op for the remote repository (read-only)
func (r *RemoteTokenRepository) DeleteToken(ctx context.Context) error {
	r.logger.Warn("DeleteToken called on RemoteTokenRepository (read-only), ignoring")
	return nil
}
