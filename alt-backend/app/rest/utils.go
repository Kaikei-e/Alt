package rest

import (
	"alt/di"
	"alt/domain"
	"alt/utils/errors"
	"alt/utils/logger"
	"alt/utils/security"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// handleError converts errors to appropriate HTTP responses using enhanced error handling
func handleError(c echo.Context, err error, operation string) error {
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

	// Log the error
	logger.Logger.Error(
		"REST API Error",
		"error", enrichedErr.Error(),
		"code", enrichedErr.Code,
		"operation", operation,
		"path", c.Request().URL.Path,
	)

	// Return JSON response
	return c.JSON(enrichedErr.HTTPStatusCode(), map[string]interface{}{
		"error": map[string]interface{}{
			"code":    enrichedErr.Code,
			"message": enrichedErr.Message,
		},
	})
}

// handleValidationError handles validation errors
func handleValidationError(c echo.Context, message string, field string, value interface{}) error {
	logger.Logger.Warn("Validation error", "message", message, "field", field, "value", value)
	return c.JSON(http.StatusBadRequest, map[string]interface{}{
		"error": message,
		"field": field,
		"value": value,
		"code":  "VALIDATION_ERROR",
	})
}

// isAllowedURL checks if the URL is allowed (not private IP)
func isAllowedURL(u *url.URL) error {
	// Allow http and https
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme not allowed: %s", u.Scheme)
	}

	// Resolve IP
	ips, err := net.LookupIP(u.Hostname())
	if err != nil {
		return fmt.Errorf("could not resolve hostname: %w", err)
	}

	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() {
			return fmt.Errorf("private IP not allowed: %s", ip.String())
		}
	}

	return nil
}

// Optimize feeds response specifically for search results (more aggressive truncation)
func optimizeFeedsResponseForSearch(feeds []*domain.FeedItem) []*domain.FeedItem {
	for _, feed := range feeds {
		feed.Title = strings.TrimSpace(feed.Title)
		// Aggressive truncation for search results
		if len(feed.Description) > 200 {
			feed.Description = feed.Description[:200] + "..."
		}
	}
	return feeds
}

// Determine cache age based on limit to optimize caching strategy
func getCacheAgeForLimit(limit int) int {
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
func fetchArticleContent(ctx context.Context, urlStr string, container *di.ApplicationComponents) (string, string, string, error) {
	// Validate URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid url: %w", err)
	}

	if err := isAllowedURL(parsedURL); err != nil {
		return "", "", "", fmt.Errorf("url not allowed: %w", err)
	}

	ssrfValidator := security.NewSSRFValidator()
	if err := ssrfValidator.ValidateURL(ctx, parsedURL); err != nil {
		return "", "", "", fmt.Errorf("ssrf validation failed: %w", err)
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set User-Agent to avoid being blocked
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AltBot/1.0; +http://alt.com/bot)")

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	// Read body
	// Limit body size to 1MB to prevent memory issues
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read body: %w", err)
	}

	// Simple title extraction (regex)
	// In a real app, use a proper HTML parser like goquery
	title := ""
	// ... (simplified title extraction logic if needed, or just return empty title)

	// Generate ID
	articleID := generateArticleID(urlStr)

	return string(bodyBytes), articleID, title, nil
}

// Generates a simple article ID from URL
func generateArticleID(feedURL string) string {
	// Simple implementation - you might want to use a hash function or UUID
	return fmt.Sprintf("article_%s", strings.ReplaceAll(feedURL, "/", "_"))
}

// callPreProcessorSummarize calls the pre-processor summarization API
func callPreProcessorSummarize(ctx context.Context, content string, articleID string, title string, preProcessorURL string) (string, error) {
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
	// Extended timeout for LLM-based summarization (news-creator processing time)
	client := &http.Client{
		Timeout: 180 * time.Second, // LLM processing can take 60-120 seconds
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
	defer resp.Body.Close()

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
