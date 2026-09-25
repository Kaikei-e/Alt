package utils

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"
)

// createEnvoyProxyClient creates an HTTP client that routes through Envoy proxy
func (f *HTTPClientFactory) createEnvoyProxyClient() *http.Client {
	baseTransport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
		},
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 30 * time.Second,
	}

	// Wrap with Envoy proxy transport
	envoyTransport := &EnvoyProxyTransport{
		Transport:    baseTransport,
		EnvoyBaseURL: f.envoyBaseURL,
	}

	return &http.Client{
		Transport: envoyTransport,
		Timeout:   60 * time.Second,
	}
}

// EnvoyProxyTransport implements RoundTripper to transform requests for Envoy Dynamic Forward Proxy
type EnvoyProxyTransport struct {
	Transport    http.RoundTripper
	EnvoyBaseURL string
}

// RoundTrip transforms requests to route through Envoy Dynamic Forward Proxy
func (t *EnvoyProxyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone the request to avoid modifying the original
	clonedReq := req.Clone(req.Context())

	// Extract original URL components
	originalHost := req.URL.Host
	originalScheme := req.URL.Scheme
	originalPath := req.URL.Path
	if req.URL.RawQuery != "" {
		originalPath += "?" + req.URL.RawQuery
	}

	// Parse Envoy base URL
	envoyURL, err := url.Parse(t.EnvoyBaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Envoy base URL: %w", err)
	}

	// Transform URL to Envoy Dynamic Forward Proxy format
	// Original: https://example.com/rss.xml
	// Transformed: http://envoy-proxy:8080/proxy/https://example.com/rss.xml
	clonedReq.URL.Scheme = envoyURL.Scheme
	clonedReq.URL.Host = envoyURL.Host
	clonedReq.URL.Path = "/proxy/" + originalScheme + "://" + originalHost + originalPath

	// Add required X-Target-Domain header for Envoy Dynamic Forward Proxy
	clonedReq.Header.Set("X-Target-Domain", originalHost)

	slog.InfoContext(req.Context(), "Envoy proxy request transformation",
		"original_url", req.URL.String(),
		"transformed_url", clonedReq.URL.String(),
		"target_domain", originalHost)

	// Execute the transformed request
	return t.Transport.RoundTrip(clonedReq)
}

// createSidecarProxyClient creates an HTTP client that routes through sidecar proxy
func (f *HTTPClientFactory) createSidecarProxyClient() *http.Client {
	baseTransport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
		},
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 30 * time.Second,
	}

	// Wrap with Sidecar proxy transport
	sidecarTransport := &SidecarProxyTransport{
		Transport:       baseTransport,
		SidecarProxyURL: f.sidecarProxyURL,
	}

	return &http.Client{
		Transport: sidecarTransport,
		Timeout:   60 * time.Second,
	}
}

// SidecarProxyTransport implements RoundTripper to route requests through the sidecar proxy
type SidecarProxyTransport struct {
	Transport       http.RoundTripper
	SidecarProxyURL string
}

// RoundTrip transforms requests to route through the sidecar proxy at localhost:8085
func (t *SidecarProxyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone the request to avoid modifying the original
	clonedReq := req.Clone(req.Context())

	// Extract original URL components
	originalURL := req.URL.String()

	// Parse sidecar proxy URL
	sidecarURL, err := url.Parse(t.SidecarProxyURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse sidecar proxy URL: %w", err)
	}

	// Transform URL to sidecar proxy format
	// Original: https://example.com/rss.xml
	// Transformed: http://localhost:8085/proxy/https://example.com/rss.xml
	clonedReq.URL.Scheme = sidecarURL.Scheme
	clonedReq.URL.Host = sidecarURL.Host
	clonedReq.URL.Path = "/proxy/" + originalURL

	// Preserve original Host header for the target
	clonedReq.Header.Set("X-Original-Host", req.Host)

	// Add trace header for debugging
	clonedReq.Header.Set("X-Proxy-Via", "sidecar-proxy")

	slog.InfoContext(req.Context(), "Sidecar proxy request transformation",
		"original_url", originalURL,
		"transformed_url", clonedReq.URL.String(),
		"sidecar_proxy", t.SidecarProxyURL)

	// Execute the transformed request
	return t.Transport.RoundTrip(clonedReq)
}
