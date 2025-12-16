package driver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"pre-processor/config"
	"pre-processor/models"
	"pre-processor/utils"
	"pre-processor/utils/html_parser"
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
	htmlTagRegex = regexp.MustCompile(`<[^>]+>`)
	// ErrContentTooShort is returned when article content is too short for summarization
	ErrContentTooShort = errors.New("article content too short for summarization (less than 100 characters)")
)

// estimateContentLength estimates the content length after HTML tag removal
func estimateContentLength(content string) int {
	if content == "" {
		return 0
	}
	// Remove HTML tags for estimation
	cleaned := htmlTagRegex.ReplaceAllString(content, "")
	cleaned = strings.TrimSpace(cleaned)
	return len(cleaned)
}

func ArticleSummarizerAPIClient(ctx context.Context, article *models.Article, cfg *config.Config, logger *slog.Logger) (*SummarizedContent, error) {
	// Zero Trust: Always extract text from content before sending to news-creator
	// This ensures we never send raw HTML, even if it was already extracted upstream
	originalLength := len(article.Content)
	logger.Info("extracting text from content before sending to news-creator (Zero Trust validation)",
		"article_id", article.ID,
		"original_length", originalLength)

	extractedContent := html_parser.ExtractArticleText(article.Content)
	extractedLength := len(extractedContent)

	if extractedContent == "" {
		logger.Warn("text extraction returned empty, using original content",
			"article_id", article.ID,
			"original_length", originalLength)
		extractedContent = article.Content
		extractedLength = originalLength
	} else {
		reductionRatio := (1.0 - float64(extractedLength)/float64(originalLength)) * 100.0
		logger.Info("text extraction completed before API call",
			"article_id", article.ID,
			"original_length", originalLength,
			"extracted_length", extractedLength,
			"reduction_ratio", fmt.Sprintf("%.2f%%", reductionRatio))
	}

	// Check content length after extraction
	const minContentLength = 100
	if extractedLength < minContentLength {
		logger.Info("Skipping summarization: content too short after extraction",
			"article_id", article.ID,
			"original_length", originalLength,
			"extracted_length", extractedLength,
			"min_required", minContentLength)
		return nil, ErrContentTooShort
	}

	// Construct API URL from config
	apiURL := cfg.NewsCreator.Host + cfg.NewsCreator.APIPath

	// Prepare request payload for /api/v1/summarize endpoint
	// Use extracted content, not original
	payload := SummarizeRequest{
		ArticleID: article.ID,
		Content:   extractedContent,
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

	logger.Info("Making request to news-creator API",
		"api_url", apiURL,
		"article_id", article.ID,
		"content_length", extractedLength,
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
