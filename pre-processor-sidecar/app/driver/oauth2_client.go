package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"pre-processor-sidecar/models"
)

// OAuth2TokenResponse represents the response from OAuth2 token endpoint
type OAuth2TokenResponse struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	ExpiresIn    int       `json:"expires_in"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"-"` // Calculated field
}

// OAuth2Client handles OAuth2 authentication with Inoreader API
type OAuth2Client struct {
	clientID     string
	clientSecret string
	baseURL      string
	httpClient   *http.Client
}

// NewOAuth2Client creates a new OAuth2 client for Inoreader API
func NewOAuth2Client(clientID, clientSecret, baseURL string) *OAuth2Client {
	return &OAuth2Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		baseURL:      baseURL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // 30秒から60秒に増加
			Transport: &http.Transport{
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
				IdleConnTimeout:       90 * time.Second, // キー修正: 30秒から90秒に増加
				MaxIdleConns:          10,
				MaxIdleConnsPerHost:   2,
			},
		},
	}
}

// RefreshToken exchanges a refresh token for a new access token
func (c *OAuth2Client) RefreshToken(ctx context.Context, refreshToken string) (*models.InoreaderTokenResponse, error) {
	// Prepare form data for OAuth2 refresh token request
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/oauth2/token", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "pre-processor-sidecar/1.0")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute refresh token request: %w", err)
	}
	defer resp.Body.Close()

	// Check for HTTP errors FIRST before parsing JSON
	if resp.StatusCode != http.StatusOK {
		// Read error response for debugging
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OAuth2 refresh token failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse JSON response only after confirming success
	var tokenResponse OAuth2TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	// Convert to models.InoreaderTokenResponse
	inoreaderResponse := &models.InoreaderTokenResponse{
		AccessToken:  tokenResponse.AccessToken,
		TokenType:    tokenResponse.TokenType,
		ExpiresIn:    tokenResponse.ExpiresIn,
		RefreshToken: tokenResponse.RefreshToken,
		Scope:        "", // Will be populated if available in response
	}

	return inoreaderResponse, nil
}

// ValidateToken checks if an access token is valid by making a test API call
func (c *OAuth2Client) ValidateToken(ctx context.Context, accessToken string) (bool, error) {
	// Make a lightweight API call to verify token validity
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/user-info", nil)
	if err != nil {
		return false, fmt.Errorf("failed to create token validation request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", "pre-processor-sidecar/1.0")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to execute token validation request: %w", err)
	}
	defer resp.Body.Close()

	// Token is valid if we get 200 OK
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return false, nil
	default:
		return false, fmt.Errorf("unexpected response status: %d", resp.StatusCode)
	}
}

// MakeAuthenticatedRequest makes an authenticated API request to Inoreader
func (c *OAuth2Client) MakeAuthenticatedRequest(ctx context.Context, accessToken, endpoint string, params map[string]string) (map[string]interface{}, error) {
	// Build request URL with parameters
	reqURL := c.baseURL + endpoint
	if len(params) > 0 {
		values := url.Values{}
		for key, value := range params {
			values.Set(key, value)
		}
		reqURL += "?" + values.Encode()
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create authenticated request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", "pre-processor-sidecar/1.0")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute authenticated request: %w", err)
	}
	defer resp.Body.Close()

	// Check for rate limit or authentication errors
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("API rate limit exceeded (Zone 1: %s/%s)", 
			resp.Header.Get("X-Reader-Zone1-Usage"), resp.Header.Get("X-Reader-Zone1-Limit"))
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed: token may be expired or invalid")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	// Parse JSON response
	var responseData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&responseData); err != nil {
		return nil, fmt.Errorf("failed to decode API response: %w", err)
	}

	return responseData, nil
}

// handleRateLimitHeaders extracts and parses rate limit information from response headers
func (c *OAuth2Client) handleRateLimitHeaders(headers map[string]string) (usage, limit, remaining int) {
	// Default values
	limit = 100 // Inoreader Zone 1 default limit

	// Parse usage header
	if usageStr, ok := headers["X-Reader-Zone1-Usage"]; ok {
		if parsedUsage, err := strconv.Atoi(usageStr); err == nil {
			usage = parsedUsage
		}
	}

	// Parse limit header
	if limitStr, ok := headers["X-Reader-Zone1-Limit"]; ok {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil {
			limit = parsedLimit
		}
	}

	// Calculate remaining requests
	remaining = limit - usage
	if remaining < 0 {
		remaining = 0
	}

	return usage, limit, remaining
}

// SetHTTPClient allows injecting a custom HTTP client (useful for testing with proxies)
func (c *OAuth2Client) SetHTTPClient(client *http.Client) {
	c.httpClient = client
}

// SetTimeout sets the HTTP client timeout for testing purposes
func (c *OAuth2Client) SetTimeout(timeout time.Duration) {
	c.httpClient.Timeout = timeout
}

// MakeAuthenticatedRequestWithHeaders makes an authenticated API request and returns response with headers
func (c *OAuth2Client) MakeAuthenticatedRequestWithHeaders(ctx context.Context, accessToken, endpoint string, params map[string]string) (map[string]interface{}, map[string]string, error) {
	// Build request URL with parameters
	reqURL := c.baseURL + endpoint
	if len(params) > 0 {
		values := url.Values{}
		for key, value := range params {
			values.Set(key, value)
		}
		reqURL += "?" + values.Encode()
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create authenticated request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", "pre-processor-sidecar/1.0")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to execute authenticated request: %w", err)
	}
	defer resp.Body.Close()

	// Extract headers
	headers := make(map[string]string)
	for key, values := range resp.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	// Check for rate limit or authentication errors
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, headers, fmt.Errorf("API rate limit exceeded (Zone 1: %s/%s)", 
			resp.Header.Get("X-Reader-Zone1-Usage"), resp.Header.Get("X-Reader-Zone1-Limit"))
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, headers, fmt.Errorf("authentication failed: token may be expired or invalid")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, headers, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	// Parse JSON response
	var responseData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&responseData); err != nil {
		return nil, headers, fmt.Errorf("failed to decode API response: %w", err)
	}

	return responseData, headers, nil
}

// GetRateLimitInfo returns current rate limit information from the last API call
func (c *OAuth2Client) GetRateLimitInfo() map[string]interface{} {
	// This would typically be populated from response headers in a real implementation
	// For now, return basic structure for testing
	return map[string]interface{}{
		"zone1_usage":   0,
		"zone1_limit":   100,
		"zone1_remaining": 100,
	}
}

// DebugDirectRequest makes a direct API call without proxy for debugging
func (c *OAuth2Client) DebugDirectRequest(ctx context.Context, accessToken, endpoint string) error {
	// Create a direct HTTP client without proxy
	directClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 2,
			IdleConnTimeout:     30 * time.Second,
			// No proxy configuration
		},
	}

	reqURL := c.baseURL + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create direct debug request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", "pre-processor-sidecar/1.0")

	resp, err := directClient.Do(req)
	if err != nil {
		return fmt.Errorf("direct debug request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil // Success
	}
	
	return fmt.Errorf("direct debug request failed with status %d", resp.StatusCode)
}