package recap_worker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"rag-orchestrator/internal/domain"
)

// Client fetches morning letter documents from recap-worker REST API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new recap-worker client.
func NewClient(baseURL string, httpClient *http.Client) *Client {
	return &Client{baseURL: baseURL, httpClient: httpClient}
}

// letterResponse is the JSON structure returned by recap-worker REST endpoints.
type letterResponse struct {
	ID         string     `json:"id"`
	TargetDate string     `json:"target_date"`
	Body       letterBody `json:"body"`
}

type letterBody struct {
	Lead     string          `json:"lead"`
	Sections []letterSection `json:"sections"`
}

type letterSection struct {
	Key     string   `json:"key"`
	Title   string   `json:"title"`
	Bullets []string `json:"bullets"`
}

func (c *Client) FetchLatest(ctx context.Context) (*domain.MorningLetterDoc, error) {
	return c.fetch(ctx, fmt.Sprintf("%s/v1/morning/letters/latest", c.baseURL))
}

func (c *Client) FetchByDate(ctx context.Context, targetDate string) (*domain.MorningLetterDoc, error) {
	return c.fetch(ctx, fmt.Sprintf("%s/v1/morning/letters/%s", c.baseURL, targetDate))
}

func (c *Client) fetch(ctx context.Context, url string) (*domain.MorningLetterDoc, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("recap_worker: create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("recap_worker: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("recap_worker: unexpected status %d", resp.StatusCode)
	}

	var lr letterResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return nil, fmt.Errorf("recap_worker: decode response: %w", err)
	}

	sections := make([]domain.MorningLetterDocSection, len(lr.Body.Sections))
	for i, s := range lr.Body.Sections {
		sections[i] = domain.MorningLetterDocSection{
			Key:     s.Key,
			Title:   s.Title,
			Bullets: s.Bullets,
		}
	}

	return &domain.MorningLetterDoc{
		Lead:     lr.Body.Lead,
		Sections: sections,
	}, nil
}
