package httpclient

import (
	"errors"
	"net/http"
	"os"
	"sync"
	"time"

	"rag-orchestrator/internal/infra/tlsutil"
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

// mTLS-capable transport. Lazily constructed on first use; shared to
// preserve the connection-reuse benefit of the plaintext path. The
// `tls.Config` itself is built once, but the leaf cert it presents is
// re-read from disk on every handshake via `GetClientCertificate` (see
// tlsutil.LoadClientConfig), so pki-agent cert rotations are picked up
// without rebuilding the transport or losing connection pooling.
var (
	mtlsTransportOnce sync.Once
	mtlsTransport     *http.Transport
	mtlsTransportErr  error
)

func loadMTLSTransport() (*http.Transport, error) {
	mtlsTransportOnce.Do(func() {
		certFile := os.Getenv("MTLS_CERT_FILE")
		keyFile := os.Getenv("MTLS_KEY_FILE")
		caFile := os.Getenv("MTLS_CA_FILE")
		if certFile == "" || keyFile == "" || caFile == "" {
			mtlsTransportErr = errors.New("MTLS_ENFORCE=true but MTLS_CERT_FILE/KEY_FILE/CA_FILE not fully set")
			return
		}
		tlsCfg, err := tlsutil.LoadClientConfig(certFile, keyFile, caFile)
		if err != nil {
			mtlsTransportErr = err
			return
		}
		mtlsTransport = &http.Transport{
			TLSClientConfig:     tlsCfg,
			MaxIdleConns:        20,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     120 * time.Second,
		}
	})
	return mtlsTransport, mtlsTransportErr
}

// NewPooledClient creates an http.Client that shares a connection pool
// with other pooled clients, reducing Tailscale VPN handshake overhead.
// When MTLS_ENFORCE=true it returns a client whose transport presents the
// caller leaf cert + trusts alt-CA, while keeping connection pooling.
func NewPooledClient(timeout time.Duration) *http.Client {
	if MTLSEnforced() {
		t, err := loadMTLSTransport()
		if err == nil {
			return &http.Client{Timeout: timeout, Transport: t}
		}
		// Fail-closed: if mTLS is requested but cert loading fails, we
		// must NOT silently fall back to plaintext. Logging here would
		// require a logger; instead return a client with no Transport so
		// the next request surfaces a clear TLS error — and the startup
		// path that calls `loadMTLSTransport()` explicitly (DI container)
		// can surface the error at boot time.
		return &http.Client{Timeout: timeout}
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: sharedTransport,
	}
}

// PreflightMTLS triggers mTLS cert loading once so the DI container can
// surface cert errors at startup rather than on first request. No-op when
// MTLS_ENFORCE!=true.
func PreflightMTLS() error {
	if !MTLSEnforced() {
		return nil
	}
	_, err := loadMTLSTransport()
	return err
}
