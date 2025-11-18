package rest

import (
	"alt/di"
	"alt/domain"
	"alt/usecase/archive_article_usecase"
	"alt/utils/errors"
	"alt/utils/logger"
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
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

	// Log the enriched error with context
	logger.Logger.Error("REST handler error",
		"error", enrichedErr.Error(),
		"error_code", enrichedErr.Code,
		"layer", enrichedErr.Layer,
		"component", enrichedErr.Component,
		"operation", enrichedErr.Operation,
		"path", c.Request().URL.Path,
		"method", c.Request().Method,
		"is_retryable", enrichedErr.IsRetryable(),
	)

	return c.JSON(enrichedErr.HTTPStatusCode(), enrichedErr.ToHTTPResponse())
}

// handleValidationError creates a validation error response with enhanced context
func handleValidationError(c echo.Context, message string, field string, value interface{}) error {
	validationErr := errors.NewValidationContextError(
		message,
		"rest",
		"RESTHandler",
		"validateInput",
		map[string]interface{}{
			"field":       field,
			"value":       value,
			"path":        c.Request().URL.Path,
			"method":      c.Request().Method,
			"remote_addr": c.Request().RemoteAddr,
			"user_agent":  c.Request().UserAgent(),
			"request_id":  c.Response().Header().Get("X-Request-ID"),
		},
	)

	logger.Logger.Error("REST validation error",
		"error", validationErr.Error(),
		"field", field,
		"value", value,
		"path", c.Request().URL.Path,
	)
	return c.JSON(validationErr.HTTPStatusCode(), validationErr.ToHTTPResponse())
}

func isAllowedURL(u *url.URL) error {
	// Allow both HTTP and HTTPS
	if u.Scheme != "https" && u.Scheme != "http" {
		return stderrors.New("only HTTP and HTTPS schemes allowed")
	}

	// Block private networks
	if isPrivateIP(u.Hostname()) {
		return stderrors.New("access to private networks not allowed")
	}

	// Block localhost variations
	hostname := strings.ToLower(u.Hostname())
	if hostname == "localhost" || hostname == "127.0.0.1" || strings.HasPrefix(hostname, "127.") {
		return stderrors.New("access to localhost not allowed")
	}

	// Block metadata endpoints (AWS, GCP, Azure)
	if hostname == "169.254.169.254" || hostname == "metadata.google.internal" {
		return stderrors.New("access to metadata endpoint not allowed")
	}

	// Block common internal domains
	internalDomains := []string{".local", ".internal", ".corp", ".lan"}
	for _, domain := range internalDomains {
		if strings.HasSuffix(hostname, domain) {
			return stderrors.New("access to internal domains not allowed")
		}
	}

	return nil
}

func isPrivateIP(hostname string) bool {
	// Try to parse as IP first
	ip := net.ParseIP(hostname)
	if ip != nil {
		return isPrivateIPAddress(ip)
	}

	// If it's a hostname, resolve it to IPs
	ips, err := net.LookupIP(hostname)
	if err != nil {
		// Block on resolution failure as a security measure
		return true
	}

	// Check if any resolved IP is private
	for _, ip := range ips {
		if isPrivateIPAddress(ip) {
			return true
		}
	}

	return false
}

func isPrivateIPAddress(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	// Check for private IPv4 ranges
	if ip.To4() != nil {
		// 10.0.0.0/8
		if ip[0] == 10 {
			return true
		}
		// 172.16.0.0/12
		if ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31 {
			return true
		}
		// 192.168.0.0/16
		if ip[0] == 192 && ip[1] == 168 {
			return true
		}
	}

	// Check for private IPv6 ranges
	if ip.To16() != nil && ip.To4() == nil {
		// Check for unique local addresses (fc00::/7)
		if ip[0] == 0xfc || ip[0] == 0xfd {
			return true
		}
	}

	return false
}

// Optimize feeds response by truncating descriptions and removing unnecessary fields
func optimizeFeedsResponse(feeds []*domain.FeedItem) []*domain.FeedItem {
	for _, feed := range feeds {
		feed.Title = strings.TrimSpace(feed.Title)
		// Simplified optimization for now
		if len(feed.Description) > 500 {
			feed.Description = feed.Description[:500] + "..."
		}
	}
	return feeds
}

func deriveNextCursorFromFeeds(feeds []*domain.FeedItem) (string, bool) {
	if len(feeds) == 0 {
		return "", false
	}

	lastFeed := feeds[len(feeds)-1]
	if !lastFeed.PublishedParsed.IsZero() {
		return lastFeed.PublishedParsed.Format(time.RFC3339), true
	}

	published := strings.TrimSpace(lastFeed.Published)
	if published == "" {
		return "", false
	}

	parsed, err := time.Parse(time.RFC3339, published)
	if err != nil {
		logger.Logger.Warn(
			"failed to parse published timestamp for next cursor",
			"published",
			published,
			"error",
			err,
		)
		return "", false
	}

	lastFeed.PublishedParsed = parsed
	return parsed.Format(time.RFC3339), true
}

// Determine cache age based on limit to optimize caching strategy
func getCacheAgeForLimit(limit int) int {
	switch {
	case limit <= 20:
		return 600 // 10 minutes for small requests
	case limit <= 100:
		return 900 // 15 minutes for medium requests
	default:
		return 1800 // 30 minutes for large requests
	}
}

// fetchArticleContent fetches article content, first from DB, then from URL if not found
// Returns: content, articleID, articleTitle, error
func fetchArticleContent(ctx context.Context, feedURL string, container *di.ApplicationComponents) (string, string, string, error) {
	logger.Logger.Info("Fetching article content", "url", feedURL)

	// First, try to fetch from database
	article, err := container.AltDBRepository.FetchArticleByURL(ctx, feedURL)
	if err != nil {
		logger.Logger.Error("Failed to query article from database", "error", err, "url", feedURL)
		return "", "", "", fmt.Errorf("failed to query article from database: %w", err)
	}

	// If article exists in DB, return it
	if article != nil {
		logger.Logger.Info("Article found in database", "url", feedURL, "content_length", len(article.Content))
		return article.Content, article.ID, article.Title, nil
	}

	// Article not in DB, fetch from URL and extract clean text
	logger.Logger.Info("Article not in database, fetching from URL", "url", feedURL)

	// Use ArchiveArticleUsecase which fetches HTML, extracts title and content, and saves to DB
	// Pass empty title so it will be extracted from HTML
	archiveInput := archive_article_usecase.ArchiveArticleInput{
		URL:   feedURL,
		Title: "", // Let the usecase extract title from HTML
	}
	err = container.ArchiveArticleUsecase.Execute(ctx, archiveInput)
	if err != nil {
		logger.Logger.Error("Failed to archive article", "error", err, "url", feedURL)
		return "", "", "", fmt.Errorf("failed to archive article: %w", err)
	}

	logger.Logger.Info("Article archived successfully", "url", feedURL)

	// Fetch the saved article to get its ID, title, and content
	savedArticle, err := container.AltDBRepository.FetchArticleByURL(ctx, feedURL)
	if err != nil || savedArticle == nil {
		// Fall back to generated ID and fallback title if fetch fails
		articleID := generateArticleID(feedURL)
		logger.Logger.Warn("Failed to fetch saved article, using generated ID and fallback title", "url", feedURL, "generated_id", articleID)
		return "", articleID, feedURL, fmt.Errorf("failed to fetch saved article after archiving")
	}

	return savedArticle.Content, savedArticle.ID, savedArticle.Title, nil
}

// generateArticleID generates a simple article ID from URL
func generateArticleID(feedURL string) string {
	// Simple implementation - you might want to use a hash function or UUID
	return fmt.Sprintf("article_%s", strings.ReplaceAll(feedURL, "/", "_"))
}

// callPreProcessorSummarize calls the pre-processor summarization API
func callPreProcessorSummarize(ctx context.Context, content string, articleID string, articleTitle string, preProcessorURL string) (string, error) {
	// Prepare request
	requestBody := map[string]interface{}{
		"content":    content,
		"article_id": articleID,
		"title":      articleTitle,
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
