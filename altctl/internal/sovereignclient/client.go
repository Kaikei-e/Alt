// Package sovereignclient provides an HTTP client for calling
// knowledge-sovereign's admin REST API on its metrics port.
package sovereignclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// APIError represents a non-2xx response from the sovereign API.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("sovereign API error (HTTP %d): %s", e.StatusCode, e.Body)
	}
	return fmt.Sprintf("sovereign API error (HTTP %d)", e.StatusCode)
}

// SovereignClient is an HTTP client for the knowledge-sovereign admin API.
type SovereignClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewClient creates a new SovereignClient.
func NewClient(baseURL string) *SovereignClient {
	return &SovereignClient{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Get performs an HTTP GET and decodes the JSON response into respBody.
func (c *SovereignClient) Get(ctx context.Context, path string, respBody interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	return c.do(req, respBody)
}

// Post performs an HTTP POST with a JSON body and decodes the response.
func (c *SovereignClient) Post(ctx context.Context, path string, reqBody, respBody interface{}) error {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	return c.do(req, respBody)
}

func (c *SovereignClient) do(req *http.Request, respBody interface{}) error {
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s %s: %w", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Body: string(respData)}
	}

	if respBody != nil {
		if err := json.Unmarshal(respData, respBody); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}
	}

	return nil
}
