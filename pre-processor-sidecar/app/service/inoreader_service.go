// ABOUTME: Business logic service for Inoreader API interactions
// ABOUTME: Orchestrates API calls, token management, and rate limiting

package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"pre-processor-sidecar/models"
	"pre-processor-sidecar/repository"
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

	// rateLimitMu guards every field of rateLimitInfo. The scheduler
	// serializes its article-fetch runs and its subscription-refresh runs
	// separately, so a stream fetch and a subscription list call can still
	// read and increment Zone1Usage concurrently.
	rateLimitMu sync.RWMutex

	fetchMu             sync.Mutex
	lastSuccessfulFetch time.Time
}

// zone1Snapshot returns a consistent (usage, limit) pair under rateLimitMu.
func (s *InoreaderService) zone1Snapshot() (usage, limit int) {
	s.rateLimitMu.RLock()
	defer s.rateLimitMu.RUnlock()
	return s.rateLimitInfo.Zone1Usage, s.rateLimitInfo.Zone1Limit
}

// incrementUsageLocally bumps the in-process counter for the zone the endpoint
// belongs to and returns the new Zone-1 value, which is what CheckAPIRateLimit
// gates on.
func (s *InoreaderService) incrementUsageLocally(zone1 bool) int {
	s.rateLimitMu.Lock()
	defer s.rateLimitMu.Unlock()
	if zone1 {
		s.rateLimitInfo.Zone1Usage++
	} else {
		s.rateLimitInfo.Zone2Usage++
	}
	return s.rateLimitInfo.Zone1Usage
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

	// Announce the tracking wiring instead of discovering it call by call: without
	// the repository the Zone 1 budget is only counted in-process and a restart
	// hands the fetch loop a fresh 100 requests.
	if apiUsageRepo == nil {
		logger.Warn("api_usage_tracking_disabled",
			"reason", "no APIUsageRepository wired",
			"effect", "Zone 1 quota counted in-process only, reset on restart")
	}

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
			usage, limit := s.zone1Snapshot()
			s.logger.Warn("API rate limit exceeded",
				"zone1_usage", usage,
				"zone1_limit", limit,
				"remaining_safe", remaining)
			return fmt.Errorf("API rate limit exceeded (Zone 1: %d/%d)", usage, limit)
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

		// Charge the call against the 100 req/day Zone 1 budget. The in-process
		// counter has already advanced when this returns an error, so the guard
		// keeps working even if the write to api_usage_tracking failed.
		if usageErr := s.recordAPIUsage(ctx, "/subscription/list"); usageErr != nil {
			s.logger.Warn("Failed to persist API usage tracking", "error", usageErr)
		}

		usage, _ := s.zone1Snapshot()
		s.logger.Info("Successfully fetched subscriptions",
			"count", len(subscriptions),
			"api_usage", usage)

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
			usage, limit := s.zone1Snapshot()
			s.logger.Warn("API rate limit exceeded for stream contents",
				"stream_id", streamID,
				"zone1_usage", usage,
				"remaining_safe", remaining)
			return fmt.Errorf("API rate limit exceeded (Zone 1: %d/%d)", usage, limit)
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

		// Charge the call against the 100 req/day Zone 1 budget (see FetchSubscriptions).
		endpoint := "/stream/contents/" + streamID
		if usageErr := s.recordAPIUsage(ctx, endpoint); usageErr != nil {
			s.logger.Warn("Failed to persist API usage tracking", "error", usageErr, "stream_id", streamID)
		}

		usage, _ := s.zone1Snapshot()
		s.logger.Info("Successfully fetched stream contents",
			"stream_id", streamID,
			"articles_count", len(articles),
			"has_next_page", nextContinuation != "",
			"api_usage", usage)

		s.recordSuccessfulFetch()
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
		usage, limit := s.zone1Snapshot()
		s.logger.Warn("API rate limit exceeded for unread stream contents",
			"stream_id", streamID,
			"zone1_usage", usage,
			"remaining_safe", remaining)
		return nil, "", fmt.Errorf("API rate limit exceeded (Zone 1: %d/%d)", usage, limit)
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

	// Charge the call against the 100 req/day Zone 1 budget (see FetchSubscriptions).
	endpoint := "/stream/contents/" + streamID + "?xt=user/-/state/com.google/read"
	if usageErr := s.recordAPIUsage(ctx, endpoint); usageErr != nil {
		s.logger.Warn("Failed to persist API usage tracking", "error", usageErr, "stream_id", streamID)
	}

	usage, _ := s.zone1Snapshot()
	s.logger.Info("Successfully fetched unread stream contents",
		"stream_id", streamID,
		"articles_count", len(articles),
		"has_next_page", nextContinuation != "",
		"api_usage", usage)

	return articles, nextContinuation, nil
}

// CheckAPIRateLimit checks if API requests are within safe limits
func (s *InoreaderService) CheckAPIRateLimit() (allowed bool, remaining int) {
	usage, limit := s.zone1Snapshot()

	// Calculate remaining requests with safety buffer
	remainingWithBuffer := limit - usage - s.safetyBuffer
	if remainingWithBuffer < 0 {
		remainingWithBuffer = 0
	}

	allowed = usage < (limit - s.safetyBuffer)

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

// recordAPIUsage charges one Inoreader call against the daily quota.
//
// The in-process counter that CheckAPIRateLimit gates on is advanced first and
// unconditionally: a Postgres hiccup must never be indistinguishable from an
// unlimited budget. Persistence to api_usage_tracking follows, so the 100 req/day
// Zone 1 limit survives a sidecar restart instead of starting over at zero.
//
// Inoreader's X-Reader-Zone1-* response headers would be the authoritative source,
// but the client layer returns only the decoded body and drops them; until it
// forwards them, the counter is derived from the calls we make.
func (s *InoreaderService) recordAPIUsage(ctx context.Context, endpoint string) error {
	newUsage := s.incrementUsageLocally(s.isReadOnlyEndpoint(endpoint))

	if s.apiUsageRepo == nil {
		return fmt.Errorf("api usage repository not wired: %s counted in-process only (zone1_usage=%d)", endpoint, newUsage)
	}

	return s.processAPIUsageHeaders(ctx, nil, endpoint)
}

// processAPIUsageHeaders persists one API call — and any rate limit headers that
// came back with it — to today's api_usage_tracking row. recordAPIUsage is the
// gatekeeper for a nil repository; reaching here without one is a wiring bug.
func (s *InoreaderService) processAPIUsageHeaders(ctx context.Context, headers map[string]string, endpoint string) error {
	// Get or create today's usage record. Only "there is no row for today yet" may
	// seed a fresh counter: UpdateUsageRecord below rewrites the row's counters
	// wholesale, so treating a transient read failure as a new day would overwrite
	// a day already at 61/100 with 1 — invisible until the next restart inherits
	// the 1 and spends the real Inoreader budget a second time.
	usage, err := s.apiUsageRepo.GetTodaysUsage(ctx)
	if err != nil {
		if !errors.Is(err, repository.ErrNoUsageRecordToday) {
			s.logger.Error("Failed to read today's API usage record", "error", err)
			return fmt.Errorf("failed to read today's API usage record: %w", err)
		}

		// Create new usage record for today
		usage = models.NewAPIUsageTracking()
		if err := s.apiUsageRepo.CreateUsageRecord(ctx, usage); err != nil {
			s.logger.Error("Failed to create API usage record", "error", err)
			return fmt.Errorf("failed to create API usage record: %w", err)
		}
		s.logger.Debug("Created new API usage record for today")
	}

	// Check if usage should be reset (new day)
	resetForNewDay := usage.ShouldResetUsage()
	if resetForNewDay {
		usage.ResetUsage()
		s.logger.Info("Reset API usage counters for new day")
	}

	// Parse and update rate limit headers. Skipped when the caller had none, or an
	// empty map would wipe the headers the last header-bearing response stored.
	if len(headers) > 0 {
		headerMap := make(map[string]interface{}, len(headers))
		for key, value := range headers {
			headerMap[key] = value
		}
		usage.UpdateRateLimitHeaders(headerMap)
	}

	// Increment appropriate usage counter based on endpoint
	if s.isReadOnlyEndpoint(endpoint) {
		usage.IncrementZone1Usage()
		s.logger.Debug("Incremented Zone 1 API usage", "endpoint", endpoint, "new_count", usage.Zone1Requests)
	} else {
		usage.IncrementZone2Usage()
		s.logger.Debug("Incremented Zone 2 API usage", "endpoint", endpoint, "new_count", usage.Zone2Requests)
	}

	// Update rate limit info from the persisted counters and any headers
	s.updateRateLimitInfoFromHeaders(headers, usage, resetForNewDay)

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

// updateRateLimitInfoFromHeaders refreshes the in-process rate limit view from the
// persisted daily counters and, when the response carried them, Inoreader's own
// X-Reader-Zone* headers.
func (s *InoreaderService) updateRateLimitInfoFromHeaders(headers map[string]string, usage *models.APIUsageTracking, resetForNewDay bool) {
	s.rateLimitMu.Lock()
	defer s.rateLimitMu.Unlock()

	// api_usage_tracking outlives the process, so it — not the counter this process
	// happens to have accumulated — is the real budget. Adopt it whenever it is
	// ahead (restart mid-day, a run that died after the write). Follow it downwards
	// only across a day rollover, or a freshly reset row would leave the guard
	// latched at yesterday's count.
	if resetForNewDay || usage.Zone1Requests > s.rateLimitInfo.Zone1Usage {
		s.rateLimitInfo.Zone1Usage = usage.Zone1Requests
	}
	if resetForNewDay || usage.Zone2Requests > s.rateLimitInfo.Zone2Usage {
		s.rateLimitInfo.Zone2Usage = usage.Zone2Requests
	}

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
		s.rateLimitMu.RLock()
		defer s.rateLimitMu.RUnlock()
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

// CircuitBreakerState returns the circuit breaker state as a string ("CLOSED", "OPEN",
// "HALF_OPEN"). Used by the /admin/health surface so the IngestionHealthProvider impl
// stays decoupled from the utils package types.
func (s *InoreaderService) CircuitBreakerState() string {
	return s.circuitBreaker.GetState().String()
}

// LastSuccessfulFetch returns the wall-clock timestamp of the most recent successful
// stream-content fetch. Zero value means no successful fetch since process start.
func (s *InoreaderService) LastSuccessfulFetch() time.Time {
	s.fetchMu.Lock()
	defer s.fetchMu.Unlock()
	return s.lastSuccessfulFetch
}

func (s *InoreaderService) recordSuccessfulFetch() {
	s.fetchMu.Lock()
	defer s.fetchMu.Unlock()
	s.lastSuccessfulFetch = time.Now()
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
