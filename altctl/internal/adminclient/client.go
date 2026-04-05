// Package adminclient provides an HTTP client for calling alt-backend's
// Connect-RPC admin API using plain HTTP/1.1 + JSON encoding.
package adminclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// ServicePrefix is the Connect-RPC service path prefix.
	ServicePrefix = "/alt.knowledge_home.v1.KnowledgeHomeAdminService/"
)

// AdminClient is an HTTP client for the Knowledge Home admin API.
type AdminClient struct {
	BaseURL      string
	ServiceToken string
	HTTPClient   *http.Client
}

// APIError represents a non-2xx response from the admin API.
type APIError struct {
	StatusCode int
	Code       string `json:"code"`
	Message    string `json:"message"`
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("admin API error (HTTP %d): [%s] %s", e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("admin API error (HTTP %d)", e.StatusCode)
}

// NewClient creates a new AdminClient with sensible defaults.
func NewClient(baseURL, serviceToken string) *AdminClient {
	return &AdminClient{
		BaseURL:      baseURL,
		ServiceToken: serviceToken,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Call invokes a Connect-RPC method via HTTP POST with JSON encoding.
// method is the RPC method name (e.g. "StartReproject").
// reqBody and respBody are JSON-serializable structs or maps.
func (c *AdminClient) Call(ctx context.Context, method string, reqBody, respBody interface{}) error {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	url := c.BaseURL + ServicePrefix + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.ServiceToken != "" {
		req.Header.Set("X-Service-Token", c.ServiceToken)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", method, err)
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &APIError{StatusCode: resp.StatusCode}
		// Try to parse error details from response body
		_ = json.Unmarshal(respData, apiErr)
		return apiErr
	}

	if respBody != nil {
		if err := json.Unmarshal(respData, respBody); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}
	}

	return nil
}
