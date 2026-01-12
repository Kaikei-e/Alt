package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	kratos "github.com/ory/kratos-client-go"
)

// Identity represents user identity information from Kratos
type Identity struct {
	ID        string
	Email     string
	CreatedAt time.Time
	SessionID string // KratosのセッションID
}

// KratosClient handles communication with Ory Kratos
type KratosClient struct {
	client       *kratos.APIClient
	adminBaseURL string
	httpClient   *http.Client
}

// NewKratosClient creates a new Kratos API client
func NewKratosClient(baseURL string, timeout time.Duration) *KratosClient {
	return NewKratosClientWithAdmin(baseURL, "", timeout)
}

// NewKratosClientWithAdmin creates a new Kratos API client with Admin API support
func NewKratosClientWithAdmin(baseURL, adminBaseURL string, timeout time.Duration) *KratosClient {
	configuration := kratos.NewConfiguration()
	configuration.Servers = []kratos.ServerConfiguration{
		{
			URL: baseURL,
		},
	}
	httpClient := &http.Client{
		Timeout: timeout,
	}
	configuration.HTTPClient = httpClient

	return &KratosClient{
		client:       kratos.NewAPIClient(configuration),
		adminBaseURL: adminBaseURL,
		httpClient:   httpClient,
	}
}

// Whoami validates a session cookie and returns identity information
func (c *KratosClient) Whoami(ctx context.Context, cookie string) (*Identity, error) {
	if cookie == "" {
		return nil, fmt.Errorf("cookie cannot be empty")
	}

	// Call Kratos API using the SDK
	// The SDK handles the request creation and execution
	session, resp, err := c.client.FrontendAPI.ToSession(ctx).Cookie(cookie).Execute()
	if err != nil {
		if resp != nil {
			if resp.StatusCode == http.StatusUnauthorized {
				return nil, fmt.Errorf("authentication failed: session invalid or expired")
			}
			return nil, fmt.Errorf("kratos returned status %d: %w", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("failed to call kratos: %w", err)
	}

	// Validate session is active
	if session.Active != nil && !*session.Active {
		return nil, fmt.Errorf("session is not active")
	}

	// Validate identity exists
	if session.Identity == nil {
		return nil, fmt.Errorf("missing identity in response")
	}

	// Extract email from traits
	email := ""
	if traits, ok := session.Identity.Traits.(map[string]interface{}); ok {
		if emailVal, ok := traits["email"]; ok {
			if emailStr, ok := emailVal.(string); ok {
				email = emailStr
			}
		}
	}

	var createdAt time.Time
	if session.Identity.CreatedAt != nil {
		createdAt = *session.Identity.CreatedAt
	}

	// セッションIDを取得
	sessionID := session.Id

	return &Identity{
		ID:        session.Identity.Id,
		Email:     email,
		CreatedAt: createdAt,
		SessionID: sessionID, // KratosのセッションIDを設定
	}, nil
}

// adminIdentity represents a Kratos identity from Admin API
type adminIdentity struct {
	ID string `json:"id"`
}

// GetFirstIdentityID fetches the first identity ID from Kratos Admin API.
// This is used for internal service operations that need a system user.
func (c *KratosClient) GetFirstIdentityID(ctx context.Context) (string, error) {
	if c.adminBaseURL == "" {
		return "", fmt.Errorf("admin base URL not configured")
	}

	url := fmt.Sprintf("%s/admin/identities?page_size=1", c.adminBaseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch identities: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch identities: status %d", resp.StatusCode)
	}

	var identities []adminIdentity
	if err := json.NewDecoder(resp.Body).Decode(&identities); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(identities) == 0 {
		return "", fmt.Errorf("no identities found in Kratos")
	}

	return identities[0].ID, nil
}
