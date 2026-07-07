package httpclient

import (
	"errors"
	"fmt"
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

// failClosedTransport makes every request fail immediately instead of
// falling back to http.DefaultTransport. A bare `&http.Client{}` (nil
// Transport) does NOT surface a TLS error for plaintext `http://` targets —
// it just completes the request over plaintext, which is exactly the
// silent-fallback bug this type exists to prevent (see NewPooledClient).
type failClosedTransport struct {
	loadErr error
}

func (t *failClosedTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("mTLS transport unavailable, refusing plaintext fallback: %w", t.loadErr)
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
		// Fail-closed: if mTLS is requested but cert loading fails, every
		// request through this client must error rather than silently
		// completing in plaintext. This matters even for callers that
		// never call PreflightMTLS() first (e.g. cmd/backfill --direct
		// mode) — the fail-closed guarantee must not depend on the
		// composition root having preflighted the cert load.
		return &http.Client{Timeout: timeout, Transport: &failClosedTransport{loadErr: err}}
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
