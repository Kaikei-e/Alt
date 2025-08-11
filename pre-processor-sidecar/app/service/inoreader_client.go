// ABOUTME: Low-level HTTP client for Inoreader API communication
// ABOUTME: Handles authentication, requests, and response parsing

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"pre-processor-sidecar/driver"
	"pre-processor-sidecar/models"
)

// RetryConfig defines retry behavior for API requests
type RetryConfig struct {
	MaxRetries    int
	InitialDelay  time.Duration
	MaxDelay      time.Duration
	Multiplier    float64
}

// InoreaderClient handles low-level HTTP communication with Inoreader API
type InoreaderClient struct {
	oauth2Driver OAuth2Driver
	logger       *slog.Logger
	baseURL      string
	retryConfig  *RetryConfig
}

// NewInoreaderClient creates a new Inoreader API client
func NewInoreaderClient(oauth2Driver OAuth2Driver, logger *slog.Logger) *InoreaderClient {
	if logger == nil {
		logger = slog.Default()
	}

	// デフォルトのリトライ設定
	defaultRetryConfig := &RetryConfig{
		MaxRetries:   3,
		InitialDelay: 5 * time.Second, // レート制限準拠
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
	}

	return &InoreaderClient{
		oauth2Driver: oauth2Driver,
		logger:       logger,
		baseURL:      "", // Empty - OAuth2Client already has full base URL
		retryConfig:  defaultRetryConfig,
	}
}

// FetchSubscriptionList fetches subscription list from Inoreader API
func (c *InoreaderClient) FetchSubscriptionList(ctx context.Context, accessToken string) (map[string]interface{}, error) {
	endpoint := "/subscription/list"  // OAuth2Client already has full base URL
	params := map[string]string{
		"output": "json",
	}

	c.logger.Debug("Fetching subscription list from Inoreader API",
		"endpoint", endpoint)

	response, err := c.oauth2Driver.MakeAuthenticatedRequest(ctx, accessToken, endpoint, params)
	if err != nil {
		c.logger.Error("Failed to fetch subscription list",
			"endpoint", endpoint,
			"error", err)
		return nil, fmt.Errorf("subscription list API call failed: %w", err)
	}

	c.logger.Debug("Successfully fetched subscription list",
		"endpoint", endpoint,
		"response_keys", c.getResponseKeys(response))

	return response, nil
}

// FetchStreamContents fetches stream contents (articles) from Inoreader API
func (c *InoreaderClient) FetchStreamContents(ctx context.Context, accessToken, streamID, continuationToken string, maxArticles int) (map[string]interface{}, error) {
	// URL encode the streamID for safe API call
	encodedStreamID := url.QueryEscape(streamID)
	endpoint := "/stream/contents/" + encodedStreamID  // OAuth2Client already has full base URL

	params := map[string]string{
		"output": "json",
		"n":      strconv.Itoa(maxArticles),
	}

	// Add continuation token if provided
	if continuationToken != "" {
		params["c"] = continuationToken
	}

	c.logger.Debug("Fetching stream contents from Inoreader API",
		"endpoint", endpoint,
		"stream_id", streamID,
		"max_articles", maxArticles,
		"has_continuation", continuationToken != "")

	response, err := c.oauth2Driver.MakeAuthenticatedRequest(ctx, accessToken, endpoint, params)
	if err != nil {
		c.logger.Error("Failed to fetch stream contents",
			"endpoint", endpoint,
			"stream_id", streamID,
			"error", err)
		return nil, fmt.Errorf("stream contents API call failed: %w", err)
	}

	c.logger.Debug("Successfully fetched stream contents",
		"endpoint", endpoint,
		"stream_id", streamID,
		"response_keys", c.getResponseKeys(response))

	return response, nil
}

// FetchUnreadStreamContents fetches only unread articles from a stream
func (c *InoreaderClient) FetchUnreadStreamContents(ctx context.Context, accessToken, streamID, continuationToken string, maxArticles int) (map[string]interface{}, error) {
	// URL encode the streamID for safe API call
	encodedStreamID := url.QueryEscape(streamID)
	endpoint := "/stream/contents/" + encodedStreamID  // OAuth2Client already has full base URL

	params := map[string]string{
		"output": "json",
		"n":      strconv.Itoa(maxArticles),
		"xt":     "user/-/state/com.google/read", // Exclude read articles
	}

	// Add continuation token if provided
	if continuationToken != "" {
		params["c"] = continuationToken
	}

	c.logger.Debug("Fetching unread stream contents from Inoreader API",
		"endpoint", endpoint,
		"stream_id", streamID,
		"max_articles", maxArticles,
		"has_continuation", continuationToken != "")

	response, err := c.oauth2Driver.MakeAuthenticatedRequest(ctx, accessToken, endpoint, params)
	if err != nil {
		c.logger.Error("Failed to fetch unread stream contents",
			"endpoint", endpoint,
			"stream_id", streamID,
			"error", err)
		return nil, fmt.Errorf("unread stream contents API call failed: %w", err)
	}

	c.logger.Debug("Successfully fetched unread stream contents",
		"endpoint", endpoint,
		"stream_id", streamID,
		"response_keys", c.getResponseKeys(response))

	return response, nil
}

// RefreshToken refreshes the OAuth2 access token
func (c *InoreaderClient) RefreshToken(ctx context.Context, refreshToken string) (*models.InoreaderTokenResponse, error) {
	c.logger.Debug("Refreshing OAuth2 token")

	response, err := c.oauth2Driver.RefreshToken(ctx, refreshToken)
	if err != nil {
		c.logger.Error("Failed to refresh OAuth2 token", "error", err)
		return nil, fmt.Errorf("token refresh failed: %w", err)
	}

	c.logger.Debug("Successfully refreshed OAuth2 token",
		"token_type", response.TokenType,
		"expires_in", response.ExpiresIn)

	return response, nil
}

// ValidateToken validates an OAuth2 access token
func (c *InoreaderClient) ValidateToken(ctx context.Context, accessToken string) (bool, error) {
	c.logger.Debug("Validating OAuth2 token")

	isValid, err := c.oauth2Driver.ValidateToken(ctx, accessToken)
	if err != nil {
		c.logger.Error("Failed to validate OAuth2 token", "error", err)
		return false, fmt.Errorf("token validation failed: %w", err)
	}

	c.logger.Debug("OAuth2 token validation completed", "is_valid", isValid)
	return isValid, nil
}

// MakeAuthenticatedRequest makes a generic authenticated request to Inoreader API
func (c *InoreaderClient) MakeAuthenticatedRequest(ctx context.Context, accessToken, endpoint string, params map[string]string) (map[string]interface{}, error) {
	fullEndpoint := endpoint  // OAuth2Client already has full base URL

	c.logger.Debug("Making authenticated request to Inoreader API",
		"endpoint", fullEndpoint,
		"params_count", len(params))

	response, err := c.oauth2Driver.MakeAuthenticatedRequest(ctx, accessToken, fullEndpoint, params)
	if err != nil {
		c.logger.Error("Authenticated request failed",
			"endpoint", fullEndpoint,
			"error", err)
		return nil, fmt.Errorf("authenticated request failed: %w", err)
	}

	c.logger.Debug("Authenticated request completed successfully",
		"endpoint", fullEndpoint,
		"response_keys", c.getResponseKeys(response))

	return response, nil
}

// MakeAuthenticatedRequestWithHeaders makes an authenticated request and returns response headers
func (c *InoreaderClient) MakeAuthenticatedRequestWithHeaders(ctx context.Context, accessToken, endpoint string, params map[string]string) (map[string]interface{}, map[string]string, error) {
	fullEndpoint := endpoint  // OAuth2Client already has full base URL

	c.logger.Debug("Making authenticated request with headers to Inoreader API",
		"endpoint", fullEndpoint,
		"params_count", len(params))

	response, headers, err := c.oauth2Driver.MakeAuthenticatedRequestWithHeaders(ctx, accessToken, fullEndpoint, params)
	if err != nil {
		c.logger.Error("Authenticated request with headers failed",
			"endpoint", fullEndpoint,
			"error", err)
		return nil, nil, fmt.Errorf("authenticated request with headers failed: %w", err)
	}

	c.logger.Debug("Authenticated request with headers completed successfully",
		"endpoint", fullEndpoint,
		"response_keys", c.getResponseKeys(response),
		"header_count", len(headers))

	return response, headers, nil
}

// ParseSubscriptionsResponse parses the subscription list response
func (c *InoreaderClient) ParseSubscriptionsResponse(response map[string]interface{}) ([]*models.Subscription, error) {
	subscriptionsData, ok := response["subscriptions"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid subscriptions response format: missing 'subscriptions' field")
	}

	var subscriptions []*models.Subscription

	for i, subData := range subscriptionsData {
		subMap, ok := subData.(map[string]interface{})
		if !ok {
			c.logger.Warn("Invalid subscription data format, skipping",
				"index", i,
				"type", fmt.Sprintf("%T", subData))
			continue
		}

		subscription, err := c.parseSubscriptionData(subMap)
		if err != nil {
			c.logger.Warn("Failed to parse subscription data, skipping",
				"index", i,
				"error", err)
			continue
		}

		subscriptions = append(subscriptions, subscription)
	}

	c.logger.Info("Parsed subscriptions response successfully",
		"total_subscriptions", len(subscriptions),
		"raw_count", len(subscriptionsData))

	return subscriptions, nil
}

// ParseStreamContentsResponse parses the stream contents response using structured binding
func (c *InoreaderClient) ParseStreamContentsResponse(response map[string]interface{}) ([]*models.Article, string, error) {
	// Phase 2: Log raw JSON response for debugging
	jsonData, err := json.Marshal(response)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal response data: %w", err)
	}

	// Phase 2: Enhanced raw response logging
	c.logger.Debug("Raw JSON response from Inoreader API",
		"json_length", len(jsonData),
		"response_keys", c.getResponseKeys(response),
		"raw_json_sample", string(jsonData[:c.minInt(500, len(jsonData))]))

	var streamResponse driver.StreamContentsResponse
	if err := json.Unmarshal(jsonData, &streamResponse); err != nil {
		// Phase 2: Enhanced error logging with raw data
		c.logger.Error("Failed to unmarshal stream contents response",
			"error", err,
			"raw_json_length", len(jsonData),
			"raw_json_preview", string(jsonData[:c.minInt(200, len(jsonData))]))
		return nil, "", fmt.Errorf("failed to unmarshal stream contents response: %w", err)
	}

	// Phase 1: Enhanced logging with structured data
	c.logger.Info("Stream contents response parsed with structured binding",
		"total_items", len(streamResponse.Items),
		"has_continuation", streamResponse.Continuation != "",
		"stream_id", streamResponse.ID,
		"updated", streamResponse.GetUpdatedTime())

	// Phase 2: Initialize content statistics tracking
	contentStats := struct {
		totalArticles    int
		articlesWithContent int
		articlesEmpty    int
		totalContentChars int
		rtlArticles      int
		truncatedArticles int
	}{}

	var articles []*models.Article

	// Phase 1: Process items using structured types
	for i, item := range streamResponse.Items {
		article, err := c.parseArticleDataFromStruct(item)
		if err != nil {
			c.logger.Warn("Failed to parse article data from struct, skipping",
				"index", i,
				"article_id", item.ID,
				"title", item.Title,
				"error", err)
			continue
		}

		// Phase 2: Update content statistics
		contentStats.totalArticles++
		contentStatus := "empty"
		
		if item.Summary.HasContent() {
			contentStatus = "present"
			contentStats.articlesWithContent++
			contentStats.totalContentChars += len(item.Summary.Content)
			
			if item.Summary.IsRTL() {
				contentStats.rtlArticles++
			}
			
			// Check if content was truncated
			if len(item.Summary.Content) >= 50000 {
				contentStats.truncatedArticles++
			}
		} else {
			contentStats.articlesEmpty++
		}
		
		c.logger.Debug("Article processed with statistics",
			"index", i,
			"article_id", item.ID,
			"title", item.Title,
			"content_status", contentStatus,
			"content_length", len(item.Summary.Content),
			"canonical_url", item.GetCanonicalURL(),
			"is_rtl", item.Summary.IsRTL())

		articles = append(articles, article)
	}

	// Phase 2: Calculate content availability metrics
	contentAvailabilityRate := 0.0
	if contentStats.totalArticles > 0 {
		contentAvailabilityRate = float64(contentStats.articlesWithContent) / float64(contentStats.totalArticles) * 100.0
	}

	avgContentLength := 0
	if contentStats.articlesWithContent > 0 {
		avgContentLength = contentStats.totalContentChars / contentStats.articlesWithContent
	}

	// Phase 2: Comprehensive content statistics logging
	c.logger.Info("Content extraction statistics",
		"total_articles_processed", contentStats.totalArticles,
		"articles_with_content", contentStats.articlesWithContent,
		"articles_empty", contentStats.articlesEmpty,
		"content_availability_rate_percent", fmt.Sprintf("%.1f", contentAvailabilityRate),
		"total_content_characters", contentStats.totalContentChars,
		"average_content_length", avgContentLength,
		"rtl_articles", contentStats.rtlArticles,
		"truncated_articles", contentStats.truncatedArticles)

	c.logger.Info("Parsed stream contents response successfully with structured binding",
		"total_articles", len(articles),
		"total_items", len(streamResponse.Items),
		"has_continuation", streamResponse.Continuation != "",
		"content_success_rate", fmt.Sprintf("%.1f%%", contentAvailabilityRate))

	return articles, streamResponse.Continuation, nil
}

// parseSubscriptionData parses individual subscription data from API response
func (c *InoreaderClient) parseSubscriptionData(subMap map[string]interface{}) (*models.Subscription, error) {
	// Extract basic subscription info
	inoreaderID, _ := subMap["id"].(string)
	feedURL, _ := subMap["url"].(string)
	title, _ := subMap["title"].(string)

	if inoreaderID == "" || feedURL == "" {
		return nil, fmt.Errorf("missing required subscription fields: id=%s, url=%s", inoreaderID, feedURL)
	}

	// Extract category (use first category if multiple)
	category := ""
	if categories, ok := subMap["categories"].([]interface{}); ok && len(categories) > 0 {
		if categoryMap, ok := categories[0].(map[string]interface{}); ok {
			if label, ok := categoryMap["label"].(string); ok {
				category = label
			}
		}
	}

	// Create subscription using factory method
	subscription := models.NewSubscription(inoreaderID, feedURL, title, category)

	return subscription, nil
}

// parseArticleData parses individual article data from API response
func (c *InoreaderClient) parseArticleData(itemMap map[string]interface{}) (*models.Article, error) {
	// Extract basic article info
	inoreaderID, _ := itemMap["id"].(string)
	title, _ := itemMap["title"].(string)
	author, _ := itemMap["author"].(string)

	if inoreaderID == "" {
		return nil, fmt.Errorf("missing required article field: id")
	}

	// Extract article URL from canonical links
	var articleURL string
	if canonical, ok := itemMap["canonical"].([]interface{}); ok && len(canonical) > 0 {
		if linkMap, ok := canonical[0].(map[string]interface{}); ok {
			if href, ok := linkMap["href"].(string); ok {
				articleURL = href
			}
		}
	}

	// Extract origin stream ID for UUID mapping
	var originStreamID string
	if origin, ok := itemMap["origin"].(map[string]interface{}); ok {
		if streamID, ok := origin["streamId"].(string); ok {
			originStreamID = streamID
		}
	}

	// Phase 4: Extract article content from summary.content field
	var content string
	var contentLength int
	var contentType string = "html"
	
	if summary, ok := itemMap["summary"].(map[string]interface{}); ok {
		if summaryContent, ok := summary["content"].(string); ok {
			// Apply content length limit (50KB for storage optimization)
			const maxContentLength = 50000
			if len(summaryContent) > maxContentLength {
				content = summaryContent[:maxContentLength] + "\n<!-- Content truncated for storage optimization -->"
				contentLength = maxContentLength
			} else {
				content = summaryContent
				contentLength = len(summaryContent)
			}
		}
		
		// Set content type based on direction for RTL languages
		if direction, ok := summary["direction"].(string); ok && direction == "rtl" {
			contentType = "html_rtl"
		}
	}

	// Create article with temporary values (UUID will be resolved later)
	article := &models.Article{
		ID:             models.NewUUID(),
		InoreaderID:    inoreaderID,
		ArticleURL:     articleURL,
		Title:          title,
		Author:         author,
		FetchedAt:      models.Now(),
		Processed:      false,
		OriginStreamID: originStreamID, // Temporary field for UUID resolution
		// Phase 4: Store extracted content fields
		Content:        content,
		ContentLength:  contentLength,
		ContentType:    contentType,
	}

	// Extract and parse published timestamp
	if published, ok := itemMap["published"]; ok {
		if publishedFloat, ok := published.(float64); ok {
			publishedTime := models.TimeFromUnix(int64(publishedFloat))
			article.PublishedAt = &publishedTime
		}
	}

	return article, nil
}

// parseArticleDataFromStruct parses individual article data from structured Inoreader response
// Phase 1: Structured binding implementation replacing map[string]interface{} parsing
func (c *InoreaderClient) parseArticleDataFromStruct(item driver.InoreaderArticleItem) (*models.Article, error) {
	// Validate required fields
	if item.ID == "" {
		return nil, fmt.Errorf("missing required article field: id")
	}

	// Extract article URL from canonical links (structured access)
	articleURL := item.GetCanonicalURL()

	// Extract origin stream ID (structured access)
	originStreamID := item.GetOriginStreamID()

	// Phase 1: Extract article content using structured Summary field
	var content string
	var contentLength int
	var contentType string = "html"

	// Phase 2: Enhanced content logging with structured data
	c.logger.Debug("Processing article content",
		"article_id", item.ID,
		"summary_has_content", item.Summary.HasContent(),
		"summary_content_length", len(item.Summary.Content),
		"summary_direction", item.Summary.Direction)

	if item.Summary.HasContent() {
		// Apply content length limit (50KB for storage optimization)
		const maxContentLength = 50000
		if len(item.Summary.Content) > maxContentLength {
			content = item.Summary.Content[:maxContentLength] + "\n<!-- Content truncated for storage optimization -->"
			contentLength = maxContentLength
			c.logger.Info("Content truncated due to size limit",
				"article_id", item.ID,
				"original_length", len(item.Summary.Content),
				"truncated_length", maxContentLength)
		} else {
			content = item.Summary.Content
			contentLength = len(item.Summary.Content)
		}

		// Set content type based on direction for RTL languages
		if item.Summary.IsRTL() {
			contentType = "html_rtl"
		}

		c.logger.Info("Article content extracted successfully",
			"article_id", item.ID,
			"title", item.Title,
			"content_length", contentLength,
			"content_type", contentType)
	} else {
		c.logger.Warn("Article has no content in summary field",
			"article_id", item.ID,
			"title", item.Title,
			"canonical_url", articleURL,
			"origin_stream_id", originStreamID)
	}

	// Create article with structured field access
	article := &models.Article{
		ID:             models.NewUUID(),
		InoreaderID:    item.ID,
		ArticleURL:     articleURL,
		Title:          item.Title,
		Author:         item.Author,
		FetchedAt:      models.Now(),
		Processed:      false,
		OriginStreamID: originStreamID, // Temporary field for UUID resolution
		// Phase 1: Store extracted content fields using structured access
		Content:        content,
		ContentLength:  contentLength,
		ContentType:    contentType,
	}

	// Extract and parse published timestamp using structured access
	if item.Published > 0 {
		publishedTime := item.GetPublishedTime()
		article.PublishedAt = &publishedTime
	}

	return article, nil
}

// getResponseKeys returns the keys of the response map for debugging
func (c *InoreaderClient) getResponseKeys(response map[string]interface{}) []string {
	keys := make([]string, 0, len(response))
	for key := range response {
		keys = append(keys, key)
	}
	return keys
}

// minInt returns the minimum of two integers (helper function)
func (c *InoreaderClient) minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// FetchSubscriptionListWithRetry fetches subscription list with retry logic
func (c *InoreaderClient) FetchSubscriptionListWithRetry(ctx context.Context, accessToken string) (map[string]interface{}, error) {
	var lastErr error
	
	for attempt := 0; attempt <= c.retryConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(float64(c.retryConfig.InitialDelay) * math.Pow(c.retryConfig.Multiplier, float64(attempt-1)))
			if delay > c.retryConfig.MaxDelay {
				delay = c.retryConfig.MaxDelay
			}
			
			c.logger.Info("リトライ実行", 
				"attempt", attempt,
				"delay_seconds", delay.Seconds())
			
			time.Sleep(delay)
		}
		
		result, err := c.FetchSubscriptionList(ctx, accessToken)
		if err == nil {
			return result, nil
		}
		
		// リトライ可能なエラーかチェック
		if isRetryableError(err) {
			lastErr = err
			continue
		}
		
		return nil, err
	}
	
	return nil, fmt.Errorf("最大リトライ回数超過 (%d回): %w", c.retryConfig.MaxRetries, lastErr)
}

// SetRetryConfig sets the retry configuration
func (c *InoreaderClient) SetRetryConfig(config RetryConfig) {
	c.retryConfig = &config
}

// GetRetryConfig returns the current retry configuration
func (c *InoreaderClient) GetRetryConfig() *RetryConfig {
	return c.retryConfig
}

// isRetryableError determines if an error should trigger a retry
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	
	errStr := err.Error()
	return strings.Contains(errStr, "403") ||
		   strings.Contains(errStr, "408") ||
		   strings.Contains(errStr, "timeout") ||
		   strings.Contains(errStr, "connection refused")
}