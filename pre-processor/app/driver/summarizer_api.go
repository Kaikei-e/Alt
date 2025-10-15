package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"pre-processor/config"
	"pre-processor/models"
	"pre-processor/utils"
)

type SummarizedContent struct {
	ArticleID       string `json:"article_id"`
	SummaryJapanese string `json:"summary_japanese"`
}

// SummarizeRequest represents the request to news-creator /api/v1/summarize endpoint
type SummarizeRequest struct {
	ArticleID string `json:"article_id"`
	Content   string `json:"content"`
}

// SummarizeResponse represents the response from news-creator /api/v1/summarize endpoint
type SummarizeResponse struct {
	Success          bool     `json:"success"`
	ArticleID        string   `json:"article_id"`
	Summary          string   `json:"summary"`
	Model            string   `json:"model"`
	PromptTokens     *int     `json:"prompt_tokens,omitempty"`
	CompletionTokens *int     `json:"completion_tokens,omitempty"`
	TotalDurationMs  *float64 `json:"total_duration_ms,omitempty"`
}

var (
	// Global rate-limited HTTP client with circuit breaker
	httpClient *utils.RateLimitedHTTPClient
	clientOnce sync.Once
)

// getHTTPClient returns a singleton rate-limited HTTP client
func getHTTPClient() *utils.RateLimitedHTTPClient {
	clientOnce.Do(func() {
		// Initialize with 5-second rate limit, 3 retries, 30-second timeout
		// Circuit breaker: 3 failures threshold, 10-second timeout
		httpClient = utils.NewRateLimitedHTTPClientWithCircuitBreaker(
			5*time.Second,  // Minimum 5-second interval between requests
			3,              // Max retries
			30*time.Second, // Request timeout
			3,              // Circuit breaker failure threshold
			10*time.Second, // Circuit breaker timeout
		)
	})
	return httpClient
}

func ArticleSummarizerAPIClient(ctx context.Context, article *models.Article, cfg *config.Config, logger *slog.Logger) (*SummarizedContent, error) {
	// Construct API URL from config
	apiURL := cfg.NewsCreator.Host + cfg.NewsCreator.APIPath

	// Prepare request payload for /api/v1/summarize endpoint
	payload := SummarizeRequest{
		ArticleID: article.ID,
		Content:   article.Content,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		logger.Error("Failed to marshal payload", "error", err)
		return nil, err
	}

	// Use singleton HTTP client manager for better performance
	clientManager := utils.NewHTTPClientManager()
	client := clientManager.GetSummaryClient()

	// Create context with timeout from config
	ctxWithTimeout, cancel := context.WithTimeout(ctx, cfg.NewsCreator.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctxWithTimeout, "POST", apiURL, strings.NewReader(string(jsonData)))
	if err != nil {
		logger.Error("Failed to create request", "error", err, "api_url", apiURL)
		return nil, err
	}

	logger.Debug("Making request to news-creator API",
		"api_url", apiURL,
		"article_id", article.ID,
		"timeout", cfg.NewsCreator.Timeout)

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		logger.Error("Failed to send request", "error", err, "api_url", apiURL)
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Error("failed to close response body", "error", err)
		}
	}()

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		logger.Error("API returned non-200 status", "status", resp.Status, "code", resp.StatusCode, "body", string(bodyBytes))
		return nil, fmt.Errorf("API request failed with status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("Failed to read response body", "error", err)
		return nil, err
	}

	logger.Info("Response received", "status", resp.Status)
	logger.Debug("Response body", "body", string(body))

	// Parse the news-creator /api/v1/summarize response
	var apiResponse SummarizeResponse

	err = json.Unmarshal(body, &apiResponse)
	if err != nil {
		logger.Error("Failed to unmarshal API response", "error", err)
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	// Check if summarization was successful
	if !apiResponse.Success {
		logger.Error("Summarization failed", "article_id", article.ID)
		return nil, fmt.Errorf("news-creator returned success=false for article %s", article.ID)
	}

	// Summary is already cleaned by news-creator, no need for additional cleaning
	summarizedContent := &SummarizedContent{
		ArticleID:       apiResponse.ArticleID,
		SummaryJapanese: apiResponse.Summary,
	}

	logger.Info("Summary generated successfully",
		"article_id", article.ID,
		"summary_length", len(apiResponse.Summary),
		"model", apiResponse.Model)

	return summarizedContent, nil
}
