package utils

import (
	"alt/config"
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
	ProxyStrategyDirect ProxyStrategy = "DIRECT"
	ProxyStrategyEnvoy  ProxyStrategy = "ENVOY"
)

// HTTPClientFactory creates HTTP clients with unified proxy strategy
type HTTPClientFactory struct {
	proxyStrategy ProxyStrategy
	envoyBaseURL  string
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

	slog.Info("HTTP client factory initialized",
		"proxy_strategy", strategy,
		"envoy_base_url", envoyBaseURL)

	return &HTTPClientFactory{
		proxyStrategy: strategy,
		envoyBaseURL:  envoyBaseURL,
	}
}

// CreateHTTPClient creates an HTTP client with proxy-aware configuration
func (f *HTTPClientFactory) CreateHTTPClient() *http.Client {
	switch f.proxyStrategy {
	case ProxyStrategyEnvoy:
		return f.createEnvoyProxyClient()
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
		Transport:     baseTransport,
		EnvoyBaseURL:  f.envoyBaseURL,
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

	slog.Info("Envoy proxy request transformation",
		"original_url", req.URL.String(),
		"transformed_url", clonedReq.URL.String(),
		"target_domain", originalHost)

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
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
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
		"22":    true, // SSH
		"23":    true, // Telnet
		"25":    true, // SMTP
		"53":    true, // DNS
		"80":    true, // HTTP (if we want to force HTTPS only)
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

// isPrivateHost checks if a hostname resolves to private IP addresses
func isPrivateHost(hostname string) bool {
	// Try to parse as IP first
	ip := net.ParseIP(hostname)
	if ip != nil {
		return isPrivateIPAddress(ip)
	}

	// Block localhost variations
	hostname = strings.ToLower(hostname)
	if hostname == "localhost" || strings.HasPrefix(hostname, "127.") {
		return true
	}

	// Block metadata endpoints
	if hostname == "169.254.169.254" || hostname == "metadata.google.internal" {
		return true
	}

	// Block common internal domains
	internalDomains := []string{".local", ".internal", ".corp", ".lan"}
	for _, domain := range internalDomains {
		if strings.HasSuffix(hostname, domain) {
			return true
		}
	}

	// If it's a hostname, resolve it to IPs
	ips, err := net.LookupIP(hostname)
	if err != nil {
		// Block on resolution failure as a security measure
		return true
	}

	// Check if any resolved IP is private
	for _, ip := range ips {
		if isPrivateIPAddress(ip) {
			return true
		}
	}

	return false
}

// isPrivateIPAddress checks if an IP address is in private ranges
func isPrivateIPAddress(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	// Check for private IPv4 ranges
	if ip.To4() != nil {
		// 10.0.0.0/8
		if ip[0] == 10 {
			return true
		}
		// 172.16.0.0/12
		if ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31 {
			return true
		}
		// 192.168.0.0/16
		if ip[0] == 192 && ip[1] == 168 {
			return true
		}
	}

	// Check for private IPv6 ranges
	if ip.To16() != nil && ip.To4() == nil {
		// Check for unique local addresses (fc00::/7)
		if ip[0] == 0xfc || ip[0] == 0xfd {
			return true
		}
	}

	return false
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
