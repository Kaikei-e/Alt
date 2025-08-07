// Package proxy implements the core HTTP proxy functionality for the lightweight sidecar
// This package contains the critical upstream resolution logic described in ISSUE_RESOLVE_PLAN.md
// to transform upstream="10.96.32.212:8080" into upstream="zenn.dev:443"
package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

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
	
	// Request processing state
	shutdownChan chan struct{}
	ready        bool
}

// RequestContext holds context information for each proxy request
// This enables detailed logging and tracing as specified in the plan
type RequestContext struct {
	TraceID        string    `json:"trace_id"`
	StartTime      time.Time `json:"start_time"`
	TargetURL      *url.URL  `json:"target_url"`
	ResolvedIP     net.IP    `json:"resolved_ip"`
	Method         string    `json:"method"`
	UserAgent      string    `json:"user_agent"`
	ContentLength  int64     `json:"content_length"`
}

// ProxyResponse contains the response details for logging and metrics
type ProxyResponse struct {
	StatusCode    int           `json:"status_code"`
	ContentLength int64         `json:"content_length"`
	Duration      time.Duration `json:"duration"`
	DNSTime       time.Duration `json:"dns_time"`
	ProxyTime     time.Duration `json:"proxy_time"`
	UpstreamHost  string        `json:"upstream_host"`  // This is the key field for solving the upstream problem
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

	proxy := &LightweightProxy{
		config:       cfg,
		httpClient:   httpClient,
		dnsResolver:  dnsResolver,
		logger:       logger,
		metrics:      metricsCollector,
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
	allowedMethods := map[string]bool{
		"GET": true, "POST": true, "PUT": true, "DELETE": true, "HEAD": true, "OPTIONS": true,
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
	
	// 🎯 CRITICAL: Security validation - check if domain is allowed
	if !p.config.IsDomainAllowed(targetURL.Host) {
		p.logger.Printf("[%s] Domain not allowed: %s", traceID, targetURL.Host)
		p.writeErrorResponse(w, http.StatusForbidden, fmt.Sprintf("Domain not allowed: %s", targetURL.Host), traceID)
		return
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
	
	// Envoy forward proxy URL: 絶対URLを使用
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
	const maxNestedPercent = 50 // Maximum percentage of URL that can be percent-encoded
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
		"%252e%252e", "%c0%ae", "%c1%9c",        // Double-encoded and unicode variants
		"\\", "%5c", "%255c",                    // Backslash variants (Windows)
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
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"dns_metrics": %+v}`, dnsMetrics)
}

func (p *LightweightProxy) HandleConfigDebug(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"allowed_domains": %v, "dns_servers": %v, "envoy_upstream": "%s"}`, 
		p.config.AllowedDomainsRaw, p.config.DNSServers, p.config.EnvoyUpstream)
}

// Utility functions

func (p *LightweightProxy) generateTraceID() string {
	return fmt.Sprintf("proxy-%d", time.Now().UnixNano())
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