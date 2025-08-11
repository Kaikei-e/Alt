// ABOUTME: Low-level HTTP client for Inoreader API communication
// ABOUTME: Handles authentication, requests, and response parsing

package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

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

// ParseStreamContentsResponse parses the stream contents response
func (c *InoreaderClient) ParseStreamContentsResponse(response map[string]interface{}) ([]*models.Article, string, error) {
	itemsData, ok := response["items"].([]interface{})
	if !ok {
		return nil, "", fmt.Errorf("invalid stream contents response format: missing 'items' field")
	}

	// Extract continuation token for pagination
	continuationToken := ""
	if token, exists := response["continuation"]; exists {
		if tokenStr, ok := token.(string); ok {
			continuationToken = tokenStr
		}
	}

	var articles []*models.Article

	for i, itemData := range itemsData {
		itemMap, ok := itemData.(map[string]interface{})
		if !ok {
			c.logger.Warn("Invalid article item data format, skipping",
				"index", i,
				"type", fmt.Sprintf("%T", itemData))
			continue
		}

		article, err := c.parseArticleData(itemMap)
		if err != nil {
			c.logger.Warn("Failed to parse article data, skipping",
				"index", i,
				"error", err)
			continue
		}

		articles = append(articles, article)
	}

	c.logger.Info("Parsed stream contents response successfully",
		"total_articles", len(articles),
		"raw_count", len(itemsData),
		"has_continuation", continuationToken != "")

	return articles, continuationToken, nil
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

// getResponseKeys returns the keys of the response map for debugging
func (c *InoreaderClient) getResponseKeys(response map[string]interface{}) []string {
	keys := make([]string, 0, len(response))
	for key := range response {
		keys = append(keys, key)
	}
	return keys
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