// Package preprocessor_client provides the single driver-layer HTTP client
// for the pre-processor's REST v1 summarization API
// (POST /api/v1/summarize, /api/v1/summarize/stream, /api/v1/summarize/queue,
// GET /api/v1/summarize/status/{id}).
//
// This consolidates what was previously ~600 lines duplicated across
// rest/utils.go, rest/rest_feeds/utils.go, and
// rest/rest_feeds/summarization/helpers.go.
package preprocessor_client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// SummarizeStatus represents the status of an asynchronous summarization job.
type SummarizeStatus struct {
	JobID        string
	Status       string
	Summary      string
	ErrorMessage string
	ArticleID    string
}

// sharedStreamClient is a connection-pooled HTTP client for streaming
// requests. Sharing a client enables keep-alive connection reuse, reducing
// TTFT by eliminating repeated TCP/TLS handshakes.
var sharedStreamClient = &http.Client{
	Timeout: 0, // No timeout for streaming; context cancellation handles cleanup.
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   true,
	},
}

// Client is the driver-layer HTTP client bound to a single pre-processor
// base URL. It is the sole place in alt-backend that speaks the
// pre-processor's REST v1 summarization protocol.
type Client struct {
	baseURL string
}

// NewClient creates a pre-processor REST client bound to baseURL.
func NewClient(baseURL string) *Client {
	return &Client{baseURL: baseURL}
}

// Summarize calls the synchronous summarization endpoint. content may be
// empty when using the pull model (pre-processor reads the article content
// from its own database by articleID).
func (c *Client) Summarize(ctx context.Context, content, articleID, title string) (string, error) {
	if articleID == "" {
		return "", fmt.Errorf("article_id is required")
	}

	jsonData, err := json.Marshal(map[string]string{
		"content":    content,
		"article_id": articleID,
		"title":      title,
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Extended timeout for LLM-based summarization (1000 tokens + continuation generation).
	client := &http.Client{Timeout: 300 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/summarize", bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call pre-processor: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("pre-processor returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var response struct {
		Success   bool   `json:"success"`
		Summary   string `json:"summary"`
		ArticleID string `json:"article_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	if !response.Success {
		return "", fmt.Errorf("summarization failed")
	}
	return response.Summary, nil
}

// StreamSummarize calls the streaming summarization endpoint and returns the
// response body for the caller to read as an SSE stream. The caller is
// responsible for closing the returned ReadCloser.
func (c *Client) StreamSummarize(ctx context.Context, content, articleID, title string) (io.ReadCloser, error) {
	if articleID == "" {
		return nil, fmt.Errorf("article_id is required")
	}

	jsonData, err := json.Marshal(map[string]string{
		"content":    content,
		"article_id": articleID,
		"title":      title,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/summarize/stream", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := sharedStreamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call pre-processor stream: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		errorBody := string(bodyBytes)
		if readErr != nil {
			errorBody = fmt.Sprintf("(failed to read error body: %v)", readErr)
		}
		return nil, fmt.Errorf("pre-processor stream returned status %d: %s", resp.StatusCode, errorBody)
	}

	return resp.Body, nil
}

// QueueSummarize submits an article for asynchronous summarization and
// returns the job ID.
func (c *Client) QueueSummarize(ctx context.Context, articleID, title string) (string, error) {
	if articleID == "" {
		return "", fmt.Errorf("article_id is required")
	}

	jsonData, err := json.Marshal(map[string]string{
		"article_id": articleID,
		"title":      title,
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/summarize/queue", bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call pre-processor: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusAccepted {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("pre-processor returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var response struct {
		JobID   string `json:"job_id"`
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	return response.JobID, nil
}

// GetSummarizeStatus checks the status of an asynchronous summarization job.
// Returns (nil, nil) when the job is not found.
func (c *Client) GetSummarizeStatus(ctx context.Context, jobID string) (*SummarizeStatus, error) {
	if jobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}

	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/v1/summarize/status/%s", c.baseURL, jobID), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call pre-processor: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("pre-processor returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var response struct {
		JobID        string `json:"job_id"`
		Status       string `json:"status"`
		Summary      string `json:"summary,omitempty"`
		ErrorMessage string `json:"error_message,omitempty"`
		ArticleID    string `json:"article_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &SummarizeStatus{
		JobID:        response.JobID,
		Status:       response.Status,
		Summary:      response.Summary,
		ErrorMessage: response.ErrorMessage,
		ArticleID:    response.ArticleID,
	}, nil
}
