package rest

import (
	"alt/di"
	"alt/domain"
	"alt/utils/errors"
	"alt/utils/html_parser"
	"alt/utils/logger"
	"alt/utils/security"
	"alt/utils/url_validator"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// HandleError converts errors to appropriate HTTP responses using enhanced error handling.
// IMPORTANT: This function ensures internal error details are NEVER exposed to clients.
// All error messages are sanitized using SafeMessage() before being returned.
func HandleError(c echo.Context, err error, operation string) error {
	// Enrich error with REST layer context
	var enrichedErr *errors.AppContextError

	// Check if it's already an AppContextError and enrich it with REST context
	if appContextErr, ok := err.(*errors.AppContextError); ok {
		enrichedErr = errors.EnrichWithContext(
			appContextErr,
			"rest",
			"RESTHandler",
			operation,
			map[string]interface{}{
				"path":        c.Request().URL.Path,
				"method":      c.Request().Method,
				"remote_addr": c.Request().RemoteAddr,
				"user_agent":  c.Request().UserAgent(),
				"request_id":  c.Response().Header().Get("X-Request-ID"),
			},
		)
	} else if appErr, ok := err.(*errors.AppError); ok {
		// Handle legacy AppError by converting to AppContextError
		enrichedErr = errors.NewAppContextError(
			string(appErr.Code),
			appErr.Message,
			"rest",
			"RESTHandler",
			operation,
			appErr.Cause,
			map[string]interface{}{
				"path":           c.Request().URL.Path,
				"method":         c.Request().Method,
				"remote_addr":    c.Request().RemoteAddr,
				"user_agent":     c.Request().UserAgent(),
				"request_id":     c.Response().Header().Get("X-Request-ID"),
				"legacy_context": appErr.Context,
			},
		)
	} else {
		// Handle unknown errors
		enrichedErr = errors.NewUnknownContextError(
			"internal server error",
			"rest",
			"RESTHandler",
			operation,
			err,
			map[string]interface{}{
				"path":        c.Request().URL.Path,
				"method":      c.Request().Method,
				"remote_addr": c.Request().RemoteAddr,
				"user_agent":  c.Request().UserAgent(),
				"request_id":  c.Response().Header().Get("X-Request-ID"),
			},
		)
	}

	// Log the full error details (internal only - never sent to client)
	ctx := c.Request().Context()
	logger.Logger.ErrorContext(ctx,
		"REST API Error",
		"error_id", enrichedErr.ErrorID,
		"error", enrichedErr.Error(),
		"code", enrichedErr.Code,
		"operation", operation,
		"path", c.Request().URL.Path,
	)

	// Return secure JSON response (SafeMessage() ensures no internal details leak)
	return c.JSON(enrichedErr.HTTPStatusCode(), enrichedErr.ToSecureHTTPResponse())
}

// handleValidationError handles validation errors
func HandleValidationError(c echo.Context, message string, field string, value interface{}) error {
	ctx := c.Request().Context()
	logger.Logger.WarnContext(ctx, "Validation error", "message", message, "field", field, "value", value)
	return c.JSON(http.StatusBadRequest, map[string]interface{}{
		"error": message,
		"field": field,
		"value": value,
		"code":  "VALIDATION_ERROR",
	})
}

// IsAllowedURL checks if the URL is allowed (not private IP).
// Deprecated: Use utils/url_validator.IsAllowedURL directly.
func IsAllowedURL(u *url.URL) error {
	return url_validator.IsAllowedURL(u)
}

// Optimize feeds response specifically for search results
// Note: Description is NOT truncated here to allow full text display in Search Feeds page
func OptimizeFeedsResponseForSearch(feeds []*domain.FeedItem) []*domain.FeedItem {
	for _, feed := range feeds {
		feed.Title = strings.TrimSpace(feed.Title)
		// Description is kept full-length for search results to support "Read more" functionality
		// Only trim whitespace, do not truncate content
		feed.Description = strings.TrimSpace(feed.Description)
	}
	return feeds
}

// Determine cache age based on limit to optimize caching strategy
func GetCacheAgeForLimit(limit int) int {
	switch {
	case limit <= 10:
		return 60 // 1 minute for small limits
	case limit <= 50:
		return 300 // 5 minutes for medium limits
	default:
		return 600 // 10 minutes for large limits
	}
}

// fetchArticleContent fetches the content of an article from the given URL
func FetchArticleContent(ctx context.Context, urlStr string, container *di.ApplicationComponents) (string, string, string, error) {
	// Validate URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid url: %w", err)
	}

	if err := IsAllowedURL(parsedURL); err != nil {
		return "", "", "", fmt.Errorf("url not allowed: %w", err)
	}

	ssrfValidator := security.NewSSRFValidator()
	if err := ssrfValidator.ValidateURL(ctx, parsedURL); err != nil {
		return "", "", "", fmt.Errorf("ssrf validation failed: %w", err)
	}

	// Create Secure HTTP client with timeout and SSRF protection (DNS rebinding prevention)
	secureClient := ssrfValidator.CreateSecureHTTPClient(10 * time.Second)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set User-Agent to avoid being blocked
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AltBot/1.0; +http://alt.com/bot)")

	// Execute request
	resp, err := secureClient.Do(req)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log but don't fail - response has been processed
			_ = closeErr
		}
	}()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	// Read body
	// Limit body size to 2MB to prevent memory issues (increased from 512KB for large sites)
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read body: %w", err)
	}

	// Zero Trust: Always extract text content from HTML
	htmlContent := string(bodyBytes)
	htmlLength := len(htmlContent)

	// Extract title from HTML using html_parser
	title := html_parser.ExtractTitle(htmlContent)

	// Extract text content from HTML (save only text, not full HTML)
	extractedText := html_parser.ExtractArticleText(htmlContent)
	extractedLength := len(extractedText)
	reductionRatio := (1.0 - float64(extractedLength)/float64(htmlLength)) * 100.0

	logger.Logger.InfoContext(ctx, "Text extraction completed in fetchArticleContent",
		"url", urlStr,
		"html_length", htmlLength,
		"extracted_length", extractedLength,
		"reduction_ratio", fmt.Sprintf("%.2f%%", reductionRatio))

	if extractedText == "" {
		// Log warning and use raw HTML if extraction fails
		logger.Logger.WarnContext(ctx, "failed to extract article text from HTML, falling back to raw HTML",
			"url", urlStr,
			"html_size_bytes", len(htmlContent))
		// Fallback to htmlContent (or a portion of it if it's too huge, but 2MB is manageable)
		return htmlContent, generateArticleID(urlStr), title, nil
	}

	// Log extraction statistics for monitoring
	originalSize := len(htmlContent)
	extractedSize := len(extractedText)
	if originalSize > 0 {
		reductionRatio := float64(extractedSize) / float64(originalSize) * 100
		logger.Logger.InfoContext(ctx, "text extraction completed",
			"url", urlStr,
			"original_size_bytes", originalSize,
			"extracted_size_bytes", extractedSize,
			"reduction_ratio_percent", reductionRatio)
	}

	// Generate ID
	articleID := generateArticleID(urlStr)

	return extractedText, articleID, title, nil
}

// Generates a simple article ID from URL
func generateArticleID(feedURL string) string {
	// Simple implementation - you might want to use a hash function or UUID
	return fmt.Sprintf("article_%s", strings.ReplaceAll(feedURL, "/", "_"))
}

// callPreProcessorSummarize calls the pre-processor summarization API
func CallPreProcessorSummarize(ctx context.Context, content string, articleID string, title string, preProcessorURL string) (string, error) {
	// Validate inputs
	if articleID == "" {
		return "", fmt.Errorf("article_id is required")
	}
	// Content can be empty if we are using the pull model (pre-processor fetches from DB)

	// Prepare request
	requestBody := map[string]string{
		"content":    content,
		"article_id": articleID,
		"title":      title,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP client with timeout
	// Extended timeout for LLM-based summarization (1000 tokens + continuation generation)
	client := &http.Client{
		Timeout: 300 * time.Second, // LLM processing with 1000 tokens can take longer
	}

	// Build API URL
	apiURL := fmt.Sprintf("%s/api/v1/summarize", preProcessorURL)

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call pre-processor: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log but don't fail - response has been processed
			_ = closeErr
		}
	}()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("pre-processor returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse response
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

// streamPreProcessorSummarize streams the pre-processor summarization API
func StreamPreProcessorSummarize(ctx context.Context, content string, articleID string, title string, preProcessorURL string) (io.ReadCloser, error) {
	// Validate inputs
	if articleID == "" {
		return nil, fmt.Errorf("article_id is required")
	}

	// Prepare request
	requestBody := map[string]string{
		"content":    content,
		"article_id": articleID,
		"title":      title,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP client with longer timeout/no timeout for streaming
	client := &http.Client{
		// No strict timeout for streaming, or very long one.
		// Context cancellation should handle cleanup.
		Timeout: 0,
	}

	// Build API URL
	apiURL := fmt.Sprintf("%s/api/v1/summarize/stream", preProcessorURL)

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call pre-processor stream: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		// Read error response body for better error reporting
		bodyBytes, readErr := io.ReadAll(resp.Body)
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log but don't fail - error response has been read
			_ = closeErr
		}

		errorBody := string(bodyBytes)
		if readErr != nil {
			errorBody = fmt.Sprintf("(failed to read error body: %v)", readErr)
		}

		logger.Logger.ErrorContext(ctx, "Pre-processor stream returned non-200 status",
			"status_code", resp.StatusCode,
			"status", resp.Status,
			"error_body", errorBody,
			"article_id", articleID)
		return nil, fmt.Errorf("pre-processor stream returned status %d: %s", resp.StatusCode, errorBody)
	}

	logger.Logger.InfoContext(ctx, "Pre-processor stream response received successfully",
		"article_id", articleID,
		"status", resp.Status,
		"content_type", resp.Header.Get("Content-Type"))

	return resp.Body, nil
}

// SummarizeStatus represents the status of a summarization job
type SummarizeStatus struct {
	JobID        string
	Status       string
	Summary      string
	ErrorMessage string
	ArticleID    string
}

// callPreProcessorSummarizeQueue calls the pre-processor queue endpoint
func CallPreProcessorSummarizeQueue(ctx context.Context, articleID string, title string, preProcessorURL string) (string, error) {
	// Validate inputs
	if articleID == "" {
		return "", fmt.Errorf("article_id is required")
	}

	// Prepare request
	requestBody := map[string]string{
		"article_id": articleID,
		"title":      title,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second, // Short timeout for queue endpoint
	}

	// Build API URL
	apiURL := fmt.Sprintf("%s/api/v1/summarize/queue", preProcessorURL)

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call pre-processor: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log but don't fail - response has been processed
			_ = closeErr
		}
	}()

	// Check status code
	if resp.StatusCode != http.StatusAccepted {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("pre-processor returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse response
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

// callPreProcessorSummarizeStatus calls the pre-processor status endpoint
func CallPreProcessorSummarizeStatus(ctx context.Context, jobID string, preProcessorURL string) (*SummarizeStatus, error) {
	// Validate inputs
	if jobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second, // Short timeout for status check
	}

	// Build API URL
	apiURL := fmt.Sprintf("%s/api/v1/summarize/status/%s", preProcessorURL, jobID)

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call pre-processor: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log but don't fail - response has been processed
			_ = closeErr
		}
	}()

	// Check status code
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // Job not found
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("pre-processor returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse response
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

// cleanSummaryContent removes markdown code blocks, repetitive patterns, and other anomalies from summary content
func CleanSummaryContent(summary string) string {
	if summary == "" {
		return ""
	}

	cleaned := summary

	// Remove markdown code blocks (```...```)
	codeBlockRegex := regexp.MustCompile("(?s)```[^`]*```")
	cleaned = codeBlockRegex.ReplaceAllString(cleaned, "")
	// Remove standalone triple backticks
	backtickRegex := regexp.MustCompile("```+")
	cleaned = backtickRegex.ReplaceAllString(cleaned, "")
	// Remove any remaining backticks
	cleaned = strings.ReplaceAll(cleaned, "`", "")

	// Remove excessive whitespace
	whitespaceRegex := regexp.MustCompile(`[ \t]+`)
	cleaned = whitespaceRegex.ReplaceAllString(cleaned, " ")
	// Remove excessive newlines
	newlineRegex := regexp.MustCompile(`\n{3,}`)
	cleaned = newlineRegex.ReplaceAllString(cleaned, "\n\n")

	// Trim whitespace
	cleaned = strings.TrimSpace(cleaned)

	return strings.TrimSpace(cleaned)
}
