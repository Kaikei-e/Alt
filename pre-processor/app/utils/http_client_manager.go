package utils

import (
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
	m.defaultClient = m.createOptimizedClient(30 * time.Second)
	// Summary client timeout is set to 120s (2 minutes) to fail fast when news-creator/Ollama is unresponsive
	// The actual timeout is controlled by context timeout in driver/summarizer_api.go
	m.summaryClient = m.createOptimizedClient(120 * time.Second)
	m.feedClient = m.createOptimizedClient(15 * time.Second)
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

func (m *HTTPClientManager) createOptimizedClient(timeout time.Duration) *http.Client {
	transport := &optimizedTransport{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment, // 統一プロキシ戦略サポート（HTTP_PROXY環境変数）
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
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
