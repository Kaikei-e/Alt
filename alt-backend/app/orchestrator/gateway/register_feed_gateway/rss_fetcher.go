package register_feed_gateway

import (
	"alt/utils/constants"
	"alt/utils/logger"
	"alt/utils/proxy"
	"alt/utils/security"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/mmcdole/gofeed"
)

// maxFeedBodyBytes bounds how much of a feed body gofeed reads. gofeed treats a
// zero MaxByteSize as "no limit", and the transport transparently decompresses
// gzip, so without a ceiling a few KB on the wire can expand into GBs resident.
const maxFeedBodyBytes = 10 * 1024 * 1024

// ProxyConfig holds proxy configuration.
type ProxyConfig struct {
	ProxyURL string
	Enabled  bool
}

// getProxyConfigFromEnv retrieves proxy configuration from environment variables.
func getProxyConfigFromEnv() *ProxyConfig {
	proxyURL := os.Getenv("HTTP_PROXY")
	if proxyURL == "" {
		proxyURL = "http://nginx-external.alt-ingress.svc.cluster.local:8888"
	}

	proxyEnabled := os.Getenv("PROXY_ENABLED")
	enabled := proxyEnabled == "true"

	return &ProxyConfig{
		ProxyURL: proxyURL,
		Enabled:  enabled,
	}
}

// EnvoyProxyConfig holds Envoy proxy configuration.
type EnvoyProxyConfig struct {
	EnvoyURL string
	Enabled  bool
}

// getEnvoyProxyConfigFromEnv retrieves proxy configuration from environment variables.
func getEnvoyProxyConfigFromEnv() *EnvoyProxyConfig {
	strategy := proxy.GetStrategy()

	if !strategy.Enabled {
		return &EnvoyProxyConfig{
			EnvoyURL: "",
			Enabled:  false,
		}
	}

	return &EnvoyProxyConfig{
		EnvoyURL: strategy.BaseURL,
		Enabled:  strategy.Enabled,
	}
}

// RSSFeedFetcher interface for mocking RSS feed fetching.
type RSSFeedFetcher interface {
	FetchRSSFeed(ctx context.Context, link string) (*gofeed.Feed, error)
}

// DefaultRSSFeedFetcher implements RSSFeedFetcher with actual HTTP requests.
type DefaultRSSFeedFetcher struct {
	proxyConfig       *ProxyConfig
	envoyProxyConfig  *EnvoyProxyConfig
	proxyStrategy     *proxy.Strategy
	ssrfValidator     *security.SSRFValidator
	httpClient        *http.Client
	allowLoopbackDial bool
}

// NewDefaultRSSFeedFetcher creates a new DefaultRSSFeedFetcher with proxy configuration.
func NewDefaultRSSFeedFetcher() *DefaultRSSFeedFetcher {
	return newDefaultRSSFeedFetcher(false)
}

// newDefaultRSSFeedFetcher builds the fetcher with optional loopback dialing for tests.
func newDefaultRSSFeedFetcher(allowLoopbackDial bool) *DefaultRSSFeedFetcher {
	strategy := proxy.GetStrategy()
	f := &DefaultRSSFeedFetcher{
		proxyConfig:       getProxyConfigFromEnv(),
		envoyProxyConfig:  getEnvoyProxyConfigFromEnv(),
		proxyStrategy:     strategy,
		ssrfValidator:     security.NewSSRFValidator(),
		allowLoopbackDial: allowLoopbackDial,
	}

	baseDialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
		},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			return f.dialSecure(ctx, baseDialer, network, address)
		},
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 50,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 30 * time.Second,
	}

	roundTripper := proxy.WrapTransportForProxy(transport, f.proxyStrategy)
	f.httpClient = &http.Client{
		Timeout:   60 * time.Second,
		Transport: roundTripper,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			if security.IsFeedHostAllowed(req.URL.Hostname()) {
				return nil
			}
			if err := f.ssrfValidator.ValidateURL(req.Context(), req.URL); err != nil {
				return fmt.Errorf("redirect blocked by SSRF policy: %w", err)
			}
			return nil
		},
	}

	return f
}

// dialSecure replaces http.Transport's default dialer to do connection-time
// SSRF validation (ADR-000954 Wave 3 batch 3, capability catalog §2.F W3-F3).
//
// Validating the URL upfront (URLSecurityValidator) is necessary but not
// sufficient: a domain can pass inspection and then resolve to 169.254.169.254
// or 127.0.0.1 when DialContext runs (DNS rebinding / TOCTOU). dialSecure hooks
// http.Transport.DialContext, so it receives the actual (network, address) pair
// (pre-resolution), which lets it:
//
//  1. honour the FEED_ALLOWED_HOSTS operator-trust allow-list by hostname, the
//     same escape hatch CheckRedirect and URLSecurityValidator.isPrivateNetwork
//     use (this is what lets the staging/e2e stub, an allow-listed host that
//     resolves to a container-internal IP, be fetched); and
//  2. resolve the name itself, validate every candidate IP, and pin the
//     connection to the first IP that passes — so we connect to exactly the IP we
//     validated, closing the DNS-rebinding window.
func (f *DefaultRSSFeedFetcher) dialSecure(ctx context.Context, dialer *net.Dialer, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	// Proxy mode: the socket targets the trusted egress proxy, not the feed host,
	// so validating (and pinning) the address would reject the proxy's internal
	// address. Dial straight through, exactly as the old Control hook skipped.
	if f.proxyStrategy != nil && f.proxyStrategy.Enabled {
		return dialer.DialContext(ctx, network, address)
	}

	// Operator-allow-listed host (FEED_ALLOWED_HOSTS): an explicit trust decision,
	// exactly as on redirects (CheckRedirect) and on the first hop
	// (URLSecurityValidator.isPrivateNetwork). Dial through so an intentionally
	// private/loopback-resolving trusted host connects.
	if security.IsFeedHostAllowed(host) {
		return dialer.DialContext(ctx, network, address)
	}

	// Loopback dialing is test-only (httptest servers). Kept independent of the
	// hostname allow-list so redirect-to-internal stays blocked (see
	// allowLoopbackDial). false in production.
	if f.allowLoopbackDial && f.isLoopbackDialHost(ctx, host) {
		return dialer.DialContext(ctx, network, address)
	}

	// Resolve the name ourselves so we validate and connect to the SAME IP. An IP
	// literal resolves to itself, so metadata/private IP literals are validated too.
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}

	var blockErr error
	for _, addr := range addrs {
		pinned := net.JoinHostPort(addr.IP.String(), port)
		if verr := f.ssrfValidator.ValidateDialIP(network, pinned); verr != nil {
			if blockErr == nil {
				blockErr = verr
			}
			continue
		}
		// Pin to the exact IP we just validated; no re-resolution before connect.
		return dialer.DialContext(ctx, network, pinned)
	}

	if blockErr != nil {
		return nil, blockErr
	}
	return nil, &net.AddrError{Err: "no addresses resolved for host", Addr: host}
}

// isLoopbackDialHost reports whether host is (or resolves to) a loopback address.
// Handles the literal pre-resolution forms ("localhost", "127.x", "::1") and,
// for a name, resolves it and checks the results.
func (f *DefaultRSSFeedFetcher) isLoopbackDialHost(ctx context.Context, host string) bool {
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		if addr.IP.IsLoopback() {
			return true
		}
	}
	return false
}

func (f *DefaultRSSFeedFetcher) FetchRSSFeed(ctx context.Context, link string) (*gofeed.Feed, error) {
	if f.proxyStrategy == nil {
		logger.SafeWarnContext(ctx, "Proxy strategy not initialized, using direct connection")
		return f.fetchRSSFeedWithRetry(ctx, link)
	}

	logger.SafeInfoContext(ctx, "DEBUG: Proxy strategy configuration check",
		"strategy_mode", string(f.proxyStrategy.Mode),
		"strategy_enabled", f.proxyStrategy.Enabled,
		"strategy_base_url", f.proxyStrategy.BaseURL,
		"strategy_path_template", f.proxyStrategy.PathTemplate)

	if f.proxyStrategy.Enabled {
		proxyURL := proxy.ConvertToProxyURLWithContext(ctx, link, f.proxyStrategy)

		u, _ := url.Parse(link)
		expectedUpstream := u.Host

		logger.SafeInfoContext(ctx, "Using strategic proxy for RSS fetching",
			"strategy_mode", string(f.proxyStrategy.Mode),
			"original_url", link,
			"proxy_url", proxyURL,
			"expected_upstream", expectedUpstream)

		return f.fetchRSSFeedWithRetry(ctx, proxyURL)
	}

	logger.SafeInfoContext(ctx, "Using direct RSS feed connection (no proxy configured)",
		"original_url", link)

	return f.fetchRSSFeedWithRetry(ctx, link)
}

// fetchRSSFeedWithRetry performs RSS feed fetching with exponential backoff retry
func (f *DefaultRSSFeedFetcher) fetchRSSFeedWithRetry(ctx context.Context, link string) (*gofeed.Feed, error) {
	fp := gofeed.NewParser()
	fp.Client = f.httpClient
	fp.MaxByteSize = maxFeedBodyBytes
	fp.UserAgent = "Alt-RSS-Reader/1.0 (+https://alt.example.com)"

	feedCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var lastErr error
	for attempt := 0; attempt < constants.DefaultMaxRetries; attempt++ {
		if attempt > 0 {
			delay := calculateBackoff(attempt, constants.DefaultInitialDelay, constants.DefaultMaxDelay)

			logger.SafeInfoContext(feedCtx, "Retrying RSS feed fetch",
				"url", link,
				"attempt", attempt+1,
				"delay_seconds", delay.Seconds())

			select {
			case <-time.After(delay):
			case <-feedCtx.Done():
				return nil, feedCtx.Err()
			}
		}

		feed, err := fp.ParseURLWithContext(link, feedCtx)
		if err == nil {
			if attempt > 0 {
				logger.SafeInfoContext(feedCtx, "RSS feed fetch succeeded after retry",
					"url", link,
					"attempts", attempt+1)
			}
			return feed, nil
		}

		lastErr = err
		if !isRetryableError(err) {
			logger.SafeWarnContext(feedCtx, "Non-retryable error, not retrying",
				"url", link,
				"error", err.Error())
			break
		}

		logger.SafeWarnContext(feedCtx, "RSS feed fetch failed, will retry",
			"url", link,
			"attempt", attempt+1,
			"error", err.Error())
	}

	return nil, lastErr
}
