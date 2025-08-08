// Package proxy implements the core HTTP proxy functionality for the lightweight sidecar
// This package contains the critical upstream resolution logic described in ISSUE_RESOLVE_PLAN.md
// to transform upstream="10.96.32.212:8080" into upstream="zenn.dev:443"
package proxy

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/alt-rss/alt-backend/sidecar-proxy/pkg/autolearn"
	"github.com/alt-rss/alt-backend/sidecar-proxy/pkg/config"
	"github.com/alt-rss/alt-backend/sidecar-proxy/pkg/dns"
	"github.com/alt-rss/alt-backend/sidecar-proxy/pkg/metrics"
)

// LightweightProxy represents the main proxy sidecar implementation
// This struct contains all necessary components for the upstream resolution solution
type LightweightProxy struct {
	config      *config.ProxyConfig
	httpClient  *http.Client
	dnsResolver *dns.ExternalDNSResolver
	logger      *log.Logger
	server      *http.Server
	metrics     *metrics.Collector

	// 自動学習機能
	autoLearner *autolearn.AutoLearner

	// オンメモリDNS管理: 動的ドメイン解決システム
	dynamicDNS *dns.DynamicResolver

	// Request processing state
	shutdownChan chan struct{}
	ready        bool
}

// RequestContext holds context information for each proxy request
// This enables detailed logging and tracing as specified in the plan
type RequestContext struct {
	TraceID       string    `json:"trace_id"`
	StartTime     time.Time `json:"start_time"`
	TargetURL     *url.URL  `json:"target_url"`
	ResolvedIP    net.IP    `json:"resolved_ip"`
	Method        string    `json:"method"`
	UserAgent     string    `json:"user_agent"`
	ContentLength int64     `json:"content_length"`
}

// ProxyResponse contains the response details for logging and metrics
type ProxyResponse struct {
	StatusCode    int           `json:"status_code"`
	ContentLength int64         `json:"content_length"`
	Duration      time.Duration `json:"duration"`
	DNSTime       time.Duration `json:"dns_time"`
	ProxyTime     time.Duration `json:"proxy_time"`
	UpstreamHost  string        `json:"upstream_host"` // This is the key field for solving the upstream problem
}

// NewLightweightProxy creates a new proxy instance with optimized configuration
// This constructor implements the architecture described in ISSUE_RESOLVE_PLAN.md
func NewLightweightProxy(cfg *config.ProxyConfig) (*LightweightProxy, error) {
	// Create optimized HTTP transport for Envoy communication
	transport := &http.Transport{
		MaxIdleConns:          cfg.EnvoyMaxConns,
		MaxConnsPerHost:       cfg.EnvoyMaxConns,
		MaxIdleConnsPerHost:   cfg.EnvoyMaxIdleConns,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,

		// TLS configuration for secure upstream connections
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false, // Always verify certificates in production
			MinVersion:         tls.VersionTLS12,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			},
		},
		DisableCompression: false,
		ForceAttemptHTTP2:  true,

		// Custom dialer for connection control
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}

	// HTTP client with configured timeouts
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   cfg.EnvoyTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	// Initialize external DNS resolver (key component for upstream resolution)
	dnsResolver := dns.NewExternalDNSResolver(
		cfg.DNSServers,
		cfg.DNSCacheTimeout,
		cfg.DNSMaxCacheEntries,
	)
	dnsResolver.SetTimeout(cfg.DNSTimeout)

	// Setup structured logging
	logger := log.New(os.Stdout, "[ProxySidecar] ", log.LstdFlags|log.Lshortfile)

	// Initialize metrics collector
	metricsCollector := metrics.NewCollector("proxy_sidecar")

	// Initialize auto-learner for transparent domain learning (in-memory)
	autoLearnerConfig := &autolearn.Config{
		MaxDomains:       1000,
		LearningEnabled:  true,
		SecurityLevel:    "moderate",
		RateLimitPerHour: 50,
		CooldownMinutes:  10,
	}

	autoLearner, err := autolearn.NewAutoLearner(autoLearnerConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize auto-learner: %w", err)
	}

	// Initialize dynamic DNS resolver for on-memory domain management
	// オンメモリDNS管理システムの初期化
	dynamicDNS := dns.NewDynamicResolver(
		cfg.AllowedDomains,     // Static domain patterns
		cfg.DNSServers,         // DNS servers for resolution
		cfg.DNSCacheTimeout,    // Cache timeout
		cfg.DNSMaxCacheEntries, // Max cache entries
	)

	proxy := &LightweightProxy{
		config:       cfg,
		httpClient:   httpClient,
		dnsResolver:  dnsResolver,
		logger:       logger,
		metrics:      metricsCollector,
		autoLearner:  autoLearner,
		dynamicDNS:   dynamicDNS,
		shutdownChan: make(chan struct{}),
		ready:        false,
	}

	return proxy, nil
}

// Start begins the proxy server with graceful shutdown support
// This implements the server lifecycle management from ISSUE_RESOLVE_PLAN.md
func (p *LightweightProxy) Start() error {
	// XPLAN7.md Web検索修正: ServeMux回避でURL正規化問題解決
	// GoのServeMuxが"/proxy/https:/"を"/proxy/https/"に正規化する問題回避
	customHandler := http.HandlerFunc(p.handleRawRequest)

	// Configure HTTP server with production settings
	p.server = &http.Server{
		Addr:         ":" + p.config.ListenPort,
		Handler:      customHandler,
		ReadTimeout:  p.config.ReadTimeout,
		WriteTimeout: p.config.WriteTimeout,
		IdleTimeout:  p.config.IdleTimeout,
		ErrorLog:     p.logger,
	}

	// Setup graceful shutdown
	go p.setupGracefulShutdown()

	p.ready = true
	p.logger.Printf("Starting lightweight proxy sidecar on port %s", p.config.ListenPort)
	p.logger.Printf("Envoy upstream: %s", p.config.EnvoyUpstream)
	p.logger.Printf("DNS servers: %v", p.config.DNSServers)
	p.logger.Printf("Allowed domains: %v", p.config.AllowedDomainsRaw)

	return p.server.ListenAndServe()
}

// handleRawRequest handles raw HTTP requests with security hardening
// XPLAN7.md Web検索セキュリティ修正: ServeMux回避でパストラバーサル対策実装
func (p *LightweightProxy) handleRawRequest(w http.ResponseWriter, r *http.Request) {
	// 🚨 セキュリティ強化: CVE-2019-16276対策 - 不正なHTTPメソッド拒否
	// CONNECT追加: HTTPS tunneling (OLLAMA model download) サポート
	allowedMethods := map[string]bool{
		"GET": true, "POST": true, "PUT": true, "DELETE": true, "HEAD": true, "OPTIONS": true, "CONNECT": true,
	}
	if !allowedMethods[r.Method] {
		p.logger.Printf("Security: Blocked disallowed HTTP method: %s", r.Method)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// 🛡️ Path Traversal対策: 安全なパス正規化
	originalPath := r.URL.Path
	if r.RequestURI != "" {
		if u, err := url.Parse(r.RequestURI); err == nil {
			originalPath = u.Path
		}
	}

	// URLパス正規化でセキュリティ確保（Web検索推奨手法）
	// net/urlパッケージでURLパスを安全に処理
	cleanPath := "/" + strings.TrimPrefix(originalPath, "/")
	if parsedURL, err := url.Parse(cleanPath); err == nil {
		cleanPath = parsedURL.Path
	}

	// 🚨 セキュリティ検証: パストラバーサル攻撃検出（https://は除外）
	if strings.Contains(originalPath, "..") ||
		strings.Contains(originalPath, "\\") ||
		(strings.Contains(originalPath, "//") && !strings.Contains(originalPath, "://")) {
		p.logger.Printf("Security: Path traversal attempt blocked: %s", originalPath)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// XPLAN7.md 特別処理: /proxy/https:// パターンのみダブルスラッシュを復元
	if strings.HasPrefix(cleanPath, "/proxy/https:/") &&
		!strings.HasPrefix(cleanPath, "/proxy/https://") {
		// セキュアなhttps://復元（特定パターンのみ）
		cleanPath = strings.Replace(cleanPath, "/proxy/https:/", "/proxy/https://", 1)
		p.logger.Printf("Security: Safe HTTPS URL restoration: %s", cleanPath)
	}

	// 🆕 責務分離: パス別処理振り分け（アーキテクチャ統一戦略）
	switch {
	case strings.HasPrefix(cleanPath, "/proxy/https://"):
		// RSS取得系: 単発HTTP通信 (既存処理)
		p.HandleProxyRequest(w, r)
		return

	case strings.HasPrefix(cleanPath, "/connect/"):
		// Model Download系: 持続的TCP通信 (新規処理)
		p.HandlePersistentTunnelRequest(w, r)
		return

	case r.Method == "CONNECT":
		// オンメモリDNS管理: 動的ドメイン許可システム
		p.HandleDynamicCONNECTRequest(w, r)
		return
	}

	// セキュアなルーティング
	switch {
	case strings.HasPrefix(cleanPath, "/proxy/"):
		r.URL.Path = cleanPath
		p.HandleProxyRequest(w, r)
	case cleanPath == "/health":
		p.HandleHealthCheck(w, r)
	case cleanPath == "/ready":
		p.HandleReadinessCheck(w, r)
	case cleanPath == "/metrics":
		p.HandleMetrics(w, r)
	case cleanPath == "/debug/dns":
		p.HandleDNSDebug(w, r)
	case cleanPath == "/debug/config":
		p.HandleConfigDebug(w, r)
	case strings.HasPrefix(cleanPath, "/admin/domains"):
		p.HandleAutoLearnAdmin(w, r)
	case cleanPath == "/admin/metrics":
		p.HandleAutoLearnMetrics(w, r)
	default:
		http.NotFound(w, r)
	}
}

// HandleProxyRequest is the core function that solves the upstream resolution problem
// This function implements the critical logic described in ISSUE_RESOLVE_PLAN.md
func (p *LightweightProxy) HandleProxyRequest(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	// Generate trace ID for request tracking
	traceID := p.generateTraceID()

	p.logger.Printf("[%s] Processing proxy request: %s %s", traceID, r.Method, r.URL.Path)

	// Extract target URL from request path (e.g., /proxy/https://zenn.dev/feed)
	targetURL, err := p.extractTargetURL(r.URL.Path)
	if err != nil {
		p.writeErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid target URL: %v", err), traceID)
		return
	}

	p.logger.Printf("[%s] Target URL extracted: %s", traceID, targetURL.String())

	// 🎯 CRITICAL: Transparent auto-learning domain validation
	domain := extractDomainFromURL(targetURL.String())

	// Check static allowlist first (fastest path)
	if !p.config.IsDomainAllowed(domain) {
		// Check auto-learned domains (second fastest path)
		if !p.autoLearner.IsAllowed(domain) {
			// 🧠 TRANSPARENT AUTO-LEARNING: Learn new domain
			if err := p.autoLearner.LearnDomain(domain, targetURL.String(), traceID); err != nil {
				p.logger.Printf("[%s] 🚫 Auto-learning failed for domain %s: %v", traceID, domain, err)
				p.writeErrorResponse(w, http.StatusForbidden,
					fmt.Sprintf("Domain learning failed: %v", err), traceID)
				return
			}
			p.logger.Printf("[%s] 🧠 Auto-learned new domain: %s", traceID, domain)
		}
	}

	// 🌐 CRITICAL: External DNS resolution to bypass Kubernetes internal DNS
	// This is the key step that enables proper upstream resolution
	dnsStartTime := time.Now()
	resolvedIPs, err := p.dnsResolver.ResolveExternal(r.Context(), targetURL.Host)
	dnsTime := time.Since(dnsStartTime)

	if err != nil {
		p.logger.Printf("[%s] DNS resolution failed for %s: %v", traceID, targetURL.Host, err)
		p.writeErrorResponse(w, http.StatusBadGateway, fmt.Sprintf("DNS resolution failed: %v", err), traceID)
		return
	}

	p.logger.Printf("[%s] DNS resolved: %s -> %v (took %v)", traceID, targetURL.Host, resolvedIPs, dnsTime)

	// 🔧 CRITICAL: Build Envoy request with proper headers for upstream resolution
	// This is where we set the headers that will make Envoy show "upstream=zenn.dev:443"
	proxyStartTime := time.Now()
	envoyReq, err := p.buildEnvoyRequest(r, targetURL, resolvedIPs[0], traceID)
	if err != nil {
		p.logger.Printf("[%s] Failed to build Envoy request: %v", traceID, err)
		p.writeErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Request building failed: %v", err), traceID)
		return
	}

	// 📡 Execute proxy request to Envoy
	resp, err := p.proxyToEnvoy(r.Context(), envoyReq, traceID)
	proxyTime := time.Since(proxyStartTime)

	if err != nil {
		p.logger.Printf("[%s] Proxy to Envoy failed: %v", traceID, err)
		p.writeErrorResponse(w, http.StatusBadGateway, fmt.Sprintf("Proxy error: %v", err), traceID)
		return
	}
	defer resp.Body.Close()

	// Copy response back to client
	p.copyResponse(w, resp, traceID)

	// 📊 Log the successful upstream resolution (this is what we want to see!)
	totalTime := time.Since(startTime)
	upstreamExpected := targetURL.Host // HTTPS default port 443 should be omitted

	p.logger.Printf("[%s] ✅ UPSTREAM RESOLUTION SUCCESS: target=%s, expected_upstream=%s, dns_time=%v, proxy_time=%v, total_time=%v, status=%d",
		traceID, targetURL.String(), upstreamExpected, dnsTime, proxyTime, totalTime, resp.StatusCode)

	// Update metrics
	p.metrics.RecordRequest(targetURL.Host, resp.StatusCode, totalTime)
}

// buildEnvoyRequest constructs the HTTP request that will be sent to Envoy
// 🎯 This is THE MOST CRITICAL function - it sets the headers that solve the upstream problem
func (p *LightweightProxy) buildEnvoyRequest(originalReq *http.Request, targetURL *url.URL, resolvedIP net.IP, traceID string) (*http.Request, error) {
	// 🚑 REPORT.md オプションB: 正統派Forward Proxy実装
	// 絶対URL + 正しい:authority で DFP自己ループ問題を根本解決

	// Envoy forward proxy URL: /proxy/ パスを保持したまま転送
	envoyProxyURL := fmt.Sprintf("http://%s%s", p.config.EnvoyUpstream, originalReq.URL.Path)
	if originalReq.URL.RawQuery != "" {
		envoyProxyURL += "?" + originalReq.URL.RawQuery
	}

	// Create new request with absolute target URL as the request URL
	req, err := http.NewRequestWithContext(originalReq.Context(), originalReq.Method, envoyProxyURL, originalReq.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Copy original headers first
	for name, values := range originalReq.Header {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}

	// 🎯 THE MAGIC HEADERS: These headers are what will make Envoy show "upstream=zenn.dev:443"
	// instead of "upstream=10.96.32.212:8080"

	// 1. Host header: This is what Envoy Dynamic Forward Proxy uses for routing
	req.Header.Set("Host", targetURL.Host)

	// 2. X-Target-Domain: This works with Envoy's host_rewrite_header setting
	req.Header.Set("X-Target-Domain", targetURL.Host)

	// 3. X-Resolved-IP: For debugging and monitoring
	req.Header.Set("X-Resolved-IP", resolvedIP.String())

	// 4. X-Original-Host: Preserve original request info
	req.Header.Set("X-Original-Host", originalReq.Host)

	// 5. Trace ID for debugging
	req.Header.Set("X-Trace-ID", traceID)

	// 6. Timestamp for performance analysis
	req.Header.Set("X-Proxy-Timestamp", time.Now().Format(time.RFC3339))

	// 7. HTTP/2 :authority pseudo-header for HTTP/2 compatibility
	if req.ProtoMajor >= 2 {
		req.Header.Set(":authority", targetURL.Host)
	}

	// 8. Ensure User-Agent is set
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "lightweight-proxy-sidecar/1.0")
	}

	p.logger.Printf("[%s] 🔧 Built Envoy request with headers: Host=%s, X-Target-Domain=%s, X-Resolved-IP=%s",
		traceID, targetURL.Host, targetURL.Host, resolvedIP.String())

	return req, nil
}

// extractTargetURL parses the proxy request path to extract the target URL
// Implements 2024 security best practices addressing CVE-2024-34155 and SSRF prevention
// Uses Go's net/url standard library without regex for maximum security
func (p *LightweightProxy) extractTargetURL(path string) (*url.URL, error) {
	// Step 1: Validate proxy path format using strict prefix matching
	if !strings.HasPrefix(path, "/proxy/") {
		return nil, fmt.Errorf("invalid proxy path: must start with /proxy/")
	}

	// Step 2: Extract URL string with proper bounds checking
	targetURLStr := strings.TrimPrefix(path, "/proxy/")
	if len(targetURLStr) == 0 {
		return nil, fmt.Errorf("empty target URL")
	}

	// Step 3: CVE-2024-34155 mitigation - prevent stack exhaustion attacks
	// Limit URL length to prevent excessive parsing depth
	const maxURLLength = 1024 // Reduced from 2048 for CVE-2024-34155 protection
	if len(targetURLStr) > maxURLLength {
		return nil, fmt.Errorf("URL too long: maximum %d characters allowed (CVE-2024-34155 protection)", maxURLLength)
	}

	// Step 4: CVE-2024-34155 mitigation - check for deeply nested patterns
	if err := p.checkParsingComplexity(targetURLStr); err != nil {
		return nil, fmt.Errorf("URL complexity validation failed: %w", err)
	}

	// Step 5: Fix URL encoding issues safely without regex
	targetURLStr = p.sanitizeURLString(targetURLStr)

	// Step 6: Use Go's standard net/url.ParseRequestURI for stricter validation
	// ParseRequestURI is more strict than Parse for HTTP requests (2024 best practice)
	targetURL, err := url.ParseRequestURI(targetURLStr)
	if err != nil {
		return nil, fmt.Errorf("invalid URL format (strict parsing): %w", err)
	}

	// Step 7: Validate URL components using allowlist approach (2024 best practice)
	if err := p.validateURLSecurity(targetURL); err != nil {
		return nil, fmt.Errorf("URL security validation failed: %w", err)
	}

	return targetURL, nil
}

// checkParsingComplexity validates URL complexity to prevent CVE-2024-34155 stack exhaustion
// Implements parsing depth limitations as recommended by Go security team
func (p *LightweightProxy) checkParsingComplexity(urlStr string) error {
	// Check for excessive nesting patterns that could trigger stack exhaustion
	const maxNestedPercent = 50      // Maximum percentage of URL that can be percent-encoded
	const maxConsecutiveSlashes = 10 // Maximum consecutive slashes
	const maxConsecutivePercent = 20 // Maximum consecutive percent signs

	// Count percent-encoded characters (CVE-2024-34155 protection)
	percentCount := strings.Count(urlStr, "%")
	if percentCount > len(urlStr)*maxNestedPercent/100 {
		return fmt.Errorf("excessive URL encoding detected: %d percent signs in %d characters", percentCount, len(urlStr))
	}

	// Check for consecutive slashes that could cause parsing issues
	consecutiveSlashes := 0
	maxConsecutiveSlashesFound := 0
	for _, char := range urlStr {
		if char == '/' {
			consecutiveSlashes++
			if consecutiveSlashes > maxConsecutiveSlashesFound {
				maxConsecutiveSlashesFound = consecutiveSlashes
			}
		} else {
			consecutiveSlashes = 0
		}
	}
	if maxConsecutiveSlashesFound > maxConsecutiveSlashes {
		return fmt.Errorf("excessive consecutive slashes detected: %d", maxConsecutiveSlashesFound)
	}

	// Check for consecutive percent signs
	consecutivePercent := 0
	for i := 0; i < len(urlStr); i++ {
		if urlStr[i] == '%' {
			consecutivePercent++
			if consecutivePercent > maxConsecutivePercent {
				return fmt.Errorf("excessive consecutive percent signs detected: %d", consecutivePercent)
			}
		} else {
			consecutivePercent = 0
		}
	}

	return nil
}

// sanitizeURLString performs safe URL string cleanup without regex
// Addresses common URL format issues using string operations only (2024 security best practice)
func (p *LightweightProxy) sanitizeURLString(urlStr string) string {
	// Remove dangerous whitespace characters
	urlStr = strings.TrimSpace(urlStr)

	// Fix single slash issues (https:/ -> https://) using safe string operations
	if strings.HasPrefix(urlStr, "https:/") && !strings.HasPrefix(urlStr, "https://") {
		urlStr = "https://" + strings.TrimPrefix(urlStr, "https:/")
	}
	if strings.HasPrefix(urlStr, "http:/") && !strings.HasPrefix(urlStr, "http://") {
		urlStr = "http://" + strings.TrimPrefix(urlStr, "http:/")
	}

	// Remove control characters that could be used for bypass attempts
	// Using strings.Map for safe character filtering without regex
	urlStr = strings.Map(func(r rune) rune {
		// Allow only printable ASCII characters and common URL characters
		if r >= 32 && r <= 126 {
			return r
		}
		// Remove control characters and non-ASCII characters
		return -1
	}, urlStr)

	return urlStr
}

// validateURLSecurity implements comprehensive URL security validation
// Following OWASP SSRF Prevention guidelines and 2024 CVE mitigations
func (p *LightweightProxy) validateURLSecurity(targetURL *url.URL) error {
	// Security Check 1: Scheme validation (only HTTPS allowed for RSS feeds)
	if targetURL.Scheme != "https" {
		return fmt.Errorf("only HTTPS URLs are allowed for security, got: %s", targetURL.Scheme)
	}

	// Security Check 2: Host validation with enhanced checks
	if targetURL.Host == "" {
		return fmt.Errorf("missing host in URL")
	}

	// Security Check 3: Enhanced host validation for 2024 security threats
	if err := p.validateHostSecurity(targetURL.Host); err != nil {
		return fmt.Errorf("host security validation failed: %w", err)
	}

	// Security Check 4: Domain allowlist validation (OWASP 2024 best practice)
	if !p.config.IsDomainAllowed(targetURL.Host) {
		return fmt.Errorf("domain not in security allowlist: %s", targetURL.Host)
	}

	// Security Check 5: Enhanced path validation
	if err := p.validatePathSecurity(targetURL.Path); err != nil {
		return fmt.Errorf("path security validation failed: %w", err)
	}

	// Security Check 6: Query parameter validation to prevent injection
	if err := p.validateQuerySecurity(targetURL.RawQuery); err != nil {
		return fmt.Errorf("query parameter security validation failed: %w", err)
	}

	// Security Check 7: Fragment validation (XSS prevention)
	if err := p.validateFragmentSecurity(targetURL.Fragment); err != nil {
		return fmt.Errorf("fragment security validation failed: %w", err)
	}

	// Security Check 8: Port validation (prevent bypass attempts)
	if err := p.validatePortSecurity(targetURL.Port()); err != nil {
		return fmt.Errorf("port security validation failed: %w", err)
	}

	return nil
}

// validateHostSecurity prevents SSRF attacks by validating host security
// Implements 2024 best practices for preventing access to internal resources
func (p *LightweightProxy) validateHostSecurity(host string) error {
	// Remove port from host for validation
	hostname := host
	if strings.Contains(host, ":") {
		var err error
		hostname, _, err = net.SplitHostPort(host)
		if err != nil {
			return fmt.Errorf("invalid host format: %w", err)
		}
	}

	// Check for localhost variants
	localhostVariants := []string{
		"localhost", "127.0.0.1", "::1", "0.0.0.0",
		"0", "0x0", "0x00000000", "2130706433", // IPv4 localhost representations
	}
	for _, variant := range localhostVariants {
		if strings.EqualFold(hostname, variant) {
			return fmt.Errorf("localhost access denied for security")
		}
	}

	// Check for private network ranges (RFC 1918)
	if ip := net.ParseIP(hostname); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() {
			return fmt.Errorf("access to private/loopback IP denied: %s", hostname)
		}
	}

	// Additional check for metadata service endpoints (cloud security)
	metadataEndpoints := []string{
		"169.254.169.254", // AWS/GCP metadata
		"metadata.google.internal",
		"169.254.169.254:80",
	}
	for _, endpoint := range metadataEndpoints {
		if strings.EqualFold(hostname, endpoint) {
			return fmt.Errorf("metadata service access denied")
		}
	}

	return nil
}

// validatePathSecurity performs enhanced path validation (2024 security practices)
func (p *LightweightProxy) validatePathSecurity(path string) error {
	// Directory traversal prevention with multiple encoding forms
	dangerousPatterns := []string{
		"..", "/..", "../", "%2e%2e", "%2E%2E", // Basic directory traversal
		"%252e%252e", "%c0%ae", "%c1%9c", // Double-encoded and unicode variants
		"\\", "%5c", "%255c", // Backslash variants (Windows)
	}

	pathLower := strings.ToLower(path)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(pathLower, pattern) {
			return fmt.Errorf("dangerous path pattern detected: %s", pattern)
		}
	}

	// Path length validation
	if len(path) > 512 {
		return fmt.Errorf("path too long: %d characters (max 512)", len(path))
	}

	return nil
}

// validateQuerySecurity validates query parameters for security threats
func (p *LightweightProxy) validateQuerySecurity(rawQuery string) error {
	if rawQuery == "" {
		return nil // Empty queries are safe
	}

	// Query length validation
	if len(rawQuery) > 1024 {
		return fmt.Errorf("query string too long: %d characters (max 1024)", len(rawQuery))
	}

	// Check for SQL injection patterns
	sqlPatterns := []string{
		"union", "select", "insert", "delete", "update", "drop", "exec", "script",
		"'", "\"", ";", "--", "/*", "*/", "xp_", "sp_",
	}

	queryLower := strings.ToLower(rawQuery)
	for _, pattern := range sqlPatterns {
		if strings.Contains(queryLower, pattern) {
			return fmt.Errorf("potentially dangerous query pattern detected: %s", pattern)
		}
	}

	return nil
}

// validateFragmentSecurity validates URL fragments for XSS prevention
func (p *LightweightProxy) validateFragmentSecurity(fragment string) error {
	if fragment == "" {
		return nil // Empty fragments are safe
	}

	// Fragment length validation
	if len(fragment) > 256 {
		return fmt.Errorf("fragment too long: %d characters (max 256)", len(fragment))
	}

	// XSS prevention - check for dangerous JavaScript patterns
	dangerousFragments := []string{
		"javascript:", "data:", "vbscript:", "onload", "onerror", "onclick",
		"<script", "</script>", "eval(", "alert(", "document.", "window.",
	}

	fragmentLower := strings.ToLower(fragment)
	for _, pattern := range dangerousFragments {
		if strings.Contains(fragmentLower, pattern) {
			return fmt.Errorf("dangerous fragment pattern detected: %s", pattern)
		}
	}

	return nil
}

// validatePortSecurity validates URL ports for security bypass prevention
func (p *LightweightProxy) validatePortSecurity(port string) error {
	if port == "" {
		return nil // Default HTTPS port (443) is allowed
	}

	// Only allow standard HTTPS port
	if port != "443" {
		return fmt.Errorf("only standard HTTPS port (443) is allowed, got: %s", port)
	}

	return nil
}

// proxyToEnvoy executes the request to Envoy with retry logic
func (p *LightweightProxy) proxyToEnvoy(ctx context.Context, req *http.Request, traceID string) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt <= p.config.MaxRetries; attempt++ {
		if attempt > 0 {
			p.logger.Printf("[%s] Retrying request to Envoy (attempt %d/%d)", traceID, attempt+1, p.config.MaxRetries+1)

			// Exponential backoff
			backoff := time.Duration(attempt*attempt) * 100 * time.Millisecond
			time.Sleep(backoff)
		}

		resp, err := p.httpClient.Do(req)
		if err == nil {
			return resp, nil
		}

		lastErr = err
		p.logger.Printf("[%s] Request to Envoy failed (attempt %d): %v", traceID, attempt+1, err)
	}

	return nil, fmt.Errorf("all retry attempts failed, last error: %w", lastErr)
}

// copyResponse copies the Envoy response back to the client
func (p *LightweightProxy) copyResponse(w http.ResponseWriter, resp *http.Response, traceID string) {
	// Copy response headers
	for name, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}

	// Add proxy identification headers
	w.Header().Set("X-Proxy-Trace-ID", traceID)
	w.Header().Set("X-Proxy-Response-Time", time.Now().Format(time.RFC3339))

	// Set status code
	w.WriteHeader(resp.StatusCode)

	// Copy response body
	_, err := io.Copy(w, resp.Body)
	if err != nil {
		p.logger.Printf("[%s] Error copying response body: %v", traceID, err)
	}
}

// Health check endpoints

func (p *LightweightProxy) HandleHealthCheck(w http.ResponseWriter, r *http.Request) {
	if p.ready {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Lightweight Proxy Sidecar OK\nVersion: 1.0.0\nUpstream Resolution: ACTIVE\nEnvoy Target: %s\n", p.config.EnvoyUpstream)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, "Not Ready")
	}
}

func (p *LightweightProxy) HandleReadinessCheck(w http.ResponseWriter, r *http.Request) {
	// Test DNS resolution capability
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := p.dnsResolver.ResolveExternal(ctx, "httpbin.org")
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "DNS resolution test failed: %v", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "Ready")
}

func (p *LightweightProxy) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := p.metrics.GetMetrics()
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, "%s", metrics)
}

func (p *LightweightProxy) HandleDNSDebug(w http.ResponseWriter, r *http.Request) {
	dnsMetrics := p.dnsResolver.GetMetrics()
	dynamicDNSStats := p.dynamicDNS.GetDNSCacheStats()
	learnedDomains := p.dynamicDNS.GetLearnedDomains()

	w.Header().Set("Content-Type", "application/json")

	debugInfo := map[string]interface{}{
		"external_dns_metrics": dnsMetrics,
		"dynamic_dns_stats":    dynamicDNSStats,
		"learned_domains":      learnedDomains,
		"learned_domain_count": len(learnedDomains),
	}

	json.NewEncoder(w).Encode(debugInfo)
}

func (p *LightweightProxy) HandleConfigDebug(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Include auto-learning status in config debug
	learnedCount := len(p.autoLearner.GetLearnedDomains())

	fmt.Fprintf(w, `{
		"static_allowed_domains": %v, 
		"dns_servers": %v, 
		"envoy_upstream": "%s",
		"auto_learning": {
			"enabled": true,
			"learned_domains_count": %d,
			"csv_path": "/etc/sidecar-proxy/learned_domains.csv"
		}
	}`, p.config.AllowedDomainsRaw, p.config.DNSServers, p.config.EnvoyUpstream, learnedCount)
}

// HandleAutoLearnAdmin handles auto-learning administration API
func (p *LightweightProxy) HandleAutoLearnAdmin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		// Return all learned domains
		domains := p.autoLearner.GetLearnedDomains()
		response := struct {
			Success bool                       `json:"success"`
			Count   int                        `json:"count"`
			Domains []*autolearn.LearnedDomain `json:"domains"`
		}{
			Success: true,
			Count:   len(domains),
			Domains: domains,
		}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}

	case http.MethodPost:
		// Manual domain blocking
		var request struct {
			Domain string `json:"domain"`
			Reason string `json:"reason"`
		}

		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if request.Domain == "" {
			http.Error(w, "Domain is required", http.StatusBadRequest)
			return
		}

		traceID := p.generateTraceID()
		if err := p.autoLearner.BlockDomain(request.Domain, request.Reason, traceID); err != nil {
			http.Error(w, fmt.Sprintf("Failed to block domain: %v", err), http.StatusBadRequest)
			return
		}

		response := struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
			Domain  string `json:"domain"`
		}{
			Success: true,
			Message: "Domain blocked successfully",
			Domain:  request.Domain,
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleAutoLearnMetrics handles auto-learning metrics endpoint
func (p *LightweightProxy) HandleAutoLearnMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	domains := p.autoLearner.GetLearnedDomains()

	// Calculate metrics
	activeCount := 0
	blockedCount := 0
	totalAccess := int64(0)

	for _, domain := range domains {
		switch domain.Status {
		case "active":
			activeCount++
		case "blocked":
			blockedCount++
		}
		totalAccess += domain.AccessCount
	}

	metrics := struct {
		TotalDomains    int    `json:"total_domains"`
		ActiveDomains   int    `json:"active_domains"`
		BlockedDomains  int    `json:"blocked_domains"`
		TotalAccess     int64  `json:"total_access"`
		LearningEnabled bool   `json:"learning_enabled"`
		StorageType     string `json:"storage_type"`
		LastUpdate      string `json:"last_update"`
	}{
		TotalDomains:    len(domains),
		ActiveDomains:   activeCount,
		BlockedDomains:  blockedCount,
		TotalAccess:     totalAccess,
		LearningEnabled: true,
		StorageType:     "in-memory",
		LastUpdate:      time.Now().Format(time.RFC3339),
	}

	json.NewEncoder(w).Encode(metrics)
}

// Utility functions

func (p *LightweightProxy) generateTraceID() string {
	return fmt.Sprintf("proxy-%d", time.Now().UnixNano())
}

// extractDomainFromURL extracts domain from full URL for auto-learning
func extractDomainFromURL(targetURL string) string {
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		// Fallback: try to extract domain from string directly
		if strings.HasPrefix(targetURL, "https://") {
			targetURL = strings.TrimPrefix(targetURL, "https://")
		} else if strings.HasPrefix(targetURL, "http://") {
			targetURL = strings.TrimPrefix(targetURL, "http://")
		}

		// Extract domain part (before first '/')
		if slashIndex := strings.Index(targetURL, "/"); slashIndex != -1 {
			targetURL = targetURL[:slashIndex]
		}

		// Remove port if present
		if colonIndex := strings.Index(targetURL, ":"); colonIndex != -1 {
			targetURL = targetURL[:colonIndex]
		}

		return targetURL
	}

	host := parsedURL.Host
	if host == "" {
		return targetURL
	}

	// Remove port if present
	if colonIndex := strings.Index(host, ":"); colonIndex != -1 {
		host = host[:colonIndex]
	}

	return host
}

func (p *LightweightProxy) writeErrorResponse(w http.ResponseWriter, status int, message string, traceID string) {
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("X-Trace-ID", traceID)
	w.WriteHeader(status)
	fmt.Fprintf(w, "Proxy Error: %s\nTrace-ID: %s\n", message, traceID)
}

func (p *LightweightProxy) setupGracefulShutdown() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	p.logger.Println("Shutting down proxy sidecar gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := p.server.Shutdown(ctx); err != nil {
		p.logger.Printf("Server shutdown error: %v", err)
	}

	close(p.shutdownChan)
}

// HandleCONNECTRequest implements unified proxy strategy by converting CONNECT to /proxy/https:// format
// This enables news-creator and other services to use the same Envoy path as RSS feeds
func (p *LightweightProxy) HandleCONNECTRequest(w http.ResponseWriter, r *http.Request) {
	traceID := p.generateTraceID()

	p.logger.Printf("[%s] CONNECT tunnel request: %s (converting to unified /proxy/https:// format)", traceID, r.Host)

	// 1. Parse host:port from CONNECT request
	targetHost, targetPort, err := p.parseCONNECTTarget(r.Host)
	if err != nil {
		p.logger.Printf("[%s] Invalid CONNECT target: %v", traceID, err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// 2. Security: Validate allowed domains (consistent with RSS feeds)
	if !p.config.IsDomainAllowed(targetHost) {
		p.logger.Printf("[%s] Security: Blocked disallowed domain: %s", traceID, targetHost)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// 3. Only support HTTPS CONNECT (port 443) for security consistency with Envoy
	if targetPort != "443" {
		p.logger.Printf("[%s] Security: Only HTTPS (port 443) CONNECT supported, got port %s", traceID, targetPort)
		http.Error(w, "Only HTTPS CONNECT supported", http.StatusBadRequest)
		return
	}

	// 4. DNS Resolution (same as RSS feeds)
	ips, err := p.dnsResolver.ResolveExternal(context.Background(), targetHost)
	if err != nil || len(ips) == 0 {
		p.logger.Printf("[%s] DNS resolution failed for %s: %v", traceID, targetHost, err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	resolvedIP := ips[0]

	p.logger.Printf("[%s] DNS resolved: %s -> %s (unified proxy path)", traceID, targetHost, resolvedIP.String())

	// 5. Convert CONNECT to unified /connect/ path format
	connectPath := fmt.Sprintf("/connect/%s:443/", targetHost)

	// Create new request with /connect/ path
	connectReq, err := http.NewRequestWithContext(r.Context(), "GET", connectPath, nil)
	if err != nil {
		p.logger.Printf("[%s] Failed to create /connect/ request: %v", traceID, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Set required headers
	connectReq.Header.Set("X-Target-Domain", targetHost)
	connectReq.Header.Set("X-Forwarded-For", r.RemoteAddr)
	connectReq.Header.Set("X-Request-ID", traceID)

	// Process through unified /connect/ path
	connectReq.URL.Path = connectPath
	p.HandlePersistentTunnelRequest(w, connectReq)
}

// HandlePersistentTunnelRequest handles /connect/ path requests for persistent connections
// Routes through unified Envoy architecture with path-based responsibility separation
func (p *LightweightProxy) HandlePersistentTunnelRequest(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	traceID := p.generateTraceID()

	p.logger.Printf("[%s] Persistent tunnel request: %s %s", traceID, r.Method, r.URL.Path)

	// 1. Parse /connect/registry.ollama.ai:443/path format
	targetPath := strings.TrimPrefix(r.URL.Path, "/connect/")
	if targetPath == "" {
		p.logger.Printf("[%s] Empty /connect/ path", traceID)
		http.Error(w, "Bad Request: /connect/ path required", http.StatusBadRequest)
		return
	}

	// Extract target host from path (registry.ollama.ai:443)
	pathParts := strings.SplitN(targetPath, "/", 2)
	hostPort := pathParts[0]
	remainingPath := "/"
	if len(pathParts) > 1 {
		remainingPath = "/" + pathParts[1]
	}

	// 2. Parse host:port
	targetHost, targetPort, err := p.parseCONNECTTarget(hostPort)
	if err != nil {
		p.logger.Printf("[%s] Invalid target format: %v", traceID, err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// 3. Security: Validate allowed domains
	if !p.config.IsDomainAllowed(targetHost) {
		p.logger.Printf("[%s] Security: Blocked disallowed domain: %s", traceID, targetHost)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// 4. Only support HTTPS (port 443) for security
	if targetPort != "443" {
		p.logger.Printf("[%s] Security: Only HTTPS (port 443) supported, got port %s", traceID, targetPort)
		http.Error(w, "Only HTTPS supported", http.StatusBadRequest)
		return
	}

	// 5. Forward to Envoy using unified /connect/ path
	err = p.forwardToEnvoyConnect(w, r, targetHost, remainingPath, traceID, startTime)
	if err != nil {
		p.logger.Printf("[%s] Envoy forward failed: %v", traceID, err)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
}

// forwardToEnvoyConnect forwards /connect/ requests to Envoy with optimized settings
func (p *LightweightProxy) forwardToEnvoyConnect(w http.ResponseWriter, r *http.Request, targetHost, remainingPath, traceID string, startTime time.Time) error {
	// Create new request to Envoy - preserve the full /connect/ path
	envoyURL := fmt.Sprintf("http://%s%s", p.config.EnvoyUpstream, r.URL.Path)
	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, envoyURL, r.Body)
	if err != nil {
		return fmt.Errorf("failed to create proxy request: %w", err)
	}

	// Copy headers from original request
	for k, vv := range r.Header {
		if k == "Connection" || k == "Upgrade" || k == "Proxy-Connection" {
			continue // Skip connection-specific headers
		}
		for _, v := range vv {
			proxyReq.Header.Add(k, v)
		}
	}

	// Set required headers for Envoy dynamic forward proxy
	proxyReq.Header.Set("X-Target-Domain", targetHost)
	proxyReq.Header.Set("Host", targetHost)
	proxyReq.Header.Set("X-Forwarded-For", r.RemoteAddr)
	proxyReq.Header.Set("X-Request-ID", traceID)

	// Use HTTP client with extended timeout for persistent connections
	client := &http.Client{
		Timeout: 10 * time.Minute, // Extended timeout for model downloads
		Transport: &http.Transport{
			DisableKeepAlives:     false, // Enable keep-alives for persistent connections
			MaxIdleConns:          10,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   30 * time.Second,
			ExpectContinueTimeout: 10 * time.Second,
		},
	}

	p.logger.Printf("[%s] Forwarding to Envoy: %s with target %s", traceID, envoyURL, targetHost)

	// Execute request
	resp, err := client.Do(proxyReq)
	if err != nil {
		return fmt.Errorf("envoy request failed: %w", err)
	}
	defer resp.Body.Close()

	// Copy response headers
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}

	// Set response status
	w.WriteHeader(resp.StatusCode)

	// Stream response body
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		p.logger.Printf("[%s] Response streaming error: %v", traceID, err)
		return err
	}

	duration := time.Since(startTime)
	p.logger.Printf("[%s] Persistent tunnel completed: %d in %v", traceID, resp.StatusCode, duration)

	return nil
}

// HandleCONNECTRedirect converts CONNECT method to /connect/ path for unified architecture
func (p *LightweightProxy) HandleCONNECTRedirect(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	traceID := p.generateTraceID()

	p.logger.Printf("[%s] CONNECT redirect: %s %s", traceID, r.Method, r.Host)

	// 1. Parse CONNECT target host:port
	targetHost, targetPort, err := p.parseCONNECTTarget(r.Host)
	if err != nil {
		p.logger.Printf("[%s] Invalid CONNECT target: %v", traceID, err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// 2. Security: Only support HTTPS CONNECT (port 443)
	if targetPort != "443" {
		p.logger.Printf("[%s] Security: Only HTTPS CONNECT supported, got port %s", traceID, targetPort)
		http.Error(w, "Only HTTPS CONNECT supported", http.StatusBadRequest)
		return
	}

	// 3. Security: Validate allowed domains
	if !p.config.IsDomainAllowed(targetHost) {
		p.logger.Printf("[%s] Security: Blocked disallowed domain: %s", traceID, targetHost)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// 4. Convert CONNECT to /connect/ path format
	connectPath := fmt.Sprintf("/connect/%s:%s/", targetHost, targetPort)

	// 5. Create new request with /connect/ path
	connectReq, err := http.NewRequestWithContext(r.Context(), "GET", connectPath, nil)
	if err != nil {
		p.logger.Printf("[%s] Failed to create /connect/ request: %v", traceID, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Copy relevant headers
	connectReq.Header.Set("X-Target-Domain", targetHost)
	connectReq.Header.Set("X-Forwarded-For", r.RemoteAddr)
	connectReq.Header.Set("X-Request-ID", traceID)
	connectReq.Header.Set("Connection", "Upgrade")
	connectReq.Header.Set("Upgrade", "tcp")

	// Copy original headers (selective)
	for k, vv := range r.Header {
		if k == "Host" || k == "Connection" || k == "Upgrade" {
			continue // Skip, we set these above
		}
		for _, v := range vv {
			connectReq.Header.Add(k, v)
		}
	}

	p.logger.Printf("[%s] Redirecting CONNECT %s:%s to /connect/ path", traceID, targetHost, targetPort)

	// 6. Process through /connect/ path (unified architecture)
	connectReq.URL.Path = connectPath
	p.HandlePersistentTunnelRequest(w, connectReq)

	duration := time.Since(startTime)
	p.logger.Printf("[%s] CONNECT redirect completed in %v", traceID, duration)
}

// HandleDynamicCONNECTRequest implements on-memory DNS management with dynamic domain resolution
// オンメモリDNS管理: 動的ドメイン許可システム
func (p *LightweightProxy) HandleDynamicCONNECTRequest(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	traceID := p.generateTraceID()

	p.logger.Printf("[%s] Dynamic CONNECT request: %s %s", traceID, r.Method, r.Host)

	// 1. Parse CONNECT target host:port
	targetHost, targetPort, err := p.parseCONNECTTarget(r.Host)
	if err != nil {
		p.logger.Printf("[%s] Invalid CONNECT target: %v", traceID, err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// 2. Security: Only support HTTPS CONNECT (port 443)
	if targetPort != "443" {
		p.logger.Printf("[%s] Security: Only HTTPS CONNECT supported, got port %s", traceID, targetPort)
		http.Error(w, "Only HTTPS CONNECT supported", http.StatusBadRequest)
		return
	}

	// 3. オンメモリDNS管理: 動的ドメイン解決と学習
	allowed, learned := p.dynamicDNS.IsDomainAllowed(targetHost)
	if !allowed {
		p.logger.Printf("[%s] Security: Blocked disallowed domain: %s", traceID, targetHost)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if learned {
		p.logger.Printf("[%s] Dynamic learning: Added new domain to cache: %s", traceID, targetHost)
	}

	// 4. DNS Pre-resolution for on-memory caching
	if err := p.dynamicDNS.PreResolveDomain(targetHost); err != nil {
		p.logger.Printf("[%s] DNS pre-resolution failed for %s: %v", traceID, targetHost, err)
		// Continue anyway - Envoy will handle DNS resolution
	}

	// 5. Convert CONNECT to /connect/ path format
	connectPath := fmt.Sprintf("/connect/%s:%s/", targetHost, targetPort)

	// 6. Create new request with /connect/ path
	connectReq, err := http.NewRequestWithContext(r.Context(), "GET", connectPath, nil)
	if err != nil {
		p.logger.Printf("[%s] Failed to create /connect/ request: %v", traceID, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Copy relevant headers
	connectReq.Header.Set("X-Target-Domain", targetHost)
	connectReq.Header.Set("X-Forwarded-For", r.RemoteAddr)
	connectReq.Header.Set("X-Request-ID", traceID)
	connectReq.Header.Set("Connection", "Upgrade")
	connectReq.Header.Set("Upgrade", "tcp")

	// Copy original headers (selective)
	for k, vv := range r.Header {
		if k == "Host" || k == "Connection" || k == "Upgrade" {
			continue // Skip, we set these above
		}
		connectReq.Header[k] = vv
	}

	// 7. Forward to persistent tunnel handler
	p.logger.Printf("[%s] Forwarding dynamic CONNECT to tunnel handler: %s", traceID, connectPath)
	p.HandlePersistentTunnelRequest(w, connectReq)

	duration := time.Since(startTime)
	p.logger.Printf("[%s] Dynamic CONNECT completed in %v", traceID, duration)
}

// parseCONNECTTarget parses CONNECT request format "host:port"
func (p *LightweightProxy) parseCONNECTTarget(hostPort string) (host, port string, err error) {
	// Parse "registry.ollama.ai:443" format
	parts := strings.Split(hostPort, ":")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid host:port format: %s", hostPort)
	}

	host = strings.TrimSpace(parts[0])
	port = strings.TrimSpace(parts[1])

	// Validate host
	if host == "" || strings.Contains(host, "/") {
		return "", "", fmt.Errorf("invalid host: %s", host)
	}

	// Validate port
	if portNum, err := strconv.Atoi(port); err != nil || portNum <= 0 || portNum > 65535 {
		return "", "", fmt.Errorf("invalid port: %s", port)
	}

	return host, port, nil
}

// hijackConnection hijacks HTTP connection for raw TCP tunneling
func (p *LightweightProxy) hijackConnection(w http.ResponseWriter) (net.Conn, error) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return nil, fmt.Errorf("connection hijacking not supported")
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		return nil, fmt.Errorf("hijack failed: %w", err)
	}

	return clientConn, nil
}

// startTCPTunnel handles bidirectional TCP data transfer
func (p *LightweightProxy) startTCPTunnel(clientConn, upstreamConn net.Conn, traceID string, startTime time.Time) {
	p.logger.Printf("[%s] Starting TCP tunnel", traceID)

	// Set connection timeouts
	deadline := time.Now().Add(p.config.CONNECTIdleTimeout)
	clientConn.SetDeadline(deadline)
	upstreamConn.SetDeadline(deadline)

	// Bidirectional copy
	done := make(chan error, 2)

	// Client -> Upstream
	go func() {
		_, err := io.Copy(upstreamConn, clientConn)
		done <- err
	}()

	// Upstream -> Client
	go func() {
		_, err := io.Copy(clientConn, upstreamConn)
		done <- err
	}()

	// Wait for one direction to close
	<-done

	duration := time.Since(startTime)
	p.logger.Printf("[%s] TCP tunnel closed, duration: %v", traceID, duration)
}
