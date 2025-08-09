package proxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// forwardToEnvoyConnect forwards CONNECT requests to Envoy with proper headers
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