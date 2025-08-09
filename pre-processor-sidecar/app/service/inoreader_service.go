//go:generate mockgen -source=inoreader_service.go -destination=../mocks/oauth2_driver_mock.go -package=mocks OAuth2Driver,APIUsageRepository

package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"pre-processor-sidecar/models"
	"github.com/google/uuid"
)

// OAuth2Driver interface for OAuth2 client operations
type OAuth2Driver interface {
	RefreshToken(ctx context.Context, refreshToken string) (*models.InoreaderTokenResponse, error)
	ValidateToken(ctx context.Context, accessToken string) (bool, error)
	MakeAuthenticatedRequest(ctx context.Context, accessToken, endpoint string, params map[string]string) (map[string]interface{}, error)
	MakeAuthenticatedRequestWithHeaders(ctx context.Context, accessToken, endpoint string, params map[string]string) (map[string]interface{}, map[string]string, error)
}

// APIUsageRepository interface for API usage tracking operations
type APIUsageRepository interface {
	GetTodaysUsage(ctx context.Context) (*models.APIUsageTracking, error)
	CreateUsageRecord(ctx context.Context, usage *models.APIUsageTracking) error
	UpdateUsageRecord(ctx context.Context, usage *models.APIUsageTracking) error
}

// InoreaderService handles Inoreader API interactions with OAuth2 and rate limiting
type InoreaderService struct {
	oauth2Client           OAuth2Driver
	apiUsageRepo           APIUsageRepository
	logger                 *slog.Logger
	currentToken           *models.OAuth2Token
	tokenRefreshBuffer     time.Duration
	apiDailyLimit          int
	maxArticlesPerRequest  int
	safetyBuffer          int
	rateLimitInfo         *models.APIRateLimitInfo
}

// NewInoreaderService creates a new Inoreader API service
func NewInoreaderService(oauth2Client OAuth2Driver, apiUsageRepo APIUsageRepository, logger *slog.Logger) *InoreaderService {
	// Use default logger if none provided
	if logger == nil {
		logger = slog.Default()
	}

	return &InoreaderService{
		oauth2Client:          oauth2Client,
		apiUsageRepo:          apiUsageRepo,
		logger:               logger,
		tokenRefreshBuffer:   5 * time.Minute, // Refresh 5 minutes before expiry
		apiDailyLimit:        100,             // Zone 1 API limit
		maxArticlesPerRequest: 100,            // Inoreader max per request
		safetyBuffer:         10,              // Safety buffer to avoid hitting exact limit
		rateLimitInfo: &models.APIRateLimitInfo{
			Zone1Limit: 100,
			Zone2Limit: 100,
		},
	}
}

// SetCurrentToken sets the current OAuth2 token
func (s *InoreaderService) SetCurrentToken(token *models.OAuth2Token) {
	s.currentToken = token
}

// RefreshTokenIfNeeded checks if the OAuth2 token needs refresh and refreshes if necessary
func (s *InoreaderService) RefreshTokenIfNeeded(ctx context.Context) error {
	if s.currentToken == nil {
		return fmt.Errorf("no current token available")
	}

	// Check if token needs refresh (with buffer time)
	if !s.currentToken.NeedsRefresh(s.tokenRefreshBuffer) {
		return nil // Token is still valid
	}

	s.logger.Info("OAuth2 token needs refresh",
		"expires_at", s.currentToken.ExpiresAt,
		"buffer", s.tokenRefreshBuffer)

	// Refresh the token
	response, err := s.oauth2Client.RefreshToken(ctx, s.currentToken.RefreshToken)
	if err != nil {
		s.logger.Error("Failed to refresh OAuth2 token", "error", err)
		return fmt.Errorf("OAuth2 token refresh failed: %w", err)
	}

	// Update current token with new values
	s.currentToken.UpdateFromRefresh(*response)

	s.logger.Info("OAuth2 token refreshed successfully",
		"expires_at", s.currentToken.ExpiresAt,
		"token_type", s.currentToken.TokenType)

	return nil
}

// FetchSubscriptions retrieves user's subscription list from Inoreader API
func (s *InoreaderService) FetchSubscriptions(ctx context.Context) ([]*models.Subscription, error) {
	// Ensure we have a valid token
	if err := s.RefreshTokenIfNeeded(ctx); err != nil {
		return nil, fmt.Errorf("token refresh failed: %w", err)
	}

	// Check rate limits
	if allowed, remaining := s.CheckAPIRateLimit(); !allowed {
		s.logger.Warn("API rate limit exceeded",
			"zone1_usage", s.rateLimitInfo.Zone1Usage,
			"zone1_limit", s.rateLimitInfo.Zone1Limit,
			"remaining_safe", remaining)
		return nil, fmt.Errorf("API rate limit exceeded (Zone 1: %d/%d)",
			s.rateLimitInfo.Zone1Usage, s.rateLimitInfo.Zone1Limit)
	}

	s.logger.Info("Fetching subscription list from Inoreader API")

	// Make API call to fetch subscriptions
	response, err := s.oauth2Client.MakeAuthenticatedRequest(
		ctx,
		s.currentToken.AccessToken,
		"/subscription/list",
		nil,
	)
	if err != nil {
		s.logger.Error("Failed to fetch subscriptions", "error", err)
		return nil, fmt.Errorf("subscription fetch failed: %w", err)
	}

	// Parse subscriptions from response
	subscriptions, err := s.parseSubscriptionsResponse(response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse subscriptions: %w", err)
	}

	s.logger.Info("Successfully fetched subscriptions",
		"count", len(subscriptions),
		"api_usage", s.rateLimitInfo.Zone1Usage)

	return subscriptions, nil
}

// FetchStreamContents retrieves stream contents (articles) from Inoreader API
func (s *InoreaderService) FetchStreamContents(ctx context.Context, streamID, continuationToken string) ([]*models.Article, string, error) {
	// Ensure we have a valid token
	if err := s.RefreshTokenIfNeeded(ctx); err != nil {
		return nil, "", fmt.Errorf("token refresh failed: %w", err)
	}

	// Check rate limits
	if allowed, remaining := s.CheckAPIRateLimit(); !allowed {
		s.logger.Warn("API rate limit exceeded for stream contents",
			"stream_id", streamID,
			"zone1_usage", s.rateLimitInfo.Zone1Usage,
			"remaining_safe", remaining)
		return nil, "", fmt.Errorf("API rate limit exceeded (Zone 1: %d/%d)",
			s.rateLimitInfo.Zone1Usage, s.rateLimitInfo.Zone1Limit)
	}

	s.logger.Info("Fetching stream contents from Inoreader API",
		"stream_id", streamID,
		"continuation_token", continuationToken != "")

	// Prepare API parameters
	params := map[string]string{
		"output": "json",
		"n":      strconv.Itoa(s.maxArticlesPerRequest),
	}
	if continuationToken != "" {
		params["c"] = continuationToken
	}

	// Make API call to fetch stream contents
	response, err := s.oauth2Client.MakeAuthenticatedRequest(
		ctx,
		s.currentToken.AccessToken,
		"/stream/contents/"+streamID,
		params,
	)
	if err != nil {
		s.logger.Error("Failed to fetch stream contents",
			"stream_id", streamID,
			"error", err)
		return nil, "", fmt.Errorf("stream contents fetch failed: %w", err)
	}

	// Parse articles and continuation token from response
	articles, nextContinuation, err := s.parseStreamContentsResponse(response)
	if err != nil {
		return nil, "", fmt.Errorf("failed to parse stream contents: %w", err)
	}

	s.logger.Info("Successfully fetched stream contents",
		"stream_id", streamID,
		"articles_count", len(articles),
		"has_next_page", nextContinuation != "",
		"api_usage", s.rateLimitInfo.Zone1Usage)

	return articles, nextContinuation, nil
}

// CheckAPIRateLimit checks if API requests are within safe limits
func (s *InoreaderService) CheckAPIRateLimit() (allowed bool, remaining int) {
	// Calculate remaining requests with safety buffer
	remainingWithBuffer := s.rateLimitInfo.Zone1Limit - s.rateLimitInfo.Zone1Usage - s.safetyBuffer
	if remainingWithBuffer < 0 {
		remainingWithBuffer = 0
	}

	allowed = s.rateLimitInfo.Zone1Usage < (s.rateLimitInfo.Zone1Limit - s.safetyBuffer)
	
	return allowed, remainingWithBuffer
}

// parseSubscriptionsResponse parses the subscription list response from Inoreader API
func (s *InoreaderService) parseSubscriptionsResponse(response map[string]interface{}) ([]*models.Subscription, error) {
	subscriptionsData, ok := response["subscriptions"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid subscriptions response format")
	}

	var subscriptions []*models.Subscription

	for _, subData := range subscriptionsData {
		subMap, ok := subData.(map[string]interface{})
		if !ok {
			continue
		}

		// Extract basic subscription info
		inoreaderID, _ := subMap["id"].(string)
		feedURL, _ := subMap["url"].(string)
		title, _ := subMap["title"].(string)

		// Extract category (use first category if multiple)
		var category string
		if categories, ok := subMap["categories"].([]interface{}); ok && len(categories) > 0 {
			if categoryMap, ok := categories[0].(map[string]interface{}); ok {
				category, _ = categoryMap["label"].(string)
			}
		}

		// Create subscription model
		subscription := models.NewSubscription(inoreaderID, feedURL, title, category)
		subscriptions = append(subscriptions, subscription)
	}

	return subscriptions, nil
}

// parseStreamContentsResponse parses stream contents response from Inoreader API
func (s *InoreaderService) parseStreamContentsResponse(response map[string]interface{}) ([]*models.Article, string, error) {
	itemsData, ok := response["items"].([]interface{})
	if !ok {
		return nil, "", fmt.Errorf("invalid stream contents response format")
	}

	var articles []*models.Article

	for _, itemData := range itemsData {
		itemMap, ok := itemData.(map[string]interface{})
		if !ok {
			continue
		}

		// Extract basic article info
		inoreaderID, _ := itemMap["id"].(string)
		title, _ := itemMap["title"].(string)
		author, _ := itemMap["author"].(string)

		// Extract article URL from alternate links
		var articleURL string
		if alternates, ok := itemMap["alternate"].([]interface{}); ok && len(alternates) > 0 {
			if altMap, ok := alternates[0].(map[string]interface{}); ok {
				articleURL, _ = altMap["href"].(string)
			}
		}

		// Extract published timestamp
		var publishedAt time.Time
		if published, ok := itemMap["published"].(float64); ok {
			publishedAt = time.Unix(int64(published), 0)
		}

		// Extract subscription ID from origin for UUID resolution
		var originStreamID string
		if origin, ok := itemMap["origin"].(map[string]interface{}); ok {
			originStreamID, _ = origin["streamId"].(string)
		}

		// Create article with OriginStreamID for later UUID resolution
		// SubscriptionID will be resolved by ArticleFetchService using subscription mapping cache
		article := &models.Article{
			ID:             uuid.New(),
			InoreaderID:    inoreaderID,
			SubscriptionID: uuid.Nil, // Will be resolved later by UUID mapping
			ArticleURL:     articleURL,
			Title:          title,
			Author:         author,
			PublishedAt:    &publishedAt,
			FetchedAt:      time.Now(),
			Processed:      false,
			OriginStreamID: originStreamID, // Set for UUID resolution
		}
		articles = append(articles, article)
	}

	// Extract continuation token for pagination
	var continuationToken string
	if continuation, ok := response["continuation"].(string); ok {
		continuationToken = continuation
	}

	return articles, continuationToken, nil
}

// UpdateAPIUsageFromHeaders updates API usage tracking from response headers
func (s *InoreaderService) UpdateAPIUsageFromHeaders(ctx context.Context, headers map[string]string, endpoint string) error {
	if s.apiUsageRepo == nil {
		s.logger.Debug("API usage repository not configured, skipping usage tracking")
		return nil
	}

	// Get or create today's usage record
	usage, err := s.apiUsageRepo.GetTodaysUsage(ctx)
	if err != nil {
		// Create new usage record for today
		usage = models.NewAPIUsageTracking()
		if err := s.apiUsageRepo.CreateUsageRecord(ctx, usage); err != nil {
			s.logger.Error("Failed to create API usage record", "error", err)
			return fmt.Errorf("failed to create API usage record: %w", err)
		}
		s.logger.Debug("Created new API usage record for today")
	}

	// Check if usage should be reset (new day)
	if usage.ShouldResetUsage() {
		usage.ResetUsage()
		s.logger.Info("Reset API usage counters for new day")
	}

	// Parse and update rate limit headers
	headerMap := make(map[string]interface{})
	for key, value := range headers {
		headerMap[key] = value
	}
	usage.UpdateRateLimitHeaders(headerMap)

	// Increment appropriate usage counter based on endpoint
	if isReadOnlyEndpoint(endpoint) {
		usage.IncrementZone1Usage()
		s.logger.Debug("Incremented Zone 1 API usage", "endpoint", endpoint, "new_count", usage.Zone1Requests)
	} else {
		usage.IncrementZone2Usage()
		s.logger.Debug("Incremented Zone 2 API usage", "endpoint", endpoint, "new_count", usage.Zone2Requests)
	}

	// Update rate limit info from headers
	s.updateRateLimitInfoFromHeaders(headers, usage)

	// Save updated usage record
	if err := s.apiUsageRepo.UpdateUsageRecord(ctx, usage); err != nil {
		s.logger.Error("Failed to update API usage record", "error", err)
		return fmt.Errorf("failed to update API usage record: %w", err)
	}

	// Log current usage info
	usageInfo := usage.GetUsageInfo()
	s.logger.Info("Updated API usage tracking",
		"endpoint", endpoint,
		"zone1_usage", usageInfo.Zone1Requests,
		"remaining", usageInfo.Remaining,
		"daily_limit", usageInfo.DailyLimit)

	return nil
}

// updateRateLimitInfoFromHeaders updates internal rate limit info from API response headers
func (s *InoreaderService) updateRateLimitInfoFromHeaders(headers map[string]string, usage *models.APIUsageTracking) {
	// Parse Inoreader-specific rate limit headers
	if zone1Usage, ok := headers["X-Reader-Zone1-Usage"]; ok {
		if parsed, err := strconv.Atoi(zone1Usage); err == nil {
			s.rateLimitInfo.Zone1Usage = parsed
		}
	}
	
	if zone1Limit, ok := headers["X-Reader-Zone1-Limit"]; ok {
		if parsed, err := strconv.Atoi(zone1Limit); err == nil {
			s.rateLimitInfo.Zone1Limit = parsed
		}
	}

	if zone1Remaining, ok := headers["X-Reader-Zone1-Remaining"]; ok {
		if parsed, err := strconv.Atoi(zone1Remaining); err == nil {
			s.rateLimitInfo.Zone1Remaining = parsed
		}
	}

	if zone2Usage, ok := headers["X-Reader-Zone2-Usage"]; ok {
		if parsed, err := strconv.Atoi(zone2Usage); err == nil {
			s.rateLimitInfo.Zone2Usage = parsed
		}
	}

	// Update timestamp
	s.rateLimitInfo.LastUpdated = time.Now()

	s.logger.Debug("Updated rate limit info from headers",
		"zone1_usage", s.rateLimitInfo.Zone1Usage,
		"zone1_limit", s.rateLimitInfo.Zone1Limit,
		"zone1_remaining", s.rateLimitInfo.Zone1Remaining)
}

// isReadOnlyEndpoint determines if an endpoint is read-only (Zone 1) or write (Zone 2)
func isReadOnlyEndpoint(endpoint string) bool {
	readOnlyEndpoints := []string{
		"/subscription/list",
		"/stream/contents/",
		"/stream/items/contents",
		"/user-info",
	}

	for _, readOnly := range readOnlyEndpoints {
		if endpoint == readOnly || (readOnly[len(readOnly)-1] == '/' && len(endpoint) > len(readOnly) && endpoint[:len(readOnly)] == readOnly) {
			return true
		}
	}

	return false
}

// GetCurrentAPIUsageInfo returns current API usage information for monitoring
func (s *InoreaderService) GetCurrentAPIUsageInfo(ctx context.Context) (*models.APIUsageInfo, error) {
	if s.apiUsageRepo == nil {
		return &models.APIUsageInfo{
			Zone1Requests: s.rateLimitInfo.Zone1Usage,
			DailyLimit:    s.rateLimitInfo.Zone1Limit,
			Remaining:     s.rateLimitInfo.Zone1Remaining,
		}, nil
	}

	usage, err := s.apiUsageRepo.GetTodaysUsage(ctx)
	if err != nil {
		s.logger.Debug("No usage record found, returning default info")
		return &models.APIUsageInfo{
			Zone1Requests: 0,
			DailyLimit:    100,
			Remaining:     100,
		}, nil
	}

	return usage.GetUsageInfo(), nil
}