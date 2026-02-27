// Package client provides HTTP clients for communicating with backend services.
package client

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/http2"

	"alt-butterfly-facade/internal/middleware"
)

// BackendClient is an HTTP client for forwarding requests to alt-backend.
type BackendClient struct {
	baseURL          string
	httpClient       *http.Client
	streamingClient  *http.Client
	requestTimeout   time.Duration
	streamingTimeout time.Duration
}

// NewBackendClient creates a new backend client with HTTP/2 support for h2c.
func NewBackendClient(baseURL string, requestTimeout, streamingTimeout time.Duration) *BackendClient {
	return NewBackendClientWithTransport(baseURL, requestTimeout, streamingTimeout, nil)
}

// NewBackendClientWithTransport creates a new backend client with a custom transport.
// If transport is nil, uses HTTP/2 cleartext (h2c) transport for Connect-RPC.
func NewBackendClientWithTransport(baseURL string, requestTimeout, streamingTimeout time.Duration, transport http.RoundTripper) *BackendClient {
	if transport == nil {
		// HTTP/2 transport for Connect-RPC (h2c - HTTP/2 cleartext)
		transport = &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, network, addr)
			},
		}
	}

	return &BackendClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Transport: transport,
			// Timeout is intentionally not set (0 = no timeout).
			// Timeouts are controlled per-request via context deadline,
			// derived from Connect-Timeout-Ms header by the handler layer.
		},
		streamingClient: &http.Client{
			Transport: transport,
			Timeout:   streamingTimeout,
		},
		requestTimeout:   requestTimeout,
		streamingTimeout: streamingTimeout,
	}
}

// DefaultTimeout returns the configured default request timeout.
// Handlers use this as the fallback when Connect-Timeout-Ms header is absent.
func (c *BackendClient) DefaultTimeout() time.Duration {
	return c.requestTimeout
}

// ForwardRequest forwards an HTTP request to the backend with the given token.
func (c *BackendClient) ForwardRequest(req *http.Request, token string) (*http.Response, error) {
	// Build backend URL
	backendURL := c.BuildBackendURL(req.URL.Path)

	// Create new request to backend
	backendReq, err := http.NewRequestWithContext(req.Context(), req.Method, backendURL, req.Body)
	if err != nil {
		return nil, err
	}

	// Copy relevant headers
	copyHeaders(req.Header, backendReq.Header)

	// Set authentication token
	backendReq.Header.Set(middleware.BackendTokenHeader, token)

	return c.httpClient.Do(backendReq)
}

// ForwardStreamingRequest forwards a streaming request to the backend.
func (c *BackendClient) ForwardStreamingRequest(req *http.Request, token string) (*http.Response, error) {
	backendURL := c.BuildBackendURL(req.URL.Path)

	backendReq, err := http.NewRequestWithContext(req.Context(), req.Method, backendURL, req.Body)
	if err != nil {
		return nil, err
	}

	copyHeaders(req.Header, backendReq.Header)
	backendReq.Header.Set(middleware.BackendTokenHeader, token)

	return c.streamingClient.Do(backendReq)
}

// BuildBackendURL constructs the full backend URL from a path.
func (c *BackendClient) BuildBackendURL(path string) string {
	if strings.HasPrefix(path, "/") {
		return c.baseURL + path
	}
	return c.baseURL + "/" + path
}

// copyHeaders copies relevant headers from source to destination.
func copyHeaders(src, dst http.Header) {
	// Headers to copy for Connect-RPC
	// Note: Accept-Encoding is intentionally excluded.
	// Go's http2.Transport does not auto-decompress when Accept-Encoding is set manually,
	// which causes gzip-compressed responses to be forwarded without decompression.
	// See: https://github.com/golang/go/issues/13298
	headersToForward := []string{
		"Content-Type",
		"Accept",
		"Connect-Protocol-Version",
		"Connect-Timeout-Ms",
		"Grpc-Timeout",
		"X-Service-Token",
	}

	for _, h := range headersToForward {
		if v := src.Get(h); v != "" {
			dst.Set(h, v)
		}
	}
}
