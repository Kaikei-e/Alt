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

type payloadModel struct {
	Model       string       `json:"model"`
	Prompt      string       `json:"prompt"`
	Options     optionsModel `json:"options"`
	KeepAlive   int          `json:"keep_alive"`
	MaxWaitTime int          `json:"max_wait_time"`
	Stream      bool         `json:"stream"`
}

type optionsModel struct {
	Stop          []string `json:"stop"`
	Temperature   float64  `json:"temperature"`
	TopP          float64  `json:"top_p"`
	NumPredict    int      `json:"num_predict"`
	RepeatPenalty float64  `json:"repeat_penalty"`
	NumCtx        int      `json:"num_ctx"`
}

type OllamaResponse struct {
	Model      string `json:"model"`
	Response   string `json:"response"`
	DoneReason string `json:"done_reason"`
	Done       bool   `json:"done"`
}

const (
	// Refined prompt template optimized for gemma3:4b.
	promptTemplate = `<start_of_turn>user
You are an expert multilingual journalist specializing in Japanese news summarization. Your task is to analyze English articles and create comprehensive Japanese summaries that capture the essence while being culturally appropriate for Japanese audiences.

GUIDELINES:
- Output in natural, professional Japanese (新聞記事スタイル)
- Maximum 1500 Japanese characters
- Focus on key facts, context, and implications
- Maintain journalistic objectivity
- Use appropriate honorifics and formal language where needed

ARTICLE TO SUMMARIZE:
---
%s
---

Create a flowing Japanese summary that reads naturally to native speakers. Begin directly with the summary without any preamble.
<end_of_turn>
<start_of_turn>model
`
)

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
	prompt := fmt.Sprintf(promptTemplate, article.Content)

	// Construct API URL from config
	apiURL := cfg.NewsCreator.Host + cfg.NewsCreator.APIPath

	payload := payloadModel{
		Model:     cfg.NewsCreator.Model,
		Prompt:    prompt,
		Stream:    false,
		KeepAlive: -1,
		Options: optionsModel{
			Temperature:   0.0,
			TopP:          0.9,
			NumPredict:    500,
			RepeatPenalty: 1.00,
			NumCtx:        8192,
			Stop:          []string{"<|user|>", "<|system|>"},
		},
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
		"model", cfg.NewsCreator.Model,
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

	// Parse the Ollama API response
	var apiResponse OllamaResponse

	err = json.Unmarshal(body, &apiResponse)
	if err != nil {
		logger.Error("Failed to unmarshal API response", "error", err)
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	// Check if response is complete
	if !apiResponse.Done {
		logger.Warn("Received incomplete response from API")
	}

	cleanedSummary := cleanSummarizedContent(apiResponse.Response)

	summarizedContent := &SummarizedContent{
		ArticleID:       article.ID,
		SummaryJapanese: cleanedSummary,
	}

	logger.Info("Summary generated successfully",
		"article_id", article.ID,
		"summary_length", len(cleanedSummary))

	return summarizedContent, nil
}

func cleanSummarizedContent(content string) string {
	// Remove any system tags that might leak through
	content = strings.ReplaceAll(content, "<|system|>", "")
	content = strings.ReplaceAll(content, "<|user|>", "")
	content = strings.ReplaceAll(content, "<|assistant|>", "")

	// Remove potential thinking tags
	if startIdx := strings.Index(content, "<think>"); startIdx != -1 {
		if endIdx := strings.Index(content, "</think>"); endIdx != -1 {
			content = content[:startIdx] + content[endIdx+8:]
		}
	}

	// Basic cleanup: trim whitespace and remove empty lines
	lines := strings.Split(content, "\n")

	var cleanLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" &&
			!strings.HasPrefix(trimmed, "---") &&
			!strings.HasPrefix(trimmed, "**") &&
			!strings.Contains(trimmed, "Summary:") &&
			!strings.Contains(trimmed, "要約:") {
			cleanLines = append(cleanLines, trimmed)
		}
	}

	// Join lines with space and return final cleaned content
	result := strings.Join(cleanLines, " ")

	return strings.TrimSpace(result)
}
