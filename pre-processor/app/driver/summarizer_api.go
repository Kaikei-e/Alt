package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"pre-processor/config"
	"pre-processor/domain"
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
	Stream    bool   `json:"stream"`
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

// ErrContentTooShort is an alias for domain.ErrContentTooShort for backward compatibility
// Deprecated: Use domain.ErrContentTooShort directly
var ErrContentTooShort = domain.ErrContentTooShort

// ErrContentTooLong is an alias for domain.ErrContentTooLong for backward compatibility
// Deprecated: Use domain.ErrContentTooLong directly
var ErrContentTooLong = domain.ErrContentTooLong

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
	// 100KB = ~25K tokens (Japanese) = fits within 32K context with margin
	// RTX 4060 (8GB VRAM) can process this stably without timeouts
	const maxContentLength = 100_000
	if extractedLength < minContentLength {
		logger.Info("Skipping summarization: content too short after extraction",
			"article_id", article.ID,
			"original_length", originalLength,
			"extracted_length", extractedLength,
			"min_required", minContentLength)
		return nil, ErrContentTooShort
	}
	if extractedLength > maxContentLength {
		logger.Info("Skipping summarization: content too long after extraction",
			"article_id", article.ID,
			"original_length", originalLength,
			"extracted_length", extractedLength,
			"max_allowed", maxContentLength)
		return nil, ErrContentTooLong
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
		bodyStr := string(bodyBytes)
		logger.Error("API returned non-200 status", "status", resp.Status, "code", resp.StatusCode, "body", bodyStr)

		// Handle 400 Bad Request as ErrContentTooShort if likely
		if resp.StatusCode == http.StatusBadRequest {
			// Simply assume 400 means content validation failed (likely too short or invalid)
			// This allows the service to handle it gracefully (save placeholder summary)
			logger.Info("Mapping 400 Bad Request to ErrContentTooShort", "article_id", article.ID)
			return nil, ErrContentTooShort
		}

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

// StreamArticleSummarizerAPIClient streams the summary generation from news-creator
func StreamArticleSummarizerAPIClient(ctx context.Context, article *models.Article, cfg *config.Config, logger *slog.Logger) (io.ReadCloser, error) {
	// Zero Trust: Always extract text from content before sending to news-creator
	originalLength := len(article.Content)
	logger.Info("extracting text from content before streaming summary (Zero Trust validation)",
		"article_id", article.ID,
		"original_length", originalLength)

	extractedContent := html_parser.ExtractArticleText(article.Content)
	extractedLength := len(extractedContent)

	if extractedContent == "" {
		logger.Warn("text extraction returned empty, using original content",
			"article_id", article.ID)
		extractedContent = article.Content
		extractedLength = originalLength
	} else {
		reductionRatio := (1.0 - float64(extractedLength)/float64(originalLength)) * 100.0
		logger.Info("text extraction completed before streaming API call",
			"article_id", article.ID,
			"original_length", originalLength,
			"extracted_length", extractedLength,
			"reduction_ratio", fmt.Sprintf("%.2f%%", reductionRatio))
	}

	const minContentLength = 100
	// 100KB = ~25K tokens (Japanese) = fits within 32K context with margin
	// RTX 4060 (8GB VRAM) can process this stably without timeouts
	const maxContentLength = 100_000
	if extractedLength < minContentLength {
		logger.Info("Skipping summarization: content too short after extraction",
			"article_id", article.ID,
			"original_length", originalLength,
			"extracted_length", extractedLength,
			"min_required", minContentLength)
		return nil, ErrContentTooShort
	}
	if extractedLength > maxContentLength {
		logger.Info("Skipping summarization: content too long after extraction",
			"article_id", article.ID,
			"original_length", originalLength,
			"extracted_length", extractedLength,
			"max_allowed", maxContentLength)
		return nil, ErrContentTooLong
	}

	apiURL := cfg.NewsCreator.Host + cfg.NewsCreator.APIPath
	payload := SummarizeRequest{
		ArticleID: article.ID,
		Content:   extractedContent,
		Stream:    true,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		logger.Error("Failed to marshal payload for streaming", "error", err, "article_id", article.ID)
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	clientManager := utils.NewHTTPClientManager()
	// For streaming, we need a client without timeout or with a very long timeout
	// Create a custom client for streaming that doesn't timeout
	streamClient := &http.Client{
		Timeout:   0, // No timeout for streaming
		Transport: clientManager.GetSummaryClient().Transport,
	}

	logger.Info("Making streaming request to news-creator API",
		"api_url", apiURL,
		"article_id", article.ID,
		"content_length", extractedLength,
		"payload_size", len(jsonData))

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(string(jsonData)))
	if err != nil {
		logger.Error("Failed to create streaming request", "error", err, "api_url", apiURL, "article_id", article.ID)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := streamClient.Do(req)
	if err != nil {
		logger.Error("Failed to send streaming request", "error", err, "api_url", apiURL, "article_id", article.ID)
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Read error response body for better error reporting
		bodyBytes, readErr := io.ReadAll(resp.Body)
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.Warn("failed to close streaming response body", "error", closeErr, "article_id", article.ID)
		}

		errorBody := string(bodyBytes)
		if readErr != nil {
			errorBody = fmt.Sprintf("(failed to read error body: %v)", readErr)
		}

		logger.Error("API returned non-200 status for streaming request",
			"status", resp.Status,
			"code", resp.StatusCode,
			"body", errorBody,
			"article_id", article.ID,
			"api_url", apiURL)
		return nil, fmt.Errorf("API request failed with status: %s, body: %s", resp.Status, errorBody)
	}

	logger.Info("Streaming response received successfully",
		"article_id", article.ID,
		"status", resp.Status,
		"content_type", resp.Header.Get("Content-Type"))

	return resp.Body, nil
}
