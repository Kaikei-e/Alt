// Package handler provides HTTP handlers for the BFF service.
package handler

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"time"

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
	proxyHandler    *ProxyHandler
	backendClient   *client.BackendClient
	authInterceptor *middleware.AuthInterceptor
	logger          *slog.Logger
	config          BFFConfig

	// BFF components
	responseCache   *cache.ResponseCache
	cacheConfig     *cache.CacheConfig
	circuitBreaker  *resilience.CircuitBreaker
	deduplicator    *RequestDeduplicator
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
		proxyHandler: NewProxyHandler(backendClient, secret, issuer, audience, logger),
		backendClient:   backendClient,
		authInterceptor: middleware.NewAuthInterceptor(logger, secret, issuer, audience),
		logger:          logger,
		config:          config,
	}

	// Initialize cache if enabled
	if config.EnableCache {
		h.responseCache = cache.NewResponseCache(config.CacheMaxSize)
		h.cacheConfig = cache.NewCacheConfig()
	}

	// Initialize circuit breaker if enabled
	if config.EnableCircuitBreaker {
		h.circuitBreaker = resilience.NewCircuitBreaker(resilience.CircuitBreakerConfig{
			FailureThreshold: config.CBFailureThreshold,
			SuccessThreshold: config.CBSuccessThreshold,
			OpenTimeout:      config.CBOpenTimeout,
		})
	}

	// Initialize deduplicator if enabled
	if config.EnableDedup {
		h.deduplicator = NewRequestDeduplicator(config.DedupWindow)
	}

	return h
}

// ServeHTTP implements http.Handler.
func (h *BFFHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	// Check circuit breaker
	if h.circuitBreaker != nil && !h.circuitBreaker.Allow() {
		h.handleCircuitOpen(w, requestID)
		return
	}

	// Check cache for cacheable endpoints
	if h.shouldUseCache(r.Method, endpoint) {
		if cached := h.checkCache(userID, endpoint, r); cached != nil {
			h.writeCachedResponse(w, cached)
			return
		}
	}

	// Read request body for dedup key and caching
	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
		r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(body))
	}

	// Handle request with deduplication if enabled
	if h.deduplicator != nil && r.Method == http.MethodPost {
		h.handleWithDedup(w, r, userID, endpoint, body, token, requestID)
		return
	}

	// Forward request directly
	h.forwardRequest(w, r, userID, endpoint, body, token, requestID)
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
		h.recordFailure()
		return nil, err
	}
	defer resp.Body.Close()

	// Record success/failure for circuit breaker
	if IsErrorResponse(resp.StatusCode) {
		h.recordFailure()
	} else {
		h.recordSuccess()
	}

	// Read response body
	respBody, _ := io.ReadAll(resp.Body)

	// Cache successful responses
	if h.shouldCacheResponse(r.Method, endpoint, resp.StatusCode) {
		h.cacheResponse(userID, endpoint, body, respBody, resp.StatusCode, resp.Header)
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
func (h *BFFHandler) checkCache(userID, endpoint string, r *http.Request) *cache.CacheEntry {
	if h.responseCache == nil {
		return nil
	}

	// Read body for cache key
	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
		r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(body))
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
func (h *BFFHandler) cacheResponse(userID, endpoint string, reqBody, respBody []byte, statusCode int, headers http.Header) {
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

// recordSuccess records a successful request for circuit breaker.
func (h *BFFHandler) recordSuccess() {
	if h.circuitBreaker != nil {
		h.circuitBreaker.RecordSuccess()
	}
}

// recordFailure records a failed request for circuit breaker.
func (h *BFFHandler) recordFailure() {
	if h.circuitBreaker != nil {
		h.circuitBreaker.RecordFailure()
	}
}

// logError logs an error with context.
func (h *BFFHandler) logError(msg string, err error) {
	if h.logger != nil {
		h.logger.Error(msg, "error", err)
	}
}

// generateRequestID generates a unique request ID.
func generateRequestID() string {
	return time.Now().Format("20060102150405.000000")
}

// GetCacheStats returns cache statistics.
func (h *BFFHandler) GetCacheStats() *cache.CacheStats {
	if h.responseCache == nil {
		return nil
	}
	stats := h.responseCache.Stats()
	return &stats
}

// GetCircuitBreakerStats returns circuit breaker statistics.
func (h *BFFHandler) GetCircuitBreakerStats() *resilience.CircuitBreakerStats {
	if h.circuitBreaker == nil {
		return nil
	}
	stats := h.circuitBreaker.Stats()
	return &stats
}
