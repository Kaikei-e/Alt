package utils

import (
	"alt/config"
	"alt/utils/security"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ProxyStrategy defines the proxy strategy for HTTP clients
type ProxyStrategy string

const (
	ProxyStrategyDirect  ProxyStrategy = "DIRECT"
	ProxyStrategyEnvoy   ProxyStrategy = "ENVOY"
	ProxyStrategySidecar ProxyStrategy = "SIDECAR"
)

// HTTPClientFactory creates HTTP clients with unified proxy strategy
type HTTPClientFactory struct {
	proxyStrategy   ProxyStrategy
	envoyBaseURL    string
	sidecarProxyURL string
}

// NewHTTPClientFactory creates a new HTTP client factory with environment-based configuration
func NewHTTPClientFactory() *HTTPClientFactory {
	strategy := ProxyStrategy(os.Getenv("PROXY_STRATEGY"))
	if strategy == "" {
		strategy = ProxyStrategyDirect
	}

	envoyBaseURL := os.Getenv("ENVOY_PROXY_BASE_URL")
	if envoyBaseURL == "" {
		envoyBaseURL = "http://envoy-proxy.alt-apps.svc.cluster.local:8080"
	}

	sidecarProxyURL := os.Getenv("SIDECAR_PROXY_BASE_URL")
	if sidecarProxyURL == "" {
		sidecarProxyURL = "http://sidecar-proxy.alt-apps.svc.cluster.local:8085"
	}

	slog.InfoContext(context.Background(), "HTTP client factory initialized",
		"proxy_strategy", strategy,
		"envoy_base_url", envoyBaseURL,
		"sidecar_proxy_url", sidecarProxyURL)

	return &HTTPClientFactory{
		proxyStrategy:   strategy,
		envoyBaseURL:    envoyBaseURL,
		sidecarProxyURL: sidecarProxyURL,
	}
}

// CreateHTTPClient creates an HTTP client with proxy-aware configuration
func (f *HTTPClientFactory) CreateHTTPClient() *http.Client {
	switch f.proxyStrategy {
	case ProxyStrategyEnvoy:
		return f.createEnvoyProxyClient()
	case ProxyStrategySidecar:
		return f.createSidecarProxyClient()
	default:
		return f.createSecureDirectClient()
	}
}

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

// createSecureDirectClient creates a secure HTTP client with SSRF protection
func (f *HTTPClientFactory) createSecureDirectClient() *http.Client {
	cfg := &config.HTTPConfig{
		ClientTimeout:       30 * time.Second,
		DialTimeout:         10 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		IdleConnTimeout:     90 * time.Second,
	}
	return SecureHTTPClientWithConfig(cfg)
}

// SecureHTTPClient creates an HTTP client with SSRF protection (deprecated - use factory)
func SecureHTTPClient() *http.Client {
	factory := NewHTTPClientFactory()
	return factory.CreateHTTPClient()
}

// SecureHTTPClientWithConfig creates an HTTP client with SSRF protection using provided configuration
func SecureHTTPClientWithConfig(cfg *config.HTTPConfig) *http.Client {
	dialer := &net.Dialer{
		Timeout: cfg.DialTimeout,
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}

			// Validate the target before making the connection
			if err := validateTarget(host, port); err != nil {
				return nil, err
			}

			return dialer.DialContext(ctx, network, addr)
		},
		TLSHandshakeTimeout: cfg.TLSHandshakeTimeout,
		IdleConnTimeout:     cfg.IdleConnTimeout,
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 50,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   cfg.ClientTimeout,
	}
}

// validateTarget validates the target host and port for SSRF protection
func validateTarget(host, port string) error {
	// Block common internal ports
	blockedPorts := map[string]bool{
		"22": true, // SSH
		"23": true, // Telnet
		"25": true, // SMTP
		"53": true, // DNS
		// "80":    true, // HTTP (allowed for robots.txt and general scraping)
		"110":   true, // POP3
		"143":   true, // IMAP
		"993":   true, // IMAPS
		"995":   true, // POP3S
		"1433":  true, // MSSQL
		"3306":  true, // MySQL
		"5432":  true, // PostgreSQL
		"6379":  true, // Redis
		"11211": true, // Memcached
	}

	if blockedPorts[port] {
		return errors.New("access to this port is not allowed")
	}

	// Check if the host resolves to a private IP
	if isPrivateHost(host) {
		return errors.New("access to private networks not allowed")
	}

	return nil
}

// isPrivateHost checks if a hostname resolves to private IP addresses.
// This function provides additional internal domain blocking on top of security.IsPrivateHost.
func isPrivateHost(hostname string) bool {
	// Block localhost variations (string check for faster path)
	hostnameLC := strings.ToLower(hostname)
	if hostnameLC == "localhost" || strings.HasPrefix(hostnameLC, "127.") {
		return true
	}

	// Block metadata endpoints
	if hostnameLC == "169.254.169.254" || hostnameLC == "metadata.google.internal" {
		return true
	}

	// Block common internal domains
	internalDomains := []string{".local", ".internal", ".corp", ".lan"}
	for _, domain := range internalDomains {
		if strings.HasSuffix(hostnameLC, domain) {
			return true
		}
	}

	// Delegate to security.IsPrivateHost for IP validation and DNS resolution checks
	return security.IsPrivateHost(hostname)
}

// ValidateURL validates a URL for SSRF protection
func ValidateURL(u *url.URL) error {
	// Allow both HTTP and HTTPS
	if u.Scheme != "https" && u.Scheme != "http" {
		return errors.New("only HTTP and HTTPS schemes allowed")
	}

	host := u.Hostname()
	port := u.Port()

	// Reuse target validation logic which checks private hosts and blocked ports
	if err := validateTarget(host, port); err != nil {
		return err
	}

	if u.Hostname() == "" {
		return errors.New("URL must contain a host")
	}

	return nil
}
