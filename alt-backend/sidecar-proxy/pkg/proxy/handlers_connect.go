package proxy

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

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

	// 2. FIXED: Use dynamic DNS management instead of static config
	allowed, learned := p.dynamicDNS.IsDomainAllowed(targetHost)
	if !allowed {
		p.logger.Printf("[%s] Security: Blocked disallowed domain: %s", traceID, targetHost)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if learned {
		p.logger.Printf("[%s] Dynamic learning: Added new domain to cache: %s", traceID, targetHost)
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

	// 3. FIXED: Use dynamic DNS management instead of static config
	allowed, learned := p.dynamicDNS.IsDomainAllowed(targetHost)
	if !allowed {
		p.logger.Printf("[%s] Security: Blocked disallowed domain: %s", traceID, targetHost)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if learned {
		p.logger.Printf("[%s] Dynamic learning: Added new domain to cache: %s", traceID, targetHost)
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

	// 3. FIXED: Use dynamic DNS management instead of static config
	allowed, learned := p.dynamicDNS.IsDomainAllowed(targetHost)
	if !allowed {
		p.logger.Printf("[%s] Security: Blocked disallowed domain: %s", traceID, targetHost)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if learned {
		p.logger.Printf("[%s] Dynamic learning: Added new domain to cache: %s", traceID, targetHost)
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
