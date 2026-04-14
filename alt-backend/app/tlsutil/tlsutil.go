// Package tlsutil provides shared TLS helpers for the Alt platform's east-west
// mTLS rollout. Certificates are loaded from disk via GetCertificate /
// GetClientCertificate callbacks so that a step-ca renewer sidecar can rotate
// the underlying files without restarting the process.
package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

// certReloader caches a parsed tls.Certificate and re-reads the underlying PEM
// files whenever either file's mtime advances. A single mutex protects both
// the cached certificate and the observed mtimes.
type certReloader struct {
	certPath string
	keyPath  string

	mu       sync.Mutex
	cert     *tls.Certificate
	certMod  time.Time
	keyMod   time.Time
}

func newCertReloader(certPath, keyPath string) (*certReloader, error) {
	r := &certReloader{certPath: certPath, keyPath: keyPath}
	if _, err := r.load(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *certReloader) load() (*tls.Certificate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	certStat, err := os.Stat(r.certPath)
	if err != nil {
		return nil, fmt.Errorf("stat cert %q: %w", r.certPath, err)
	}
	keyStat, err := os.Stat(r.keyPath)
	if err != nil {
		return nil, fmt.Errorf("stat key %q: %w", r.keyPath, err)
	}

	if r.cert != nil && !certStat.ModTime().After(r.certMod) && !keyStat.ModTime().After(r.keyMod) {
		return r.cert, nil
	}

	cert, err := tls.LoadX509KeyPair(r.certPath, r.keyPath)
	if err != nil {
		return nil, fmt.Errorf("load x509 keypair: %w", err)
	}

	r.cert = &cert
	r.certMod = certStat.ModTime()
	r.keyMod = keyStat.ModTime()
	return r.cert, nil
}

func loadRootCAs(caPath string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("read ca bundle %q: %w", caPath, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("ca bundle %q did not contain any valid PEM certificates", caPath)
	}
	return pool, nil
}

// LoadServerConfig returns a *tls.Config wired up for a mTLS-capable server.
// In Phase 1 the caller sets ClientAuth explicitly (NoClientCert for
// server-only rollout, RequireAndVerifyClientCert when flipping enforcement).
// The config re-reads the cert/key files whenever their mtime advances, so
// a sidecar renewer can rotate the leaf without a process restart.
func LoadServerConfig(certPath, keyPath, caPath string) (*tls.Config, error) {
	reloader, err := newCertReloader(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	clientCAs, err := loadRootCAs(caPath)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS13,
		ClientCAs:  clientCAs,
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			return reloader.load()
		},
	}, nil
}

// LoadClientConfig returns a *tls.Config for an mTLS-capable HTTP client. The
// client presents its leaf cert via GetClientCertificate (re-read on mtime
// change) and trusts the supplied CA bundle as RootCAs.
func LoadClientConfig(certPath, keyPath, caPath string) (*tls.Config, error) {
	reloader, err := newCertReloader(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	rootCAs, err := loadRootCAs(caPath)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS13,
		RootCAs:    rootCAs,
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return reloader.load()
		},
	}, nil
}

// NewMTLSHTTPServer wraps a handler in an http.Server with timeouts tuned for
// a mTLS rollout. IdleTimeout is bounded to 60s so that HTTP/2 connection
// reuse cannot outlive a 24h leaf certificate by more than a negligible
// window — the renewer sidecar rotates at 8h remaining, giving a wide margin.
func NewMTLSHTTPServer(addr string, tlsConfig *tls.Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
