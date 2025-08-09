package proxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

// HandleProxyRequest is the core function that solves the upstream resolution problem
// This function implements the critical logic described in ISSUE_RESOLVE_PLAN.md
func (p *LightweightProxy) HandleProxyRequest(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	// Generate trace ID for request tracking
	traceID := p.generateTraceID()

	p.logger.Printf("[%s] Processing proxy request: %s %s", traceID, r.Method, r.URL.Path)

	// Extract target URL from request path (e.g., /proxy/https://zenn.dev/feed)
	p.logger.Printf("[%s] About to extract target URL from path: %s", traceID, r.URL.Path)
	targetURL, err := p.extractTargetURL(r.URL.Path)
	if err != nil {
		p.logger.Printf("[%s] Failed to extract target URL: %v", traceID, err)
		p.writeErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid target URL: %v", err), traceID)
		return
	}

	p.logger.Printf("[%s] Target URL extracted: %s", traceID, targetURL.String())

	// 🎯 CRITICAL: Transparent auto-learning domain validation
	domain := targetURL.Hostname() // Use hostname without port

	// Check static allowlist first (fastest path)
	if !p.config.IsDomainAllowed(domain) {
		p.logger.Printf("[%s] Domain %s not in static allowlist, blocked", traceID, domain)
		p.writeErrorResponse(w, http.StatusForbidden,
			fmt.Sprintf("Domain not allowed: %s", domain), traceID)
		return
	}

	// 🌐 CRITICAL: External DNS resolution to bypass Kubernetes internal DNS
	// This is the key step that enables proper upstream resolution
	dnsStartTime := time.Now()
	// Use hostname without port for DNS resolution
	hostname := targetURL.Hostname()
	resolvedIPs, err := p.dnsResolver.ResolveExternal(r.Context(), hostname)
	dnsTime := time.Since(dnsStartTime)

	if err != nil {
		p.logger.Printf("[%s] DNS resolution failed for %s: %v", traceID, hostname, err)
		p.writeErrorResponse(w, http.StatusBadGateway, fmt.Sprintf("DNS resolution failed: %v", err), traceID)
		return
	}

	p.logger.Printf("[%s] DNS resolved: %s -> %v (took %v)", traceID, hostname, resolvedIPs, dnsTime)

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
	// For HTTPS, show hostname without default port 443 (standard practice)
	upstreamExpected := targetURL.Hostname()
	if targetURL.Scheme == "https" && targetURL.Port() != "" && targetURL.Port() != "443" {
		upstreamExpected = targetURL.Host // Only show port if it's non-standard
	}

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

	// 1. Host header: Use hostname without default port (443 for HTTPS) to avoid 403 errors
	// Many servers/CDNs reject explicit default ports in Host header
	hostname := targetURL.Hostname()
	req.Header.Set("Host", hostname)

	// 2. X-Target-Domain: Also use hostname without port for consistency
	req.Header.Set("X-Target-Domain", hostname)

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
		req.Header.Set(":authority", hostname)
	}

	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	}

	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	}
	if req.Header.Get("Accept-Language") == "" {
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	}
	if req.Header.Get("Accept-Encoding") == "" {
		req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	}
	if req.Header.Get("Referer") == "" {
		refererURL := "https://" + hostname + "/"
		req.Header.Set("Referer", refererURL)
	}
	if req.Header.Get("Sec-Fetch-Dest") == "" {
		req.Header.Set("Sec-Fetch-Dest", "document")
	}
	if req.Header.Get("Sec-Fetch-Mode") == "" {
		req.Header.Set("Sec-Fetch-Mode", "navigate")
	}
	if req.Header.Get("Sec-Fetch-Site") == "" {
		req.Header.Set("Sec-Fetch-Site", "same-origin")
	}

	p.logger.Printf("[%s] 🔧 Built Envoy request with headers: Host=%s, X-Target-Domain=%s, X-Resolved-IP=%s",
		traceID, hostname, hostname, resolvedIP.String())

	return req, nil
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