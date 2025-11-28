package client

import (
	"context"
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
}

// KratosClient handles communication with Ory Kratos
type KratosClient struct {
	client *kratos.APIClient
}

// NewKratosClient creates a new Kratos API client
func NewKratosClient(baseURL string, timeout time.Duration) *KratosClient {
	configuration := kratos.NewConfiguration()
	configuration.Servers = []kratos.ServerConfiguration{
		{
			URL: baseURL,
		},
	}
	configuration.HTTPClient = &http.Client{
		Timeout: timeout,
	}

	return &KratosClient{
		client: kratos.NewAPIClient(configuration),
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

	return &Identity{
		ID:        session.Identity.Id,
		Email:     email,
		CreatedAt: createdAt,
	}, nil
}
