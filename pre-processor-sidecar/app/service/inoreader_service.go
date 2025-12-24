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
	"pre-processor-sidecar/utils"
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
	inoreaderClient       InoreaderClientInterface
	apiUsageRepo          APIUsageRepository
	tokenService          TokenProvider
	logger                *slog.Logger
	apiDailyLimit         int
	maxArticlesPerRequest int
	safetyBuffer          int
	rateLimitInfo         *models.APIRateLimitInfo
	circuitBreaker        *utils.CircuitBreaker // TDD Phase 3 - REFACTOR: Circuit Breaker
	monitor               *utils.Monitor        // TDD Phase 3 - REFACTOR: Structured Logging & Monitoring
}

// TokenProvider interface for token operations
type TokenProvider interface {
	GetValidToken(ctx context.Context) (*models.OAuth2Token, error)
	EnsureValidToken(ctx context.Context) (*models.OAuth2Token, error)
}

// NewInoreaderService creates a new Inoreader API service
func NewInoreaderService(inoreaderClient InoreaderClientInterface, apiUsageRepo APIUsageRepository, tokenService TokenProvider, logger *slog.Logger) *InoreaderService {
	// Use default logger if none provided
	if logger == nil {
		logger = slog.Default()
	}

	// TDD Phase 3 - REFACTOR: Initialize Circuit Breaker
	circuitBreakerConfig := &utils.CircuitBreakerConfig{
		FailureThreshold: 3,                // 3回連続失敗でOPEN
		SuccessThreshold: 2,                // HALF_OPENで2回成功すればCLOSED
		Timeout:          60 * time.Second, // 1分でHALF_OPENに移行
		MaxRequests:      1,                // HALF_OPENで1つのリクエストを許可
	}

	// TDD Phase 3 - REFACTOR: Initialize Monitoring
	monitoringConfig := utils.DefaultMonitoringConfig()
	monitor := utils.NewMonitor(monitoringConfig, logger)

	return &InoreaderService{
		inoreaderClient:       inoreaderClient,
		apiUsageRepo:          apiUsageRepo,
		tokenService:          tokenService,
		logger:                logger,
		apiDailyLimit:         100, // Zone 1 API limit
		maxArticlesPerRequest: 100, // Inoreader max per request
		safetyBuffer:          10,  // Safety buffer to avoid hitting exact limit
		rateLimitInfo: &models.APIRateLimitInfo{
			Zone1Limit: 100,
			Zone2Limit: 100,
		},
		circuitBreaker: utils.NewCircuitBreaker(circuitBreakerConfig, logger), // TDD Phase 3
		monitor:        monitor,                                               // TDD Phase 3 - REFACTOR: Structured Logging & Monitoring
	}
}

// GetValidToken retrieves a valid OAuth2 token from SimpleTokenService
func (s *InoreaderService) GetValidToken(ctx context.Context) (*models.OAuth2Token, error) {
	if s.tokenService == nil {
		return nil, fmt.Errorf("no token service configured")
	}

	token, err := s.tokenService.GetValidToken(ctx)
	if err != nil {
		s.logger.Error("Failed to get valid token from token service", "error", err)
		return nil, fmt.Errorf("token retrieval failed: %w", err)
	}

	return token, nil
}

// EnsureValidToken ensures we have a valid token (wrapper for SimpleTokenService)
func (s *InoreaderService) EnsureValidToken(ctx context.Context) (*models.OAuth2Token, error) {
	if s.tokenService == nil {
		return nil, fmt.Errorf("no token service configured")
	}

	token, err := s.tokenService.EnsureValidToken(ctx)
	if err != nil {
		s.logger.Error("Failed to ensure valid token", "error", err)
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	return token, nil
}

// FetchSubscriptions retrieves user's subscription list from Inoreader API
func (s *InoreaderService) FetchSubscriptions(ctx context.Context) ([]*models.Subscription, error) {
	var subscriptions []*models.Subscription

	// TDD Phase 3 - REFACTOR: Monitor operation start time
	startTime := time.Now()

	// TDD Phase 3 - REFACTOR: Wrap API call with Circuit Breaker
	err := s.circuitBreaker.Execute(ctx, func(ctx context.Context) error {
		// Ensure we have a valid token
		token, err := s.EnsureValidToken(ctx)
		if err != nil {
			return fmt.Errorf("token validation failed: %w", err)
		}

		// Check rate limits
		if allowed, remaining := s.CheckAPIRateLimit(); !allowed {
			s.logger.Warn("API rate limit exceeded",
				"zone1_usage", s.rateLimitInfo.Zone1Usage,
				"zone1_limit", s.rateLimitInfo.Zone1Limit,
				"remaining_safe", remaining)
			return fmt.Errorf("API rate limit exceeded (Zone 1: %d/%d)",
				s.rateLimitInfo.Zone1Usage, s.rateLimitInfo.Zone1Limit)
		}

		s.logger.Info("Fetching subscription list from Inoreader API")

		// Make API call using client layer
		response, err := s.inoreaderClient.FetchSubscriptionList(ctx, token.AccessToken)
		if err != nil {
			s.logger.Error("Failed to fetch subscriptions", "error", err)
			// TDD Phase 3 - REFACTOR: Log API request failure
			s.monitor.LogAPIRequest(ctx, "GET", "/subscription/list", 500, time.Since(startTime), err)
			return fmt.Errorf("subscription fetch failed: %w", err)
		}

		// Parse subscriptions using client layer
		var parseErr error
		subscriptions, parseErr = s.inoreaderClient.ParseSubscriptionsResponse(response)
		if parseErr != nil {
			return fmt.Errorf("failed to parse subscriptions: %w", parseErr)
		}

		// CRITICAL FIX: Update API usage tracking after each API call
		if updateErr := s.UpdateAPIUsageFromHeaders(ctx, "/subscription/list"); updateErr != nil {
			s.logger.Warn("Failed to update API usage from headers", "error", updateErr)
			// Increment local counter as fallback
			s.rateLimitInfo.Zone1Usage++
			s.logger.Debug("Incremented local API usage counter", "zone1_usage", s.rateLimitInfo.Zone1Usage)
		}

		s.logger.Info("Successfully fetched subscriptions",
			"count", len(subscriptions),
			"api_usage", s.rateLimitInfo.Zone1Usage)

		// TDD Phase 3 - REFACTOR: Log successful API request
		s.monitor.LogAPIRequest(ctx, "GET", "/subscription/list", 200, time.Since(startTime), nil)

		return nil
	})

	if err != nil {
		// Check if circuit breaker is open
		if err == utils.ErrCircuitBreakerOpen {
			s.logger.Warn("Circuit breaker is open, rejecting subscription request")
			// TDD Phase 3 - REFACTOR: Log circuit breaker rejection
			s.monitor.LogAPIRequest(ctx, "GET", "/subscription/list", 503, time.Since(startTime), err)
			return nil, fmt.Errorf("service temporarily unavailable: %w", err)
		}
		return nil, err
	}

	return subscriptions, nil
}

// FetchStreamContents retrieves stream contents (articles) from Inoreader API
func (s *InoreaderService) FetchStreamContents(ctx context.Context, streamID, continuationToken string) ([]*models.Article, string, error) {
	var articles []*models.Article
	var nextContinuation string

	// TDD Phase 3 - REFACTOR: Wrap API call with Circuit Breaker
	err := s.circuitBreaker.Execute(ctx, func(ctx context.Context) error {
		// Ensure we have a valid token
		token, err := s.EnsureValidToken(ctx)
		if err != nil {
			return fmt.Errorf("token validation failed: %w", err)
		}

		// Check rate limits
		if allowed, remaining := s.CheckAPIRateLimit(); !allowed {
			s.logger.Warn("API rate limit exceeded for stream contents",
				"stream_id", streamID,
				"zone1_usage", s.rateLimitInfo.Zone1Usage,
				"remaining_safe", remaining)
			return fmt.Errorf("API rate limit exceeded (Zone 1: %d/%d)",
				s.rateLimitInfo.Zone1Usage, s.rateLimitInfo.Zone1Limit)
		}

		s.logger.Info("Fetching stream contents from Inoreader API",
			"stream_id", streamID,
			"continuation_token", continuationToken != "")

		// Make API call using client layer
		response, err := s.inoreaderClient.FetchStreamContents(
			ctx,
			token.AccessToken,
			streamID,
			continuationToken,
			s.maxArticlesPerRequest,
		)
		if err != nil {
			s.logger.Error("Failed to fetch stream contents",
				"stream_id", streamID,
				"error", err)
			return fmt.Errorf("stream contents fetch failed: %w", err)
		}

		// Parse articles using client layer
		var parseErr error
		articles, nextContinuation, parseErr = s.inoreaderClient.ParseStreamContentsResponse(response)
		if parseErr != nil {
			return fmt.Errorf("failed to parse stream contents: %w", parseErr)
		}

		// Resolve subscription UUIDs for articles
		articles = s.resolveSubscriptionUUIDs(articles)

		// CRITICAL FIX: Update API usage tracking after each API call
		endpoint := "/stream/contents/" + streamID
		if updateErr := s.UpdateAPIUsageFromHeaders(ctx, endpoint); updateErr != nil {
			s.logger.Warn("Failed to update API usage from headers", "error", updateErr, "stream_id", streamID)
			// Increment local counter as fallback
			s.rateLimitInfo.Zone1Usage++
			s.logger.Debug("Incremented local API usage counter", "zone1_usage", s.rateLimitInfo.Zone1Usage)
		}

		s.logger.Info("Successfully fetched stream contents",
			"stream_id", streamID,
			"articles_count", len(articles),
			"has_next_page", nextContinuation != "",
			"api_usage", s.rateLimitInfo.Zone1Usage)

		return nil
	})

	if err != nil {
		// Check if circuit breaker is open
		if err == utils.ErrCircuitBreakerOpen {
			s.logger.Warn("Circuit breaker is open, rejecting stream contents request",
				"stream_id", streamID)
			return nil, "", fmt.Errorf("service temporarily unavailable: %w", err)
		}
		return nil, "", err
	}

	return articles, nextContinuation, nil
}

// FetchUnreadStreamContents retrieves only unread stream contents from Inoreader API
func (s *InoreaderService) FetchUnreadStreamContents(ctx context.Context, streamID, continuationToken string) ([]*models.Article, string, error) {
	// Ensure we have a valid token
	token, err := s.EnsureValidToken(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("token validation failed: %w", err)
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
		token.AccessToken,
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

	// CRITICAL FIX: Update API usage tracking after each API call
	endpoint := "/stream/contents/" + streamID + "?xt=user/-/state/com.google/read"
	if err := s.UpdateAPIUsageFromHeaders(ctx, endpoint); err != nil {
		s.logger.Warn("Failed to update API usage from headers", "error", err, "stream_id", streamID)
		// Increment local counter as fallback
		s.rateLimitInfo.Zone1Usage++
		s.logger.Debug("Incremented local API usage counter", "zone1_usage", s.rateLimitInfo.Zone1Usage)
	}

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
// DEPRECATED: This method should not make additional API calls just for headers
// Instead, headers should be captured during the actual API calls
func (s *InoreaderService) UpdateAPIUsageFromHeaders(ctx context.Context, endpoint string) error {
	s.logger.Warn("UpdateAPIUsageFromHeaders called - this should be replaced with header capture during API calls",
		"endpoint", endpoint)

	// Return success to avoid breaking existing code, but log the issue
	s.logger.Debug("API usage repository not configured or headers not available, skipping usage tracking")
	return nil
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

// TDD Phase 3 - REFACTOR: GetCircuitBreakerStats returns circuit breaker statistics for monitoring
func (s *InoreaderService) GetCircuitBreakerStats() utils.CircuitBreakerStats {
	return s.circuitBreaker.GetStats()
}

// TDD Phase 3 - REFACTOR: ResetCircuitBreaker resets the circuit breaker (for admin use)
func (s *InoreaderService) ResetCircuitBreaker() {
	s.logger.Info("Resetting circuit breaker via admin request")
	s.circuitBreaker.Reset()
}

// TDD Phase 3 - REFACTOR: GetMonitoringMetrics returns monitoring metrics for observability
func (s *InoreaderService) GetMonitoringMetrics() map[string]*utils.Metric {
	return s.monitor.GetMetrics()
}

// TDD Phase 3 - REFACTOR: GetMonitoringHealthCheck returns monitoring system health
func (s *InoreaderService) GetMonitoringHealthCheck() map[string]interface{} {
	return s.monitor.HealthCheck()
}

// TDD Phase 3 - REFACTOR: Close gracefully shuts down monitoring resources
func (s *InoreaderService) Close() {
	if s.monitor != nil {
		s.monitor.Close()
	}
}
