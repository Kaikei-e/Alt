package driver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"unicode/utf8"

	"pre-processor/config"
	"pre-processor/domain"
	"pre-processor/utils"
	"pre-processor/utils/html_parser"
)

// isTransportBusyError reports whether the error from http.Client.Do is likely
// caused by the upstream pipeline being busy processing an in-flight request
// for the same article (response header timeout, context deadline, net dial
// errors), as opposed to a permanent failure.
func isTransportBusyError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	// net/http emits this wrapped string when ResponseHeaderTimeout fires.
	return strings.Contains(err.Error(), "timeout awaiting response headers")
}

type SummarizedContent struct {
	ArticleID       string `json:"article_id"`
	SummaryJapanese string `json:"summary_japanese"`
}

// SummarizeRequest represents the request to news-creator /api/v1/summarize endpoint
type SummarizeRequest struct {
	ArticleID string `json:"article_id"`
	Content   string `json:"content"`
	Stream    bool   `json:"stream"`
	Priority  string `json:"priority,omitempty"`
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

// minContentRunes / maxContentRunes bound the extracted article content sent
// to news-creator. Both bounds use rune (character) counts — not byte counts
// — for consistency with Python's len() on the receiving side, since a
// Japanese character is 3 bytes in UTF-8 and a byte-based max would reject
// real Japanese articles at roughly a third of the intended budget.
const (
	minContentRunes = 100
	// 100K runes = ~25K tokens (Japanese) = fits within 32K context with margin.
	// RTX 4060 (8GB VRAM) can process this stably without timeouts.
	maxContentRunes = 100_000
)

// prepareSummarizeContent extracts plain text from raw article HTML (Zero
// Trust: news-creator must never receive raw HTML, even if it was already
// extracted upstream) and validates its length. logContext names the calling
// path ("sending to news-creator" / "streaming summary") for log messages.
// Shared by the blocking and streaming summarizer clients below.
func prepareSummarizeContent(ctx context.Context, article *domain.Article, cfg *config.Config, logger *slog.Logger, logContext string) (string, error) {
	originalLength := len(article.Content)
	logger.InfoContext(ctx, "extracting text from content before "+logContext+" (Zero Trust validation)",
		"article_id", article.ID,
		"original_length", originalLength)

	extractedContent := html_parser.ExtractArticleText(article.Content)

	if extractedContent == "" {
		logger.WarnContext(ctx, "text extraction returned empty, using original content",
			"article_id", article.ID,
			"original_length", originalLength)
		extractedContent = article.Content
	} else {
		extractedLength := len(extractedContent)
		reductionRatio := (1.0 - float64(extractedLength)/float64(originalLength)) * 100.0
		if reductionRatio < 0 {
			logger.InfoContext(ctx, "text extraction did not reduce content (content may already be plain text)",
				"article_id", article.ID,
				"original_length", originalLength,
				"extracted_length", extractedLength,
				"size_increase_ratio", fmt.Sprintf("%.2f%%", -reductionRatio))
		} else {
			logger.InfoContext(ctx, "text extraction completed before "+logContext,
				"article_id", article.ID,
				"original_length", originalLength,
				"extracted_length", extractedLength,
				"reduction_ratio", fmt.Sprintf("%.2f%%", reductionRatio))
		}
	}

	runeCount := utf8.RuneCountInString(extractedContent)
	if runeCount < minContentRunes {
		// Fallback: if content is too short but title is long enough, use title as content
		titleRuneCount := utf8.RuneCountInString(article.Title)
		if titleRuneCount >= minContentRunes {
			logger.InfoContext(ctx, "Content too short, falling back to title-based summarization",
				"article_id", article.ID,
				"content_rune_count", runeCount,
				"title_rune_count", titleRuneCount)
			return article.Title, nil
		}
		logger.InfoContext(ctx, "Skipping summarization: content too short after extraction",
			"article_id", article.ID,
			"original_length", originalLength,
			"extracted_length_chars", runeCount,
			"min_required", minContentRunes)
		return "", ErrContentTooShort
	}
	if runeCount > maxContentRunes {
		logger.InfoContext(ctx, "Skipping summarization: content too long after extraction",
			"article_id", article.ID,
			"original_length", originalLength,
			"extracted_length_chars", runeCount,
			"max_allowed", maxContentRunes)
		return "", ErrContentTooLong
	}

	return extractedContent, nil
}

// isContentTooShortResponse reports whether a news-creator 400 body indicates
// content-length validation rather than a generic request bug.
func isContentTooShortResponse(body string) bool {
	lower := strings.ToLower(body)
	return strings.Contains(lower, "too short") || strings.Contains(lower, "content_too_short")
}

// classifyBusyOrErrorStatus maps a non-200 news-creator response to a
// sentinel domain error for the codes that mean "upstream busy, retry
// later" (429/422/502/503/504), or nil when the caller must classify the
// status itself (e.g. 400, or the final fallback error). Shared by the
// blocking and streaming summarizer clients below.
func classifyBusyOrErrorStatus(resp *http.Response, body string, article *domain.Article, logger *slog.Logger, streaming bool) error {
	suffix := ""
	if streaming {
		suffix = " (streaming)"
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		retryAfter := resp.Header.Get("Retry-After")
		logger.Warn("news-creator queue full"+suffix+", backing off",
			"article_id", article.ID, "retry_after", retryAfter)
		return domain.ErrServiceOverloaded
	case http.StatusUnprocessableEntity:
		logger.Warn("news-creator returned 422"+suffix+": content not processable by model",
			"article_id", article.ID, "body", body)
		return domain.ErrContentNotProcessable
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		logger.Warn("news-creator returned busy status"+suffix+", treating as upstream busy",
			"article_id", article.ID, "status_code", resp.StatusCode)
		return fmt.Errorf("upstream busy (status %d): %w", resp.StatusCode, domain.ErrUpstreamBusy)
	default:
		return nil
	}
}

func ArticleSummarizerAPIClient(ctx context.Context, article *domain.Article, cfg *config.Config, logger *slog.Logger, priority string) (*SummarizedContent, error) {
	extractedContent, err := prepareSummarizeContent(ctx, article, cfg, logger, "sending to news-creator")
	if err != nil {
		return nil, err
	}
	extractedLength := len(extractedContent)

	// Construct API URL from config
	apiURL := cfg.NewsCreator.Host + cfg.NewsCreator.APIPath

	// CLAUDE.md critical rule: enforce the configured minimum interval between
	// outbound calls to the same host. Uses the process-wide limiter so it
	// covers both the blocking and streaming paths.
	if err := utils.DefaultHostRateLimiter(cfg.RateLimit.DefaultInterval).Wait(ctx, cfg.NewsCreator.Host); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	// Prepare request payload for /api/v1/summarize endpoint
	// Use extracted content, not original
	payload := SummarizeRequest{
		ArticleID: article.ID,
		Content:   extractedContent,
		Priority:  priority,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to marshal payload", "error", err)
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
		logger.ErrorContext(ctx, "Failed to create request", "error", err, "api_url", apiURL)
		return nil, err
	}

	logger.InfoContext(ctx, "Making request to news-creator API",
		"api_url", apiURL,
		"article_id", article.ID,
		"content_length", extractedLength,
		"timeout", cfg.NewsCreator.Timeout)

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to send request", "error", err, "api_url", apiURL)
		if isTransportBusyError(err) {
			return nil, fmt.Errorf("send request: %w", errors.Join(domain.ErrUpstreamBusy, err))
		}
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.ErrorContext(ctx, "failed to close response body", "error", err)
		}
	}()

	// Check HTTP status code
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		bodyStr := string(bodyBytes)
		logger.ErrorContext(ctx, "API returned non-200 status", "status", resp.Status, "code", resp.StatusCode, "body", bodyStr)

		if err := classifyBusyOrErrorStatus(resp, bodyStr, article, logger, false); err != nil {
			return nil, err
		}

		if resp.StatusCode == http.StatusBadRequest {
			if isContentTooShortResponse(bodyStr) {
				logger.InfoContext(ctx, "Mapping 400 Bad Request to ErrContentTooShort", "article_id", article.ID)
				return nil, ErrContentTooShort
			}
			return nil, fmt.Errorf("API request failed with status: %s, body: %s: %w", resp.Status, bodyStr, domain.ErrInvalidRequest)
		}

		return nil, fmt.Errorf("API request failed with status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to read response body", "error", err)
		return nil, err
	}

	logger.InfoContext(ctx, "Response received", "status", resp.Status)
	logger.DebugContext(ctx, "Response body", "body", string(body))

	// Parse the news-creator /api/v1/summarize response
	var apiResponse SummarizeResponse

	err = json.Unmarshal(body, &apiResponse)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to unmarshal API response", "error", err)
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	// Check if summarization was successful
	if !apiResponse.Success {
		logger.ErrorContext(ctx, "Summarization failed", "article_id", article.ID)
		return nil, fmt.Errorf("news-creator returned success=false for article %s", article.ID)
	}

	// Summary is already cleaned by news-creator, no need for additional cleaning
	summarizedContent := &SummarizedContent{
		ArticleID:       apiResponse.ArticleID,
		SummaryJapanese: apiResponse.Summary,
	}

	logger.InfoContext(ctx, "Summary generated successfully",
		"article_id", article.ID,
		"summary_length", len(apiResponse.Summary),
		"model", apiResponse.Model)

	return summarizedContent, nil
}

// StreamArticleSummarizerAPIClient streams the summary generation from news-creator
func StreamArticleSummarizerAPIClient(ctx context.Context, article *domain.Article, cfg *config.Config, logger *slog.Logger, priority string) (io.ReadCloser, error) {
	extractedContent, err := prepareSummarizeContent(ctx, article, cfg, logger, "streaming summary")
	if err != nil {
		return nil, err
	}
	extractedLength := len(extractedContent)

	apiURL := cfg.NewsCreator.Host + cfg.NewsCreator.APIPath

	// Same 5-second floor as the blocking path above.
	if err := utils.DefaultHostRateLimiter(cfg.RateLimit.DefaultInterval).Wait(ctx, cfg.NewsCreator.Host); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	payload := SummarizeRequest{
		ArticleID: article.ID,
		Content:   extractedContent,
		Stream:    true,
		Priority:  priority,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to marshal payload for streaming", "error", err, "article_id", article.ID)
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	clientManager := utils.NewHTTPClientManager()
	// For streaming, we need a client without timeout or with a very long timeout
	// Create a custom client for streaming that doesn't timeout
	streamClient := &http.Client{
		Timeout:   0, // No timeout for streaming
		Transport: clientManager.GetSummaryClient().Transport,
	}

	logger.InfoContext(ctx, "Making streaming request to news-creator API",
		"api_url", apiURL,
		"article_id", article.ID,
		"content_length", extractedLength,
		"payload_size", len(jsonData))

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(string(jsonData)))
	if err != nil {
		logger.ErrorContext(ctx, "Failed to create streaming request", "error", err, "api_url", apiURL, "article_id", article.ID)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := streamClient.Do(req)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to send streaming request", "error", err, "api_url", apiURL, "article_id", article.ID)
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Read error response body for better error reporting
		bodyBytes, readErr := io.ReadAll(resp.Body)
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.WarnContext(ctx, "failed to close streaming response body", "error", closeErr, "article_id", article.ID)
		}

		errorBody := string(bodyBytes)
		if readErr != nil {
			errorBody = fmt.Sprintf("(failed to read error body: %v)", readErr)
		}

		if err := classifyBusyOrErrorStatus(resp, errorBody, article, logger, true); err != nil {
			return nil, err
		}

		logger.ErrorContext(ctx, "API returned non-200 status for streaming request",
			"status", resp.Status,
			"code", resp.StatusCode,
			"body", errorBody,
			"article_id", article.ID,
			"api_url", apiURL)
		return nil, fmt.Errorf("API request failed with status: %s, body: %s", resp.Status, errorBody)
	}

	logger.InfoContext(ctx, "Streaming response received successfully",
		"article_id", article.ID,
		"status", resp.Status,
		"content_type", resp.Header.Get("Content-Type"))

	return resp.Body, nil
}
