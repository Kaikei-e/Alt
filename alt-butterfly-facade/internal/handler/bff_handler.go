// Package handler provides HTTP handlers for the BFF service.
package handler

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
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

	// Circuit breaker configuration for the external-content class
	CBExternalContentFailureThreshold int
	CBExternalContentOpenTimeout      time.Duration

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
	responseCache     *cache.ResponseCache
	cacheConfig       *cache.CacheConfig
	mutationCB        *resilience.CircuitBreaker
	projectionCB      *resilience.CircuitBreaker
	nonCritical       *nonCriticalBreakers
	externalContentCB *resilience.CircuitBreaker
	telemetryHealth   *resilience.TelemetryHealth
	deduplicator      *RequestDeduplicator
}

// nonCriticalBreakers lazily creates one circuit breaker per endpoint in the
// default (ClassNonCritical) dependency class, all built from the same
// threshold config. ClassNonCritical is not a semantic grouping the way the
// other classes are -- it is every endpoint nobody has classified yet -- so a
// single shared instance here let five 500s on any unclassified endpoint
// blackout every other one, KnowledgeHome / Search / FetchArticlesByTag
// included (adversarial review). Per-endpoint isolation fixes that by
// construction: no enumeration of "which endpoints are related" is needed,
// because none of them share anything.
type nonCriticalBreakers struct {
	mu         sync.Mutex
	cfg        resilience.CircuitBreakerConfig
	byEndpoint map[string]*resilience.CircuitBreaker
}

func newNonCriticalBreakers(cfg resilience.CircuitBreakerConfig) *nonCriticalBreakers {
	return &nonCriticalBreakers{cfg: cfg, byEndpoint: make(map[string]*resilience.CircuitBreaker)}
}

// forEndpoint returns endpoint's breaker, creating it on first use.
func (n *nonCriticalBreakers) forEndpoint(endpoint string) *resilience.CircuitBreaker {
	n.mu.Lock()
	defer n.mu.Unlock()

	if cb, ok := n.byEndpoint[endpoint]; ok {
		return cb
	}
	cb := resilience.NewCircuitBreaker(n.cfg)
	n.byEndpoint[endpoint] = cb
	return cb
}

// stats rolls every endpoint's breaker up into one summary for
// /v1/bff/stats: State is worst-of (Open beats HalfOpen beats Closed) so an
// operator sees "something in this class is degraded" at a glance, and the
// totals are straight sums. An endpoint never queried yet is absent from
// byEndpoint and so does not exist for this rollup -- consistent with it
// having made zero requests.
func (n *nonCriticalBreakers) stats() resilience.CircuitBreakerStats {
	n.mu.Lock()
	defer n.mu.Unlock()

	rollup := resilience.CircuitBreakerStats{State: resilience.StateClosed}
	for _, cb := range n.byEndpoint {
		s := cb.Stats()
		rollup.TotalSuccesses += s.TotalSuccesses
		rollup.TotalFailures += s.TotalFailures
		rollup.TotalRejections += s.TotalRejections
		switch {
		case s.State == resilience.StateOpen:
			rollup.State = resilience.StateOpen
		case s.State == resilience.StateHalfOpen && rollup.State != resilience.StateOpen:
			rollup.State = resilience.StateHalfOpen
		}
	}
	return rollup
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
			OnTransition:     h.logCircuitTransition,
		}
		cbCfg.Class = resilience.ClassCriticalMutation
		h.mutationCB = resilience.NewCircuitBreaker(cbCfg)
		cbCfg.Class = resilience.ClassUnreadProjection
		h.projectionCB = resilience.NewCircuitBreaker(cbCfg)
		cbCfg.Class = resilience.ClassNonCritical
		h.nonCritical = newNonCriticalBreakers(cbCfg)
		// The external-content class talks to the open internet, so it gets a
		// looser budget than an internal dependency: a rate-limited or
		// unreachable publisher is not an alt-backend outage, and it re-probes
		// sooner because the next article is usually a different domain.
		h.externalContentCB = resilience.NewCircuitBreaker(resilience.CircuitBreakerConfig{
			FailureThreshold: config.CBExternalContentFailureThreshold,
			SuccessThreshold: config.CBSuccessThreshold,
			OpenTimeout:      config.CBExternalContentOpenTimeout,
			Class:            resilience.ClassExternalContent,
			OnTransition:     h.logCircuitTransition,
		})
		// ClassTelemetry is exempt from gating (breakerFor returns nil for
		// it below), but exempt from gating must not mean silent:
		// DegradedThreshold reuses the same failure budget as a reporting
		// cadence rather than an admission one.
		h.telemetryHealth = resilience.NewTelemetryHealth(resilience.TelemetryHealthConfig{
			DegradedThreshold: config.CBFailureThreshold,
			OnChange:          h.logTelemetryHealth,
		})
		if logger != nil {
			logger.Info("circuit_breaker_classes",
				"critical_mutation", resilience.CriticalMutationEndpointCount(),
				"unread_projection", resilience.UnreadProjectionEndpointCount(),
				"external_content", resilience.ExternalContentEndpointCount(),
				"telemetry_exempt", resilience.TelemetryEndpointCount(),
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
		permit, allowed := cb.Acquire(endpoint)
		if !allowed {
			h.handleCircuitOpen(w, endpoint, requestID)
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
		permit, allowed := cb.Acquire(endpoint)
		if !allowed {
			h.handleCircuitOpen(w, endpoint, requestID)
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

	h.recordOutcome(endpoint, resp.StatusCode, resp.Header)

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

	// Read the response body before charging the breaker. A body that ends
	// mid-stream is a failed response whatever its status line said, and
	// recording the outcome first got that backwards twice over: the breaker
	// banked a success (so a backend dying halfway through every response
	// never trips it, and a half-open trial passes on a torn body) and the
	// truncated bytes went on to be cached, handing every caller a parse
	// error behind X-Cache: HIT for the rest of the endpoint's TTL.
	respBody, err := io.ReadAll(resp.Body)

	// Invalidation stays keyed on the status line: a 2xx means the mutation
	// already landed upstream, so read-your-writes has to hold even when the
	// response carrying it was cut short.
	h.invalidateUnreadProjectionOnMutation(userID, endpoint, resp.StatusCode)

	if err != nil {
		h.recordFailure(endpoint)
		return nil, fmt.Errorf("read backend response body: %w", err)
	}

	// Record success/failure for the endpoint's dependency-class circuit breaker
	h.recordOutcome(endpoint, resp.StatusCode, resp.Header)

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
	if ct := w.Header().Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
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
	if ct := w.Header().Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
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

	http.Error(w, html.EscapeString(message), statusCode)
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

// handleCircuitOpen rejects a request whose dependency class has an open
// breaker. Unlike a proxied backend error this response is authored by the
// BFF, so it is emitted as a Connect error envelope regardless of the
// normalization flag: the caller is always a Connect client, and only that
// shape survives its parser. Retry guidance rides the Retry-After header
// because Connect has no wire slot for it in the error body, and it carries
// the rejecting class's own open timeout — external content re-probes sooner
// than the internal classes.
func (h *BFFHandler) handleCircuitOpen(w http.ResponseWriter, endpoint, requestID string) {
	connectErr := &ConnectError{
		Code:    ConnectCodeUnavailable,
		Message: "Service temporarily unavailable due to circuit breaker",
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", strconv.Itoa(int(h.openTimeoutFor(endpoint).Seconds())))
	// The one condition that really does reach every host: a breaker rejection
	// is a state of this gateway, and no publisher can be routed around it.
	// Declaring it lets the client stop inferring the same thing from a bare
	// CodeUnavailable, which a single dead article also produces.
	w.Header().Set(FailureScopeHeader, FailureScopeGlobal)
	w.Header().Set("X-Request-Id", requestID)
	w.WriteHeader(http.StatusServiceUnavailable)
	jsonBytes, _ := connectErr.ToJSON()
	w.Write(jsonBytes)
}

// breakerFor returns the circuit breaker for the endpoint's dependency class.
// ClassTelemetry deliberately has none: those RPCs are fire-and-forget writes
// whose result the caller discards, so a nil breaker here means their
// failures never charge a shared budget and no other class's open breaker
// ever blocks them either (their outcomes are still counted and logged, see
// telemetryHealth / logTelemetryHealth). The default (ClassNonCritical)
// branch hands back that endpoint's own breaker, never a shared one — see
// nonCriticalBreakers.
func (h *BFFHandler) breakerFor(endpoint string) *resilience.CircuitBreaker {
	switch resilience.ClassForEndpoint(endpoint) {
	case resilience.ClassCriticalMutation:
		return h.mutationCB
	case resilience.ClassUnreadProjection:
		return h.projectionCB
	case resilience.ClassExternalContent:
		return h.externalContentCB
	case resilience.ClassTelemetry:
		return nil
	default:
		if h.nonCritical == nil {
			return nil
		}
		return h.nonCritical.forEndpoint(endpoint)
	}
}

// openTimeoutFor is the open-state duration of the endpoint's dependency-class
// breaker, used as the client's back-off hint.
func (h *BFFHandler) openTimeoutFor(endpoint string) time.Duration {
	if resilience.ClassForEndpoint(endpoint) == resilience.ClassExternalContent {
		return h.config.CBExternalContentOpenTimeout
	}
	return h.config.CBOpenTimeout
}

// recordOutcome charges the endpoint's dependency-class breaker for a backend
// response. A 4xx is deliberately neither: recorded as a failure, one
// rate-limited or paywalled publisher opens the breaker for every endpoint in
// the class; recorded as a success, it resets consecFailures and masks a real
// backend outage.
//
// A 5xx the backend attributed to a publisher gets that same neutrality, and
// for the same reason. The status code alone cannot tell "the site never
// answered" from "alt-backend fell over" — both are 503 — so without the
// attribution header the commonest failure a feed reader meets was charged to
// alt-backend's budget, which is the budget ADR-000959 set out to protect.
func (h *BFFHandler) recordOutcome(endpoint string, statusCode int, respHeader http.Header) {
	switch {
	case IsDependencyFailure(statusCode) && !IsUpstreamAttributed(respHeader):
		h.recordFailure(endpoint)
	case !IsErrorResponse(statusCode):
		h.recordSuccess(endpoint)
	}
}

// recordSuccess records a successful request for the endpoint's circuit
// breaker, or — for ClassTelemetry, which has no breaker — for telemetryHealth.
func (h *BFFHandler) recordSuccess(endpoint string) {
	if resilience.ClassForEndpoint(endpoint) == resilience.ClassTelemetry {
		if h.telemetryHealth != nil {
			h.telemetryHealth.RecordSuccess(endpoint)
		}
		return
	}
	if cb := h.breakerFor(endpoint); cb != nil {
		cb.RecordSuccess(endpoint)
	}
}

// recordFailure records a failed request for the endpoint's circuit breaker,
// or — for ClassTelemetry, which has no breaker — for telemetryHealth. This is
// the fix for the class being otherwise completely silent: exempt from
// gating must not mean exempt from observability (adversarial review).
func (h *BFFHandler) recordFailure(endpoint string) {
	if resilience.ClassForEndpoint(endpoint) == resilience.ClassTelemetry {
		if h.telemetryHealth != nil {
			h.telemetryHealth.RecordFailure(endpoint)
		}
		return
	}
	if cb := h.breakerFor(endpoint); cb != nil {
		cb.RecordFailure(endpoint)
	}
}

// logCircuitTransition is the breakers' transition observer. The breaker
// package stays free of a logging dependency: it hands over a value describing
// the change, captured under its own lock and delivered after that lock is
// released, and this is where it becomes a log line.
//
// Rejections are deliberately not logged one by one — an open breaker refuses
// every request in its class, which during the incident that motivated this
// would have been thousands of lines a minute. The count rides
// rejected_since_last_report on the next transition instead, so the volume of
// user-visible 503s is still recorded, bounded to two lines per OpenTimeout
// (the probe and the trial that fails behind it) while a dependency stays down.
func (h *BFFHandler) logCircuitTransition(t resilience.StateTransition) {
	if h.logger == nil {
		return
	}

	attrs := []any{
		"transition_seq", t.Seq,
		"class", t.Class.String(),
		"endpoint", t.Endpoint,
		"from", t.From.String(),
		"to", t.To.String(),
		"consecutive_failures", t.ConsecFailures,
		"consecutive_successes", t.ConsecSuccesses,
		"failure_threshold", t.FailureThreshold,
		"success_threshold", t.SuccessThreshold,
		"open_timeout_seconds", t.OpenTimeout.Seconds(),
		"rejected_since_last_report", t.RejectedSinceLastReport,
	}

	// OPEN is user-visible degradation: every request in the class is being
	// refused from here until the breaker re-probes.
	if t.To == resilience.StateOpen {
		h.logger.Warn("circuit_breaker_transition", attrs...)
		return
	}
	h.logger.Info("circuit_breaker_transition", attrs...)
}

// logTelemetryHealth is telemetryHealth's OnChange observer, ClassTelemetry's
// counterpart to logCircuitTransition. It fires only on the two edges an
// operator needs to act on — an endpoint crossing into degraded, and its
// first success after — not on every call, so a 100%-failing endpoint still
// logs once instead of flooding.
func (h *BFFHandler) logTelemetryHealth(o resilience.TelemetryOutcome) {
	if h.logger == nil {
		return
	}

	attrs := []any{
		"endpoint", o.Endpoint,
		"consecutive_failures", o.ConsecFailures,
		"total_failures", o.TotalFailures,
		"total_successes", o.TotalSuccesses,
	}

	if o.Degraded {
		h.logger.Warn("telemetry_endpoint_degraded", attrs...)
		return
	}
	h.logger.Info("telemetry_endpoint_recovered", attrs...)
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
	for _, s := range []*resilience.CircuitBreakerStats{class.Mutation, class.Projection, class.NonCritical, class.ExternalContent} {
		if s == nil {
			continue
		}
		rollup.TotalSuccesses += s.TotalSuccesses
		rollup.TotalFailures += s.TotalFailures
		rollup.TotalRejections += s.TotalRejections
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
	Mutation        *resilience.CircuitBreakerStats
	Projection      *resilience.CircuitBreakerStats
	NonCritical     *resilience.CircuitBreakerStats
	ExternalContent *resilience.CircuitBreakerStats
	// Telemetry has no CircuitState -- ClassTelemetry endpoints are never
	// gated -- so it is a separate, simpler shape from the other classes.
	Telemetry *resilience.TelemetryHealthStats
}

// GetCircuitBreakerClassStats returns per-dependency-class CB stats.
func (h *BFFHandler) GetCircuitBreakerClassStats() *CircuitBreakerClassStats {
	if h.mutationCB == nil && h.projectionCB == nil && h.nonCritical == nil && h.externalContentCB == nil && h.telemetryHealth == nil {
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
	if h.nonCritical != nil {
		s := h.nonCritical.stats()
		out.NonCritical = &s
	}
	if h.externalContentCB != nil {
		s := h.externalContentCB.Stats()
		out.ExternalContent = &s
	}
	if h.telemetryHealth != nil {
		s := h.telemetryHealth.Stats()
		out.Telemetry = &s
	}
	return out
}
