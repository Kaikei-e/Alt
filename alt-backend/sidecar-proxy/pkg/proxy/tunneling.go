package proxy

import (
	"fmt"
	"io"
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

	// Set required headers for Envoy dynamic forward proxy.
	// net/http sends the Host line from proxyReq.Host, not from the Header
	// map — Header.Set("Host", ...) was a silent no-op.
	proxyReq.Header.Set("X-Target-Domain", targetHost)
	proxyReq.Host = targetHost
	proxyReq.Header.Set("X-Forwarded-For", r.RemoteAddr)
	proxyReq.Header.Set("X-Request-ID", traceID)

	p.logger.Printf("[%s] Forwarding to Envoy: %s with target %s", traceID, envoyURL, targetHost)

	// Execute request using the shared, long-lived tunnel client (built once
	// in NewLightweightProxy) so keep-alive connections to Envoy are
	// actually reused across CONNECT/persistent-tunnel requests instead of
	// being torn down and rebuilt every call.
	resp, err := p.connectHTTPClient.Do(proxyReq)
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
