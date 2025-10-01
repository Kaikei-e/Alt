package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Identity represents user identity information from Kratos
type Identity struct {
	ID    string
	Email string
}

// KratosClient handles communication with Ory Kratos
type KratosClient struct {
	baseURL    string
	httpClient *http.Client
}

// kratosSessionResponse represents the Kratos /sessions/whoami response
type kratosSessionResponse struct {
	ID       string           `json:"id"`
	Active   bool             `json:"active"`
	Identity *kratosIdentity  `json:"identity"`
}

type kratosIdentity struct {
	ID     string         `json:"id"`
	Traits map[string]any `json:"traits"`
}

// NewKratosClient creates a new Kratos API client
func NewKratosClient(baseURL string, timeout time.Duration) *KratosClient {
	return &KratosClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// Whoami validates a session cookie and returns identity information
func (c *KratosClient) Whoami(ctx context.Context, cookie string) (*Identity, error) {
	if cookie == "" {
		return nil, fmt.Errorf("cookie cannot be empty")
	}

	// Create request with context for timeout/cancellation
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/sessions/whoami", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add session cookie
	req.Header.Set("Cookie", cookie)

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call kratos: %w", err)
	}
	defer resp.Body.Close()

	// Handle non-2xx status codes
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed: session invalid or expired")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kratos returned status %d", resp.StatusCode)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse JSON response
	var sessionResp kratosSessionResponse
	if err := json.Unmarshal(body, &sessionResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Validate session is active
	if !sessionResp.Active {
		return nil, fmt.Errorf("session is not active")
	}

	// Validate identity exists
	if sessionResp.Identity == nil {
		return nil, fmt.Errorf("missing identity in response")
	}

	// Extract email from traits
	email := ""
	if emailVal, ok := sessionResp.Identity.Traits["email"]; ok {
		if emailStr, ok := emailVal.(string); ok {
			email = emailStr
		}
	}

	return &Identity{
		ID:    sessionResp.Identity.ID,
		Email: email,
	}, nil
}
