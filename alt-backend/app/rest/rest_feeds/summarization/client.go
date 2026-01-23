package summarization

import (
	"net"
	"net/http"
	"time"
)

// sharedStreamClient is a connection-pooled HTTP client for streaming requests.
// Using a shared client enables connection reuse (keep-alive), reducing TTFT by
// eliminating TCP/TLS handshake overhead on subsequent requests.
var sharedStreamClient *http.Client

func init() {
	sharedStreamClient = &http.Client{
		Timeout: 0, // No timeout for streaming requests
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			// Enable HTTP/2 for better multiplexing
			ForceAttemptHTTP2: true,
		},
	}
}

// GetStreamClient returns the shared HTTP client for streaming requests.
// This client is configured for connection pooling and has no timeout,
// suitable for long-running streaming operations.
func GetStreamClient() *http.Client {
	return sharedStreamClient
}
