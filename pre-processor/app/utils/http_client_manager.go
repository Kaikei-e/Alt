package utils

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// HTTPClientManager manages reusable HTTP clients
type HTTPClientManager struct {
	defaultClient *http.Client
	summaryClient *http.Client
	feedClient    *http.Client
}

// optimizedTransport wraps http.Transport to expose fields for testing
type optimizedTransport struct {
	*http.Transport
}

var (
	instance *HTTPClientManager
	mu       sync.Mutex
)

// NewHTTPClientManager returns a singleton instance of HTTPClientManager
func NewHTTPClientManager() *HTTPClientManager {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		instance = &HTTPClientManager{}
		instance.init()
	}

	return instance
}

func (m *HTTPClientManager) init() {
	const shortResponseHeaderTimeout = 20 * time.Second
	m.defaultClient = m.createOptimizedClient(30*time.Second, shortResponseHeaderTimeout)
	// Summary client: Client.Timeout=0 to delegate timeout control to context.WithTimeout in driver/summarizer_api.go (NEWS_CREATOR_TIMEOUT, default 600s).
	// ResponseHeaderTimeout=0 (disabled) because hierarchical Map-Reduce summarization in news-creator can take several minutes; a short per-header timeout causes client-side retry storms while the upstream request continues in flight.
	m.summaryClient = m.createOptimizedClient(0, 0)
	m.feedClient = m.createOptimizedClient(15*time.Second, shortResponseHeaderTimeout)
}

// GetDefaultClient returns the default HTTP client
func (m *HTTPClientManager) GetDefaultClient() *http.Client {
	return m.defaultClient
}

// GetSummaryClient returns HTTP client optimized for summary API calls
func (m *HTTPClientManager) GetSummaryClient() *http.Client {
	return m.summaryClient
}

// GetFeedClient returns HTTP client optimized for feed fetching
func (m *HTTPClientManager) GetFeedClient() *http.Client {
	return m.feedClient
}

func (m *HTTPClientManager) createOptimizedClient(timeout, responseHeaderTimeout time.Duration) *http.Client {
	// Layered timeouts: Client.Timeout caps the whole request lifecycle, while
	// Transport fields bound each phase so a slow peer cannot tie up a slot
	// (Cloudflare's "net/http default will break your production" pattern).
	// ResponseHeaderTimeout is parameterized because summary callers stream
	// from a multi-minute Map-Reduce pipeline and need it disabled.
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := &optimizedTransport{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment, // 統一プロキシ戦略サポート（HTTP_PROXY環境変数）
			DialContext:           dialer.DialContext,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: responseHeaderTimeout,
			ExpectContinueTimeout: 1 * time.Second,
			DisableKeepAlives:     false,
			DisableCompression:    false,
			ForceAttemptHTTP2:     true,
		},
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}
