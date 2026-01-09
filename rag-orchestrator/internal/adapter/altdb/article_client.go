package altdb

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"rag-orchestrator/internal/domain"
	"time"

	"github.com/google/uuid"
)

// HTTPArticleClient implements domain.ArticleClient using HTTP calls to alt-backend
type HTTPArticleClient struct {
	baseURL    string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewHTTPArticleClient creates a new HTTP-based article client
func NewHTTPArticleClient(baseURL string, timeout time.Duration, logger *slog.Logger) *HTTPArticleClient {
	return &HTTPArticleClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		logger: logger,
	}
}

// recentArticlesResponse represents the response from alt-backend
type recentArticlesResponse struct {
	Articles []articleMetadataDTO `json:"articles"`
	Since    string               `json:"since"`
	Until    string               `json:"until"`
	Count    int                  `json:"count"`
}

type articleMetadataDTO struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	PublishedAt string   `json:"published_at"`
	FeedID      string   `json:"feed_id"`
	Tags        []string `json:"tags"`
}

// GetRecentArticles fetches recent articles from alt-backend
func (c *HTTPArticleClient) GetRecentArticles(ctx context.Context, withinHours int, limit int) ([]domain.ArticleMetadata, error) {
	url := fmt.Sprintf("%s/v1/internal/articles/recent?within_hours=%d&limit=%d", c.baseURL, withinHours, limit)

	c.logger.Info("fetching recent articles from alt-backend",
		slog.String("url", url),
		slog.Int("within_hours", withinHours),
		slog.Int("limit", limit))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch recent articles: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var response recentArticlesResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	c.logger.Info("successfully fetched recent articles",
		slog.Int("count", response.Count))

	// Convert DTOs to domain entities
	articles := make([]domain.ArticleMetadata, len(response.Articles))
	for i, dto := range response.Articles {
		articleID, err := uuid.Parse(dto.ID)
		if err != nil {
			c.logger.Warn("invalid article ID, skipping", slog.String("id", dto.ID), slog.String("error", err.Error()))
			continue
		}

		feedID, err := uuid.Parse(dto.FeedID)
		if err != nil {
			// FeedID may be optional, use zero UUID
			feedID = uuid.Nil
		}

		publishedAt, err := time.Parse(time.RFC3339, dto.PublishedAt)
		if err != nil {
			c.logger.Warn("invalid published_at, using current time", slog.String("published_at", dto.PublishedAt))
			publishedAt = time.Now()
		}

		articles[i] = domain.ArticleMetadata{
			ID:          articleID,
			Title:       dto.Title,
			URL:         dto.URL,
			PublishedAt: publishedAt,
			FeedID:      feedID,
			Tags:        dto.Tags,
		}
	}

	return articles, nil
}
