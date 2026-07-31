// Package handler provides HTTP handlers for the BFF service.
package handler

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"alt-butterfly-facade/internal/cache"
	"alt-butterfly-facade/internal/client"
	"alt-butterfly-facade/internal/middleware"
	"alt-butterfly-facade/internal/resilience"
)

// BFFConfig holds configuration for BFF features.
type BFFConfig struct {
	// Feature flags
	EnableCache              bool
	EnableCircuitBreaker     bool
	EnableDedup              bool
	EnableErrorNormalization bool

	// Cache configuration
	CacheMaxSize    int
	CacheDefaultTTL time.Duration

	// Circuit breaker configuration
	CBFailureThreshold int
	CBSuccessThreshold int
	CBOpenTimeout      time.Duration

	// Dedup configuration
	DedupWindow time.Duration
}

// BFFHandler wraps the proxy handler with BFF features.
type BFFHandler struct {
	backendClient    *client.BackendClient
	authInterceptor  *middleware.AuthInterceptor
	logger           *slog.Logger
	config           BFFConfig
	defaultTimeout   time.Duration
	streamingTimeout time.Duration

	// BFF components
	responseCache *cache.ResponseCache
	cacheConfig   *cache.CacheConfig
	mutationCB    *resilience.CircuitBreaker
	projectionCB  *resilience.CircuitBreaker
	nonCriticalCB *resilience.CircuitBreaker
	deduplicator  *RequestDeduplicator
}

// NewBFFHandler creates a new BFF handler with all features.
func NewBFFHandler(
	backendClient *client.BackendClient,
	secret []byte,
	issuer, audience string,
	logger *slog.Logger,
	config BFFConfig,
) *BFFHandler {
	h := &BFFHandler{
		backendClient:    backendClient,
		authInterceptor:  middleware.NewAuthInterceptor(logger, secret, issuer, audience),
		logger:           logger,
		config:           config,
		defaultTimeout:   backendClient.DefaultTimeout(),
		streamingTimeout: backendClient.StreamingTimeout(),
	}

	// Initialize cache if enabled
	if config.EnableCache {
		h.responseCache = cache.NewResponseCache(config.CacheMaxSize)
		h.cacheConfig = cache.NewCacheConfig()
	}

	// Initialize per-dependency-class circuit breakers if enabled.
	// Mutation and unread-projection budgets are separate so GetAllFeeds
	// storms cannot trip MarkAsRead (adversarial review / per-path CB guidance).
	// Everything else shares the non-critical breaker (default / opt-in).
	if config.EnableCircuitBreaker {
		cbCfg := resilience.CircuitBreakerConfig{
			FailureThreshold: config.CBFailureThreshold,
			SuccessThreshold: config.CBSuccessThreshold,
			OpenTimeout:      config.CBOpenTimeout,
		}
		h.mutationCB = resilience.NewCircuitBreaker(cbCfg)
		h.projectionCB = resilience.NewCircuitBreaker(cbCfg)
		h.nonCriticalCB = resilience.NewCircuitBreaker(cbCfg)
		if logger != nil {
			logger.Info("circuit_breaker_classes",
				"critical_mutation", resilience.CriticalMutationEndpointCount(),
				"unread_projection", resilience.UnreadProjectionEndpointCount(),
				"non_critical", "default",
				"unclassified_as", "non_critical",
			)
		}
	}

	// Initialize deduplicator if enabled
	if config.EnableDedup {
		h.deduplicator = NewRequestDeduplicator(config.DedupWindow)
	}

	return h
}

// applyConnectTimeout parses the Connect-Timeout-Ms header and sets a context
// deadline on the request. If the header is absent or invalid, defaultTimeout
// is used. The timeout is capped at maxConnectTimeout (5 min).
func (h *BFFHandler) applyConnectTimeout(r *http.Request) (*http.Request, context.CancelFunc) {
	return resolveTimeout(r, h.defaultTimeout, maxConnectTimeout)
}

// applyStreamingConnectTimeout is applyConnectTimeout's streaming
// counterpart: base is streamingTimeout instead of defaultTimeout, capped at
// maxStreamingTimeout (40 min) instead of the unary 5 min cap. Mirrors
// ProxyHandler.applyConnectTimeout's streaming branch.
func (h *BFFHandler) applyStreamingConnectTimeout(r *http.Request) (*http.Request, context.CancelFunc) {
	return resolveTimeout(r, h.streamingTimeout, maxStreamingTimeout)
}

// ServeHTTP implements http.Handler.
func (h *BFFHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Streaming procedures bypass cache/dedup (streamed bodies aren't
	// cacheable/dedupable units) and stream response chunks directly with
	// Flusher.Flush instead of buffering the full body, but circuit breaker
	// gating/recording still applies — see serveStreaming.
	if isStreamingProcedure(r.URL.Path) {
		h.serveStreaming(w, r)
		return
	}

	// Apply Connect-Timeout-Ms based context deadline (unary only)
	r, cancel := h.applyConnectTimeout(r)
	defer cancel()

	// Generate request ID for tracing
	requestID := generateRequestID()

	// Extract and validate token
	token := r.Header.Get(middleware.BackendTokenHeader)
	userCtx, err := h.authInterceptor.ValidateToken(token)
	if err != nil {
		h.handleError(w, http.StatusUnauthorized, "Unauthorized", requestID)
		return
	}

	// Get user ID for caching and dedup
	userID := userCtx.UserID.String()
	endpoint := r.URL.Path

	// Check dependency-class circuit breaker. The permit is released on every
	// exit path: a cache hit or a body-read failure never reaches the backend
	// and so records no outcome, and a half-open trial permit that is never
	// handed back rejects the whole class until the process restarts.
	if cb := h.breakerFor(endpoint); cb != nil {
		permit, allowed := cb.Acquire()
		if !allowed {
			h.handleCircuitOpen(w, requestID)
			return
		}
		defer permit.Release()
	}

	// Read request body once, for cache key, dedup key, and forwarding.
	var body []byte
	if r.Body != nil {
		var err error
		body, err = io.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			h.handleError(w, http.StatusBadRequest, "Failed to read request body", requestID)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
	}

	// Check cache for cacheable endpoints
	if h.shouldUseCache(r.Method, endpoint) {
		if cached := h.checkCache(userID, endpoint, body); cached != nil {
			h.writeCachedResponse(w, cached)
			return
		}
	}

	// Handle request with deduplication if enabled
	if h.deduplicator != nil && r.Method == http.MethodPost {
		h.handleWithDedup(w, r, userID, endpoint, body, token, requestID)
		return
	}

	// Forward request directly
	h.forwardRequest(w, r, userID, endpoint, body, token, requestID)
}

// serveStreaming forwards a server-streaming Connect-RPC request straight to
// the backend and copies the response body chunk-by-chunk with
// Flusher.Flush via streamCopy (proxy_handler.go), so the client observes
// true streaming instead of the full body being buffered first. Cache and
// dedup are never consulted here — a streamed body is not a single
// cacheable/dedupable unit. The circuit breaker is still gated (Allow())
// and recorded (success/failure), judged solely by connection establishment
// and the initial response status; chunk content never influences it.
func (h *BFFHandler) serveStreaming(w http.ResponseWriter, r *http.Request) {
	r, cancel := h.applyStreamingConnectTimeout(r)
	defer cancel()

	requestID := generateRequestID()

	token := r.Header.Get(middleware.BackendTokenHeader)
	if _, err := h.authInterceptor.ValidateToken(token); err != nil {
		h.handleError(w, http.StatusUnauthorized, "Unauthorized", requestID)
		return
	}

	endpoint := r.URL.Path
	if cb := h.breakerFor(endpoint); cb != nil {
		permit, allowed := cb.Acquire()
		if !allowed {
			h.handleCircuitOpen(w, requestID)
			return
		}
		defer permit.Release()
	}

	resp, err := h.backendClient.ForwardStreamingRequest(r, token)
	if err != nil {
		h.recordFailure(endpoint)
		h.handleBackendError(w, err, requestID)
		return
	}
	defer resp.Body.Close()

	if IsErrorResponse(resp.StatusCode) {
		h.recordFailure(endpoint)
	} else {
		h.recordSuccess(endpoint)
	}

	// Error-normalization only ever rewrites the initial status; once we
	// decide to stream (below), the body is copied verbatim and untouched.
	if h.config.EnableErrorNormalization && IsErrorResponse(resp.StatusCode) {
		normalized := NormalizeError(resp, requestID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		jsonBytes, _ := normalized.ToJSON()
		w.Write(jsonBytes)
		return
	}

	copyResponseHeaders(resp.Header, w.Header())
	w.WriteHeader(resp.StatusCode)
	streamCopy(w, resp.Body, h.logger)
}

// handleWithDedup handles a request with deduplication.
func (h *BFFHandler) handleWithDedup(w http.ResponseWriter, r *http.Request, userID, endpoint string, body []byte, token, requestID string) {
	dedupKey := BuildDedupKey(userID, r.Method, endpoint, body)

	result, err := h.deduplicator.Do(dedupKey, func() (*DedupResult, error) {
		return h.executeRequest(r, userID, endpoint, body, token, requestID)
	})

	if err != nil {
		h.handleBackendError(w, err, requestID)
		return
	}

	if result != nil {
		h.writeResult(w, result)
	}
}

// executeRequest executes the actual backend request.
func (h *BFFHandler) executeRequest(r *http.Request, userID, endpoint string, body []byte, token, requestID string) (*DedupResult, error) {
	// Snapshot cache epoch before upstream I/O so a concurrent MarkAsRead
	// invalidation can fence this response from re-populating stale unread data.
	var cacheEpoch uint64
	fenceCache := h.responseCache != nil && h.cacheConfig != nil && h.cacheConfig.IsCacheable(endpoint)
	if fenceCache {
		cacheEpoch = h.responseCache.UserEpoch(userID)
	}

	// Create new request with body
	newReq := CreateDedupRequest(r, body)

	// Determine if streaming
	isStreaming := isStreamingProcedure(endpoint)

	var resp *http.Response
	var err error

	if isStreaming {
		resp, err = h.backendClient.ForwardStreamingRequest(newReq, token)
	} else {
		resp, err = h.backendClient.ForwardRequest(newReq, token)
	}

	if err != nil {
		h.recordFailure(endpoint)
		return nil, err
	}
	defer resp.Body.Close()

	// Record success/failure for the endpoint's dependency-class circuit breaker
	if IsErrorResponse(resp.StatusCode) {
		h.recordFailure(endpoint)
	} else {
		h.recordSuccess(endpoint)
		h.invalidateUnreadProjectionOnMutation(userID, endpoint, resp.StatusCode)
	}

	// Read response body
	respBody, _ := io.ReadAll(resp.Body)

	// Cache successful responses (epoch-fenced when applicable)
	if h.shouldCacheResponse(r.Method, endpoint, resp.StatusCode) {
		h.cacheResponse(userID, endpoint, body, respBody, resp.StatusCode, resp.Header, cacheEpoch, fenceCache)
	}

	return &DedupResult{
		Body:       respBody,
		StatusCode: resp.StatusCode,
		Headers:    resp.Header.Clone(),
	}, nil
}

// forwardRequest forwards a request without deduplication.
func (h *BFFHandler) forwardRequest(w http.ResponseWriter, r *http.Request, userID, endpoint string, body []byte, token, requestID string) {
	result, err := h.executeRequest(r, userID, endpoint, body, token, requestID)
	if err != nil {
		h.handleBackendError(w, err, requestID)
		return
	}

	if result != nil {
		h.writeResult(w, result)
	}
}

// shouldUseCache checks if caching should be used for this request.
func (h *BFFHandler) shouldUseCache(method, endpoint string) bool {
	if !h.config.EnableCache || h.responseCache == nil || h.cacheConfig == nil {
		return false
	}
	// Only cache GET and POST (Connect-RPC uses POST for unary)
	if method != http.MethodGet && method != http.MethodPost {
		return false
	}
	return h.cacheConfig.IsCacheable(endpoint)
}

// checkCache checks if a response is cached.
func (h *BFFHandler) checkCache(userID, endpoint string, body []byte) *cache.CacheEntry {
	if h.responseCache == nil {
		return nil
	}

	key := cache.BuildCacheKey(userID, endpoint, body)
	entry, found := h.responseCache.Get(key)
	if found {
		return entry
	}
	return nil
}

// writeCachedResponse writes a cached response.
func (h *BFFHandler) writeCachedResponse(w http.ResponseWriter, entry *cache.CacheEntry) {
	// Copy headers
	for k, v := range entry.Headers {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}
	w.Header().Set("X-Cache", "HIT")
	w.WriteHeader(entry.StatusCode)
	w.Write(entry.Response)
}

// shouldCacheResponse checks if a response should be cached.
func (h *BFFHandler) shouldCacheResponse(method, endpoint string, statusCode int) bool {
	if !h.config.EnableCache || h.cacheConfig == nil {
		return false
	}
	// Only cache successful responses
	if statusCode < 200 || statusCode >= 300 {
		return false
	}
	return h.cacheConfig.IsCacheable(endpoint)
}

// cacheResponse stores a response in the cache.
// When fence is true, SetIfEpoch refuses the write if a mutation bumped the
// user's epoch after this request started (in-flight unread refill race).
func (h *BFFHandler) cacheResponse(userID, endpoint string, reqBody, respBody []byte, statusCode int, headers http.Header, epoch uint64, fence bool) {
	if h.responseCache == nil || h.cacheConfig == nil {
		return
	}

	ttl := h.cacheConfig.GetTTL(endpoint)
	if ttl == 0 {
		return
	}

	key := cache.BuildCacheKey(userID, endpoint, reqBody)
	entry := &cache.CacheEntry{
		Response:   respBody,
		StatusCode: statusCode,
		Headers:    headers.Clone(),
		CachedAt:   time.Now(),
		TTL:        ttl,
	}
	if fence {
		h.responseCache.SetIfEpoch(userID, key, entry, epoch)
		return
	}
	h.responseCache.Set(key, entry)
}

// writeResult writes a dedup result to the response.
func (h *BFFHandler) writeResult(w http.ResponseWriter, result *DedupResult) {
	for k, v := range result.Headers {
		for _, vv := range v {
			w.Header().Add(k, vv)
		}
	}
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(result.StatusCode)
	w.Write(result.Body)
}

// handleError handles an error response.
func (h *BFFHandler) handleError(w http.ResponseWriter, statusCode int, message, requestID string) {
	if h.config.EnableErrorNormalization {
		resp := &http.Response{
			StatusCode: statusCode,
			Header:     make(http.Header),
		}
		normalized := NormalizeError(resp, requestID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		jsonBytes, _ := normalized.ToJSON()
		w.Write(jsonBytes)
		return
	}

	http.Error(w, message, statusCode)
}

// handleBackendError handles a backend error.
func (h *BFFHandler) handleBackendError(w http.ResponseWriter, err error, requestID string) {
	if h.config.EnableErrorNormalization {
		normalized := NormalizeNetworkError(err.Error(), requestID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		jsonBytes, _ := normalized.ToJSON()
		w.Write(jsonBytes)
		return
	}

	h.logError("backend request failed", err)
	http.Error(w, "Bad Gateway", http.StatusBadGateway)
}

// handleCircuitOpen handles a circuit open error.
func (h *BFFHandler) handleCircuitOpen(w http.ResponseWriter, requestID string) {
	if h.config.EnableErrorNormalization {
		normalized := &NormalizedError{
			Code:        CodeServiceUnavailable,
			Message:     "Service temporarily unavailable due to circuit breaker",
			IsRetryable: true,
			RetryAfter:  int(h.config.CBOpenTimeout.Seconds()),
			RequestID:   requestID,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		jsonBytes, _ := normalized.ToJSON()
		w.Write(jsonBytes)
		return
	}

	http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
}

// breakerFor returns the circuit breaker for the endpoint's dependency class.
func (h *BFFHandler) breakerFor(endpoint string) *resilience.CircuitBreaker {
	switch resilience.ClassForEndpoint(endpoint) {
	case resilience.ClassCriticalMutation:
		return h.mutationCB
	case resilience.ClassUnreadProjection:
		return h.projectionCB
	default:
		return h.nonCriticalCB
	}
}

// recordSuccess records a successful request for the endpoint's circuit breaker.
func (h *BFFHandler) recordSuccess(endpoint string) {
	if cb := h.breakerFor(endpoint); cb != nil {
		cb.RecordSuccess()
	}
}

// recordFailure records a failed request for the endpoint's circuit breaker.
func (h *BFFHandler) recordFailure(endpoint string) {
	if cb := h.breakerFor(endpoint); cb != nil {
		cb.RecordFailure()
	}
}

// invalidateUnreadProjectionOnMutation drops cached Unread Projection Reads
// for the user after a successful Critical Feed Mutation (read-your-writes).
func (h *BFFHandler) invalidateUnreadProjectionOnMutation(userID, endpoint string, statusCode int) {
	if h.responseCache == nil || !resilience.IsCriticalFeedMutation(endpoint) {
		return
	}
	if statusCode < 200 || statusCode >= 300 {
		return
	}
	h.responseCache.DeleteByUserAndEndpoints(userID, resilience.UnreadProjectionEndpoints())
}

// logError logs an error with context.
func (h *BFFHandler) logError(msg string, err error) {
	if h.logger != nil {
		h.logger.Error(msg, "error", err)
	}
}

// generateRequestID generates a unique request ID.
func generateRequestID() string {
	return uuid.New().String()
}

// GetCacheStats returns cache statistics.
func (h *BFFHandler) GetCacheStats() *cache.CacheStats {
	if h.responseCache == nil {
		return nil
	}
	stats := h.responseCache.Stats()
	return &stats
}

// GetCircuitBreakerStats returns a worst-of rollup across classes for legacy
// consumers of top-level /stats.state. Prefer GetCircuitBreakerClassStats.
func (h *BFFHandler) GetCircuitBreakerStats() *resilience.CircuitBreakerStats {
	class := h.GetCircuitBreakerClassStats()
	if class == nil {
		return nil
	}
	rollup := resilience.CircuitBreakerStats{State: resilience.StateClosed}
	for _, s := range []*resilience.CircuitBreakerStats{class.Mutation, class.Projection, class.NonCritical} {
		if s == nil {
			continue
		}
		rollup.TotalSuccesses += s.TotalSuccesses
		rollup.TotalFailures += s.TotalFailures
		switch {
		case s.State == resilience.StateOpen:
			rollup.State = resilience.StateOpen
		case s.State == resilience.StateHalfOpen && rollup.State != resilience.StateOpen:
			rollup.State = resilience.StateHalfOpen
		}
	}
	return &rollup
}

// CircuitBreakerClassStats holds per-class circuit breaker statistics.
type CircuitBreakerClassStats struct {
	Mutation    *resilience.CircuitBreakerStats
	Projection  *resilience.CircuitBreakerStats
	NonCritical *resilience.CircuitBreakerStats
}

// GetCircuitBreakerClassStats returns mutation / projection / non-critical CB stats.
func (h *BFFHandler) GetCircuitBreakerClassStats() *CircuitBreakerClassStats {
	if h.mutationCB == nil && h.projectionCB == nil && h.nonCriticalCB == nil {
		return nil
	}
	out := &CircuitBreakerClassStats{}
	if h.mutationCB != nil {
		s := h.mutationCB.Stats()
		out.Mutation = &s
	}
	if h.projectionCB != nil {
		s := h.projectionCB.Stats()
		out.Projection = &s
	}
	if h.nonCriticalCB != nil {
		s := h.nonCriticalCB.Stats()
		out.NonCritical = &s
	}
	return out
}
