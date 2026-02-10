package httpclient

import (
	"net/http"
	"time"
)

// sharedTransport is reused across all pooled clients to maximize
// connection reuse. This is especially important for Tailscale VPN
// connections where each new TCP handshake adds 5-20ms overhead.
var sharedTransport = &http.Transport{
	MaxIdleConns:        20,
	MaxIdleConnsPerHost: 10,
	IdleConnTimeout:     120 * time.Second,
	DisableKeepAlives:   false,
}

// NewPooledClient creates an http.Client that shares a connection pool
// with other pooled clients, reducing Tailscale VPN handshake overhead.
func NewPooledClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: sharedTransport,
	}
}
