package proxy

import (
	"net/http"
	"net/url"
	"strings"

	"alt/utils/logger"
)

// EnvoyProxyRoundTripper is an http.RoundTripper that fixes Host headers
// for Envoy Dynamic Forward Proxy requests.
//
// When using Envoy as a forward proxy, requests are sent to the Envoy proxy
// with a path like /proxy/https://target.com/path. However, the Host header
// needs to be set to the target domain for proper TLS SNI (Server Name Indication).
//
// This RoundTripper:
//  1. Detects proxy requests (paths containing /proxy/https:// or /proxy/http://)
//  2. Extracts the target domain from the proxy path
//  3. Sets the Host header and X-Target-Domain header to the target domain
type EnvoyProxyRoundTripper struct {
	// Transport is the underlying RoundTripper to use for actual requests.
	// If nil, http.DefaultTransport is used.
	Transport http.RoundTripper
}

// RoundTrip implements http.RoundTripper.
// It modifies proxy requests to set the correct Host header for Envoy Dynamic Forward Proxy.
func (ert *EnvoyProxyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	transport := ert.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}

	// Check if this is an Envoy proxy request (/proxy/https://domain.com/path)
	if IsProxyPath(req.URL.Path) {
		// Extract target domain from proxy path
		targetHost, ok := ExtractTargetHost(req.URL.Path)
		if ok {
			// Set Host header to target domain for proper TLS SNI
			req.Host = targetHost
			req.Header.Set("Host", targetHost)
			// CRITICAL FIX: Add X-Target-Domain header required by Envoy proxy route matching
			req.Header.Set("X-Target-Domain", targetHost)

			logger.SafeInfoContext(req.Context(), "Fixed Host header for Envoy Dynamic Forward Proxy",
				"original_host", req.URL.Host,
				"target_host", targetHost,
				"request_url", req.URL.String())
		}
	}

	return transport.RoundTrip(req)
}

// NewEnvoyProxyRoundTripper creates a new EnvoyProxyRoundTripper with the given transport.
// If transport is nil, http.DefaultTransport will be used.
func NewEnvoyProxyRoundTripper(transport http.RoundTripper) *EnvoyProxyRoundTripper {
	return &EnvoyProxyRoundTripper{
		Transport: transport,
	}
}

// WrapTransportForProxy wraps the given transport with EnvoyProxyRoundTripper
// if the strategy is enabled. Returns the original transport if strategy is nil or disabled.
func WrapTransportForProxy(transport http.RoundTripper, strategy *Strategy) http.RoundTripper {
	if strategy == nil || !strategy.Enabled {
		return transport
	}
	return NewEnvoyProxyRoundTripper(transport)
}

// extractTargetURL extracts the full target URL from a proxy path.
// For paths like "/proxy/https://example.com/path?query=1", returns "https://example.com/path?query=1".
func extractTargetURL(path string) string {
	// Check for proxy prefixes
	prefixes := []string{"/proxy/", "/rss-proxy/"}

	for _, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) {
			return path[len(prefix):]
		}
		// Also check for prefix appearing after initial path
		idx := strings.Index(path, prefix)
		if idx != -1 {
			return path[idx+len(prefix):]
		}
	}

	return ""
}

// ParseProxyTargetURL parses the target URL from a proxy path and returns the parsed URL.
// Returns nil if the path is not a valid proxy path or parsing fails.
func ParseProxyTargetURL(path string) *url.URL {
	targetURLStr := extractTargetURL(path)
	if targetURLStr == "" {
		return nil
	}

	u, err := url.Parse(targetURLStr)
	if err != nil {
		return nil
	}

	return u
}
