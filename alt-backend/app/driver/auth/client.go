package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"alt/config"
	"alt/domain"
)

// AuthClient defines the interface for Auth Service operations
type AuthClient interface {
	ValidateSession(ctx context.Context, sessionToken string, tenantID string) (*SessionValidationResponse, error)
	GenerateCSRFToken(ctx context.Context, sessionToken string) (*CSRFTokenResponse, error)
	ValidateCSRFToken(ctx context.Context, token, sessionToken string) (*CSRFValidationResponse, error)
	HealthCheck(ctx context.Context) error
}

// Client represents an Auth Service client wrapper
type Client struct {
	baseURL    string
	httpClient *http.Client
	logger     *slog.Logger
}

// Ensure Client implements AuthClient interface
var _ AuthClient = (*Client)(nil)

// SessionValidationRequest represents the request to validate a session
type SessionValidationRequest struct {
	SessionToken string `json:"session_token"`
	TenantID     string `json:"tenant_id,omitempty"`
}

// SessionValidationResponse represents the response from session validation
type SessionValidationResponse struct {
	Valid   bool                `json:"valid"`
	UserID  string              `json:"user_id,omitempty"`
	Email   string              `json:"email,omitempty"`
	Role    string              `json:"role,omitempty"`
	Context *domain.UserContext `json:"context,omitempty"`
}

// CSRFTokenRequest represents the request to generate a CSRF token
type CSRFTokenRequest struct {
	SessionToken string `json:"session_token"`
}

// CSRFTokenResponse represents the response containing a CSRF token
type CSRFTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// CSRFValidationRequest represents the request to validate a CSRF token
type CSRFValidationRequest struct {
	Token        string `json:"token"`
	SessionToken string `json:"session_token"`
}

// CSRFValidationResponse represents the response from CSRF validation
type CSRFValidationResponse struct {
	Valid bool `json:"valid"`
}

// NewClient creates a new Auth Service client
func NewClient(config *config.Config, logger *slog.Logger) *Client {
	timeout := config.Auth.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	serviceURL := config.Auth.ServiceURL
	if serviceURL == "" {
		serviceURL = config.AuthServiceURL // fallback to legacy field
	}

	return &Client{
		baseURL: serviceURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		logger: logger,
	}
}

// ValidateSession validates a session token with the Auth Service
func (c *Client) ValidateSession(ctx context.Context, sessionToken string, tenantID string) (*SessionValidationResponse, error) {
	// Input validation
	if sessionToken == "" {
		return c.createInvalidSessionResponse(), nil // Return invalid session for empty token
	}

	req := SessionValidationRequest{
		SessionToken: sessionToken,
		TenantID:     tenantID,
	}

	response, err := c.makeRequest(ctx, "POST", "/api/v1/session/validate", req)
	if err != nil {
		c.logger.Warn("auth service unavailable, returning invalid session for OptionalAuth compatibility",
			"error", err,
			"session_token_prefix", sessionToken[:min(len(sessionToken), 8)])
		// Return invalid session instead of error for OptionalAuth middleware compatibility
		return c.createInvalidSessionResponse(), nil
	}

	var result SessionValidationResponse
	if err := json.Unmarshal(response, &result); err != nil {
		c.logger.Error("failed to unmarshal session validation response", "error", err)
		return c.createInvalidSessionResponse(), nil
	}

	return &result, nil
}

// GenerateCSRFToken generates a CSRF token for the given session
func (c *Client) GenerateCSRFToken(ctx context.Context, sessionToken string) (*CSRFTokenResponse, error) {
	req := CSRFTokenRequest{
		SessionToken: sessionToken,
	}

	response, err := c.makeRequest(ctx, "POST", "/api/v1/csrf/generate", req)
	if err != nil {
		c.logger.Error("CSRF token generation failed",
			"error", err,
			"session_token_prefix", sessionToken[:min(len(sessionToken), 8)])
		return nil, fmt.Errorf("failed to generate CSRF token: %w", err)
	}

	var result CSRFTokenResponse
	if err := json.Unmarshal(response, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal CSRF token response: %w", err)
	}

	return &result, nil
}

// ValidateCSRFToken validates a CSRF token with the given session
func (c *Client) ValidateCSRFToken(ctx context.Context, token, sessionToken string) (*CSRFValidationResponse, error) {
	req := CSRFValidationRequest{
		Token:        token,
		SessionToken: sessionToken,
	}

	response, err := c.makeRequest(ctx, "POST", "/api/v1/csrf/validate", req)
	if err != nil {
		c.logger.Error("CSRF token validation failed",
			"error", err,
			"csrf_token_prefix", token[:min(len(token), 8)],
			"session_token_prefix", sessionToken[:min(len(sessionToken), 8)])
		return nil, fmt.Errorf("failed to validate CSRF token: %w", err)
	}

	var result CSRFValidationResponse
	if err := json.Unmarshal(response, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal CSRF validation response: %w", err)
	}

	return &result, nil
}

// HealthCheck checks if the Auth Service is healthy
func (c *Client) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	response, err := c.makeRequest(ctx, "GET", "/health", nil)
	if err != nil {
		c.logger.Debug("auth service health check failed, service may be unavailable", "error", err)
		return nil // Graceful handling - service unavailability is not a fatal error for the application
	}

	var healthResponse map[string]interface{}
	if err := json.Unmarshal(response, &healthResponse); err != nil {
		c.logger.Debug("failed to parse auth service health response", "error", err)
		return nil // Graceful handling
	}

	status, ok := healthResponse["status"].(string)
	if !ok || status != "ok" {
		c.logger.Debug("auth service is unhealthy", "status", healthResponse["status"])
		return nil // Graceful handling
	}

	c.logger.Debug("auth service is healthy")
	return nil
}

// makeRequest is a helper method to make HTTP requests to the Auth Service
func (c *Client) makeRequest(ctx context.Context, method, endpoint string, payload interface{}) ([]byte, error) {
	var body io.Reader

	if payload != nil {
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request payload: %w", err)
		}
		body = bytes.NewBuffer(jsonData)
	}

	url := c.baseURL + endpoint
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	c.logger.Debug("making auth service request",
		"method", method,
		"url", url)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("auth service error: status=%d, body=%s", resp.StatusCode, string(responseBody))
	}

	return responseBody, nil
}

// createInvalidSessionResponse creates a response for invalid/failed sessions
// This is used for graceful handling when the auth service is unavailable
func (c *Client) createInvalidSessionResponse() *SessionValidationResponse {
	return &SessionValidationResponse{
		Valid:  false,
		UserID: "",
		Email:  "",
		Role:   "",
	}
}

// min returns the minimum of two integers (helper function for Go versions < 1.21)
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
