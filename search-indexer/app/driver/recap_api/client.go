package recap_api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// RecapGenreItem represents a single recap genre from the recap-worker API.
type RecapGenreItem struct {
	JobID      string   `json:"job_id"`
	ExecutedAt string   `json:"executed_at"`
	WindowDays int      `json:"window_days"`
	Genre      string   `json:"genre"`
	Summary    string   `json:"summary"`
	TopTerms   []string `json:"top_terms"`
	Tags       []string `json:"tags"`
	Bullets    []string `json:"bullets"`
}

// IndexableGenresResponse is the response from recap-worker's indexable endpoint.
type IndexableGenresResponse struct {
	Results []RecapGenreItem `json:"results"`
	HasMore bool             `json:"has_more"`
}

// Client communicates with recap-worker's HTTP API.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new recap-worker API client.
func NewClient(baseURL string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    baseURL,
	}
}

// GetIndexableGenres fetches recap genres for Meilisearch indexing.
func (c *Client) GetIndexableGenres(ctx context.Context, since string, limit int) (*IndexableGenresResponse, error) {
	u, err := url.Parse(fmt.Sprintf("%s/v1/recaps/genres/indexable", c.baseURL))
	if err != nil {
		return nil, fmt.Errorf("invalid recap-worker URL: %w", err)
	}

	q := u.Query()
	q.Set("limit", fmt.Sprintf("%d", limit))
	if since != "" {
		q.Set("since", since)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch indexable genres: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("recap-worker returned status %d", resp.StatusCode)
	}

	var result IndexableGenresResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}
