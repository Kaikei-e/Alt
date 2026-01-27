package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// generateTraceID creates a unique trace ID for request tracking
func (p *LightweightProxy) generateTraceID() string {
	return fmt.Sprintf("proxy-%d", time.Now().UnixNano())
}

// extractDomainFromURL extracts domain from full URL for auto-learning
func (p *LightweightProxy) extractDomainFromURL(targetURL string) string {
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

// writeErrorResponse writes a standardized error response to the client
func (p *LightweightProxy) writeErrorResponse(w http.ResponseWriter, status int, message string, traceID string) {
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("X-Trace-ID", traceID)
	w.WriteHeader(status)
	fmt.Fprintf(w, "Proxy Error: %s\nTrace-ID: %s\n", message, traceID)
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

// setupGracefulShutdown sets up graceful shutdown handling
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
