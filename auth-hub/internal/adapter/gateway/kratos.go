package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"auth-hub/internal/domain"

	kratos "github.com/ory/kratos-client-go"
)

// KratosGateway implements domain.SessionValidator and domain.IdentityProvider.
type KratosGateway struct {
	client       *kratos.APIClient
	adminBaseURL string
	httpClient   *http.Client
}

// NewKratosGateway creates a new Kratos gateway with tuned HTTP transport.
func NewKratosGateway(baseURL, adminBaseURL string, timeout time.Duration) *KratosGateway {
	configuration := kratos.NewConfiguration()
	configuration.Servers = []kratos.ServerConfiguration{
		{URL: baseURL},
	}

	transport := &http.Transport{
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 50,
		IdleConnTimeout:     90 * time.Second,
	}

	httpClient := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
	configuration.HTTPClient = httpClient

	return &KratosGateway{
		client:       kratos.NewAPIClient(configuration),
		adminBaseURL: adminBaseURL,
		httpClient:   httpClient,
	}
}

// ValidateSession validates a session cookie and returns the identity.
func (g *KratosGateway) ValidateSession(ctx context.Context, cookie string) (*domain.Identity, error) {
	if cookie == "" {
		return nil, domain.ErrSessionNotFound
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	session, resp, err := g.client.FrontendAPI.ToSession(ctx).Cookie(cookie).Execute()
	if err != nil {
		if resp != nil {
			if resp.StatusCode == http.StatusUnauthorized {
				return nil, domain.ErrAuthFailed
			}
			if resp.StatusCode == http.StatusTooManyRequests {
				return nil, fmt.Errorf("%w: kratos returned status 429", domain.ErrRateLimited)
			}
			return nil, fmt.Errorf("%w: kratos returned status %d", domain.ErrKratosUnavailable, resp.StatusCode)
		}
		return nil, fmt.Errorf("%w: %w", domain.ErrKratosUnavailable, err)
	}

	if session.Active != nil && !*session.Active {
		return nil, domain.ErrSessionInactive
	}

	if session.Identity == nil {
		return nil, domain.ErrMissingIdentity
	}

	email := ""
	role := "user"
	if traits, ok := session.Identity.Traits.(map[string]interface{}); ok {
		if emailVal, ok := traits["email"]; ok {
			if emailStr, ok := emailVal.(string); ok {
				email = emailStr
			}
		}
		if roleVal, ok := traits["role"]; ok {
			if roleStr, ok := roleVal.(string); ok && roleStr == "admin" {
				role = "admin"
			}
		}
	}

	var createdAt time.Time
	if session.Identity.CreatedAt != nil {
		createdAt = *session.Identity.CreatedAt
	}

	return &domain.Identity{
		UserID:    session.Identity.Id,
		Email:     email,
		Role:      role,
		SessionID: session.Id,
		CreatedAt: createdAt,
	}, nil
}

// adminIdentity represents a Kratos identity from Admin API.
type adminIdentity struct {
	ID string `json:"id"`
}

// GetFirstIdentityID fetches the first identity ID from Kratos Admin API.
func (g *KratosGateway) GetFirstIdentityID(ctx context.Context) (string, error) {
	if g.adminBaseURL == "" {
		return "", domain.ErrAdminNotConfigured
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/admin/identities?page_size=1", g.adminBaseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("%w: %w", domain.ErrKratosUnavailable, err)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %w", domain.ErrKratosUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: admin API returned status %d", domain.ErrKratosUnavailable, resp.StatusCode)
	}

	var identities []adminIdentity
	if err := json.NewDecoder(resp.Body).Decode(&identities); err != nil {
		return "", fmt.Errorf("%w: %w", domain.ErrKratosUnavailable, err)
	}

	if len(identities) == 0 {
		return "", domain.ErrNoIdentitiesFound
	}

	return identities[0].ID, nil
}
