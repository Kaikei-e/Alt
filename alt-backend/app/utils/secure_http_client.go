package utils

import (
	"alt/utils/security"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// HTTPConfig holds outbound HTTP transport timeouts.
type HTTPConfig struct {
	ClientTimeout       time.Duration
	DialTimeout         time.Duration
	TLSHandshakeTimeout time.Duration
	IdleConnTimeout     time.Duration
}

// ProxyStrategy defines the proxy strategy for HTTP clients
type ProxyStrategy string

const (
	ProxyStrategyDirect  ProxyStrategy = "DIRECT"
	ProxyStrategyEnvoy   ProxyStrategy = "ENVOY"
	ProxyStrategySidecar ProxyStrategy = "SIDECAR"
)

// IPResolver resolves hostnames to IP addresses.
type IPResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// IPResolverFunc adapts a function to the IPResolver interface.
type IPResolverFunc func(ctx context.Context, host string) ([]net.IPAddr, error)

func (f IPResolverFunc) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return f(ctx, host)
}

// ErrDestinationNotAllowed is returned when a destination is blocked for SSRF protection.
// It is intentionally generic to avoid leaking internal IP addresses or topology.
var (
	ErrDestinationNotAllowed = errors.New("destination not allowed")
	ErrLookupFailed          = errors.New("host lookup failed")
	ErrConnectionFailed      = errors.New("connection failed")
)

// HTTPClientFactory creates HTTP clients with unified proxy strategy
type HTTPClientFactory struct {
	proxyStrategy   ProxyStrategy
	envoyBaseURL    string
	sidecarProxyURL string
	resolver        IPResolver
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
		resolver:        net.DefaultResolver,
	}
}

// WithResolver configures a custom IPResolver on the factory.
func (f *HTTPClientFactory) WithResolver(resolver IPResolver) *HTTPClientFactory {
	f.resolver = resolver
	return f
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

// createSecureDirectClient creates a secure HTTP client with SSRF protection
func (f *HTTPClientFactory) createSecureDirectClient() *http.Client {
	cfg := &HTTPConfig{
		ClientTimeout:       30 * time.Second,
		DialTimeout:         10 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		IdleConnTimeout:     90 * time.Second,
	}
	resolver := f.resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	return SecureHTTPClientWithConfigAndResolver(cfg, resolver)
}

// SecureHTTPClient creates an HTTP client with SSRF protection (deprecated - use factory)
func SecureHTTPClient() *http.Client {
	factory := NewHTTPClientFactory()
	return factory.CreateHTTPClient()
}

// SecureHTTPClientWithConfig creates an HTTP client with SSRF protection using provided configuration
func SecureHTTPClientWithConfig(cfg *HTTPConfig) *http.Client {
	return SecureHTTPClientWithConfigAndResolver(cfg, net.DefaultResolver)
}

// SecureHTTPClientWithConfigAndResolver creates an HTTP client with SSRF protection and an injectable resolver.
func SecureHTTPClientWithConfigAndResolver(cfg *HTTPConfig, resolver IPResolver) *http.Client {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	dialer := &net.Dialer{
		Timeout: cfg.DialTimeout,
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, ErrDestinationNotAllowed
			}

			// Block common internal ports
			if isBlockedPort(port) {
				return nil, ErrDestinationNotAllowed
			}

			// Resolve host once inside DialContext using the configured resolver.
			// An IP literal resolves to itself.
			var addrs []net.IPAddr
			if ip := net.ParseIP(host); ip != nil {
				addrs = []net.IPAddr{{IP: ip}}
			} else {
				var err error
				addrs, err = resolver.LookupIPAddr(ctx, host)
				if err != nil {
					return nil, fmt.Errorf("%w for %s: %v", ErrLookupFailed, host, err)
				}
				if len(addrs) == 0 {
					return nil, fmt.Errorf("%w for %s: no addresses found", ErrLookupFailed, host)
				}
			}

			// Validate EVERY returned IP.
			// Reject if any is private/special unless the host is on FEED_ALLOWED_HOSTS.
			isAllowedHost := security.IsFeedHostAllowed(host)
			if !isAllowedHost {
				// Block known internal domains, metadata names, and literal private IPs before resolution
				if isPrivateDomainOrLiteral(host) {
					return nil, ErrDestinationNotAllowed
				}

				for _, a := range addrs {
					if security.IsPrivateIPAddress(a.IP) || IsMetadataIP(a.IP) {
						return nil, ErrDestinationNotAllowed
					}
				}
			}

			// Determine overall deadline for dialing across all candidates.
			var overallDeadline time.Time
			if dl, ok := ctx.Deadline(); ok {
				overallDeadline = dl
			} else if cfg.DialTimeout > 0 {
				overallDeadline = time.Now().Add(cfg.DialTimeout)
			}

			// Pin connection to validated IPs in order, respecting context and splitting deadline
			var dialErr error
			for i, a := range addrs {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}

				remAddrs := len(addrs) - i
				stepCtx := ctx
				var cancel context.CancelFunc

				if !overallDeadline.IsZero() {
					timeRemaining := time.Until(overallDeadline)
					if timeRemaining <= 0 {
						return nil, context.DeadlineExceeded
					}
					stepTimeout := timeRemaining / time.Duration(remAddrs)
					if cfg.DialTimeout > 0 && stepTimeout > cfg.DialTimeout {
						stepTimeout = cfg.DialTimeout
					}
					stepCtx, cancel = context.WithTimeout(ctx, stepTimeout)
				} else if cfg.DialTimeout > 0 {
					stepCtx, cancel = context.WithTimeout(ctx, cfg.DialTimeout)
				}

				pinned := net.JoinHostPort(a.IP.String(), port)
				conn, err := dialer.DialContext(stepCtx, network, pinned)
				if cancel != nil {
					cancel()
				}
				if err == nil {
					return conn, nil
				}
				slog.DebugContext(ctx, "dial attempt failed", "host", host, "ip", a.IP.String(), "error", err)
				dialErr = err
			}

			if dialErr != nil {
				return nil, fmt.Errorf("%w: failed to connect to host %s", ErrConnectionFailed, host)
			}
			return nil, ErrDestinationNotAllowed
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

// isBlockedPort checks if a port is on the blocked list
func isBlockedPort(port string) bool {
	blockedPorts := map[string]bool{
		"22":    true, // SSH
		"23":    true, // Telnet
		"25":    true, // SMTP
		"53":    true, // DNS
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
	return blockedPorts[port]
}

// IsMetadataIP checks if an IP is a known cloud metadata endpoint
func IsMetadataIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	metadataIPs := []string{
		"169.254.169.254", // AWS/Azure/GCP
		"100.100.100.200", // Alibaba Cloud
		"192.0.0.192",     // Oracle Cloud
	}
	ipStr := ip.String()
	for _, m := range metadataIPs {
		if ipStr == m {
			return true
		}
	}
	return false
}

// isPrivateDomainOrLiteral checks fast-path internal domain suffixes and literal IP ranges
func isPrivateDomainOrLiteral(hostname string) bool {
	hostnameLC := strings.ToLower(hostname)
	if hostnameLC == "localhost" || strings.HasPrefix(hostnameLC, "127.") {
		return true
	}
	if security.IsMetadataHost(hostnameLC) {
		return true
	}
	internalDomains := []string{".local", ".internal", ".corp", ".lan", ".localhost"}
	for _, domain := range internalDomains {
		if strings.HasSuffix(hostnameLC, domain) {
			return true
		}
	}
	if ip := net.ParseIP(hostname); ip != nil {
		return security.IsPrivateIPAddress(ip)
	}
	return false
}
