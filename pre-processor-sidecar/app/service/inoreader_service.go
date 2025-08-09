// ABOUTME: Business logic service for Inoreader API interactions
// ABOUTME: Orchestrates API calls, token management, and rate limiting

package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"pre-processor-sidecar/models"
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

// InoreaderClientInterface defines interface for HTTP communication layer
type InoreaderClientInterface interface {
	FetchSubscriptionList(ctx context.Context, accessToken string) (map[string]interface{}, error)
	FetchStreamContents(ctx context.Context, accessToken, streamID, continuationToken string, maxArticles int) (map[string]interface{}, error)
	FetchUnreadStreamContents(ctx context.Context, accessToken, streamID, continuationToken string, maxArticles int) (map[string]interface{}, error)
	RefreshToken(ctx context.Context, refreshToken string) (*models.InoreaderTokenResponse, error)
	ValidateToken(ctx context.Context, accessToken string) (bool, error)
	MakeAuthenticatedRequestWithHeaders(ctx context.Context, accessToken, endpoint string, params map[string]string) (map[string]interface{}, map[string]string, error)
	ParseSubscriptionsResponse(response map[string]interface{}) ([]*models.Subscription, error)
	ParseStreamContentsResponse(response map[string]interface{}) ([]*models.Article, string, error)
}

// InoreaderService handles business logic for Inoreader API interactions
type InoreaderService struct {
	inoreaderClient        InoreaderClientInterface
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
func NewInoreaderService(inoreaderClient InoreaderClientInterface, apiUsageRepo APIUsageRepository, logger *slog.Logger) *InoreaderService {
	// Use default logger if none provided
	if logger == nil {
		logger = slog.Default()
	}

	return &InoreaderService{
		inoreaderClient:       inoreaderClient,
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

	// Refresh the token using client layer
	response, err := s.inoreaderClient.RefreshToken(ctx, s.currentToken.RefreshToken)
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

	// Make API call using client layer
	response, err := s.inoreaderClient.FetchSubscriptionList(ctx, s.currentToken.AccessToken)
	if err != nil {
		s.logger.Error("Failed to fetch subscriptions", "error", err)
		return nil, fmt.Errorf("subscription fetch failed: %w", err)
	}

	// Parse subscriptions using client layer
	subscriptions, err := s.inoreaderClient.ParseSubscriptionsResponse(response)
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

	// Make API call using client layer
	response, err := s.inoreaderClient.FetchStreamContents(
		ctx, 
		s.currentToken.AccessToken, 
		streamID, 
		continuationToken, 
		s.maxArticlesPerRequest,
	)
	if err != nil {
		s.logger.Error("Failed to fetch stream contents",
			"stream_id", streamID,
			"error", err)
		return nil, "", fmt.Errorf("stream contents fetch failed: %w", err)
	}

	// Parse articles using client layer
	articles, nextContinuation, err := s.inoreaderClient.ParseStreamContentsResponse(response)
	if err != nil {
		return nil, "", fmt.Errorf("failed to parse stream contents: %w", err)
	}

	// Resolve subscription UUIDs for articles
	articles = s.resolveSubscriptionUUIDs(articles)

	s.logger.Info("Successfully fetched stream contents",
		"stream_id", streamID,
		"articles_count", len(articles),
		"has_next_page", nextContinuation != "",
		"api_usage", s.rateLimitInfo.Zone1Usage)

	return articles, nextContinuation, nil
}

// FetchUnreadStreamContents retrieves only unread stream contents from Inoreader API
func (s *InoreaderService) FetchUnreadStreamContents(ctx context.Context, streamID, continuationToken string) ([]*models.Article, string, error) {
	// Ensure we have a valid token
	if err := s.RefreshTokenIfNeeded(ctx); err != nil {
		return nil, "", fmt.Errorf("token refresh failed: %w", err)
	}

	// Check rate limits
	if allowed, remaining := s.CheckAPIRateLimit(); !allowed {
		s.logger.Warn("API rate limit exceeded for unread stream contents",
			"stream_id", streamID,
			"zone1_usage", s.rateLimitInfo.Zone1Usage,
			"remaining_safe", remaining)
		return nil, "", fmt.Errorf("API rate limit exceeded (Zone 1: %d/%d)",
			s.rateLimitInfo.Zone1Usage, s.rateLimitInfo.Zone1Limit)
	}

	s.logger.Info("Fetching unread stream contents from Inoreader API",
		"stream_id", streamID,
		"continuation_token", continuationToken != "")

	// Make API call using client layer for unread items
	response, err := s.inoreaderClient.FetchUnreadStreamContents(
		ctx, 
		s.currentToken.AccessToken, 
		streamID, 
		continuationToken, 
		s.maxArticlesPerRequest,
	)
	if err != nil {
		s.logger.Error("Failed to fetch unread stream contents",
			"stream_id", streamID,
			"error", err)
		return nil, "", fmt.Errorf("unread stream contents fetch failed: %w", err)
	}

	// Parse articles using client layer
	articles, nextContinuation, err := s.inoreaderClient.ParseStreamContentsResponse(response)
	if err != nil {
		return nil, "", fmt.Errorf("failed to parse unread stream contents: %w", err)
	}

	// Resolve subscription UUIDs for articles
	articles = s.resolveSubscriptionUUIDs(articles)

	s.logger.Info("Successfully fetched unread stream contents",
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

// resolveSubscriptionUUIDs resolves OriginStreamID to SubscriptionID for articles
// TODO: Implement subscription mapping cache for UUID resolution
func (s *InoreaderService) resolveSubscriptionUUIDs(articles []*models.Article) []*models.Article {
	// Placeholder: For now, articles keep uuid.Nil for SubscriptionID
	// This will be implemented when subscription mapping cache is available
	for _, article := range articles {
		if article.OriginStreamID != "" {
			s.logger.Debug("Article needs UUID resolution",
				"inoreader_id", article.InoreaderID,
				"origin_stream_id", article.OriginStreamID)
			// TODO: Look up UUID from OriginStreamID using subscription cache
			// article.SubscriptionID = lookupUUID(article.OriginStreamID)
		}
	}
	return articles
}

// UpdateAPIUsageFromHeaders updates API usage tracking from response headers
func (s *InoreaderService) UpdateAPIUsageFromHeaders(ctx context.Context, endpoint string) error {
	// Fetch headers using client with header support
	_, headers, err := s.inoreaderClient.MakeAuthenticatedRequestWithHeaders(
		ctx,
		s.currentToken.AccessToken,
		endpoint,
		nil,
	)
	if err != nil {
		s.logger.Error("Failed to fetch headers for API usage tracking", "error", err)
		return fmt.Errorf("failed to fetch headers: %w", err)
	}

	// Process the headers for rate limit tracking
	return s.processAPIUsageHeaders(ctx, headers, endpoint)
}

// processAPIUsageHeaders processes API response headers for usage tracking
func (s *InoreaderService) processAPIUsageHeaders(ctx context.Context, headers map[string]string, endpoint string) error {
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
	if s.isReadOnlyEndpoint(endpoint) {
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
		if parsed, err := strconv.ParseInt(zone1Usage, 10, 32); err == nil {
			s.rateLimitInfo.Zone1Usage = int(parsed)
		}
	}
	
	if zone1Limit, ok := headers["X-Reader-Zone1-Limit"]; ok {
		if parsed, err := strconv.ParseInt(zone1Limit, 10, 32); err == nil {
			s.rateLimitInfo.Zone1Limit = int(parsed)
		}
	}

	if zone1Remaining, ok := headers["X-Reader-Zone1-Remaining"]; ok {
		if parsed, err := strconv.ParseInt(zone1Remaining, 10, 32); err == nil {
			s.rateLimitInfo.Zone1Remaining = int(parsed)
		}
	}

	if zone2Usage, ok := headers["X-Reader-Zone2-Usage"]; ok {
		if parsed, err := strconv.ParseInt(zone2Usage, 10, 32); err == nil {
			s.rateLimitInfo.Zone2Usage = int(parsed)
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
func (s *InoreaderService) isReadOnlyEndpoint(endpoint string) bool {
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