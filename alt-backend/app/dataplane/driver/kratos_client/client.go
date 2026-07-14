// Package kratos_client provides HTTP client for auth-hub service.
// This abstracts the Kratos identity provider behind auth-hub,
// following the BFF/Aggregator pattern.
package kratos_client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// KratosClient defines the interface for identity operations via auth-hub.
type KratosClient interface {
	// GetFirstIdentityID returns the ID of the first identity.
	// This is used to get a system user ID for internal operations.
	GetFirstIdentityID(ctx context.Context) (string, error)
}

// authHubClientImpl implements KratosClient by calling auth-hub.
type authHubClientImpl struct {
	authHubURL   string
	sharedSecret string
	httpClient   *http.Client
}

// systemUserResponse represents the response from auth-hub /internal/system-user endpoint.
type systemUserResponse struct {
	UserID string `json:"user_id"`
}

// NewKratosClient creates a new auth-hub client.
// Note: Despite the name, this now calls auth-hub instead of Kratos directly.
// This provides abstraction so alt-backend doesn't need to know about Kratos.
func NewKratosClient(authHubURL string, sharedSecret string) KratosClient {
	return &authHubClientImpl{
		authHubURL:   authHubURL,
		sharedSecret: sharedSecret,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetFirstIdentityID fetches the system user ID from auth-hub.
func (c *authHubClientImpl) GetFirstIdentityID(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/internal/system-user", c.authHubURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("X-Internal-Auth", c.sharedSecret)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch system user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch system user: status %d", resp.StatusCode)
	}

	var response systemUserResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if response.UserID == "" {
		return "", fmt.Errorf("empty user_id in response")
	}

	return response.UserID, nil
}
