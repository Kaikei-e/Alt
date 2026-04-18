// Package tlsutil provides rag-orchestrator's outbound mTLS helpers. The
// leaf cert/key are re-read from disk whenever their mtimes advance so the
// pki-agent sidecar can rotate the underlying files without a process
// restart. The shape mirrors the reference implementation in
// `alt-backend/app/tlsutil/tlsutil.go` (the Alt platform convention); the
// duplication is intentional — each service module owns its TLS helpers
// locally until a shared Go module is introduced.
package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// certReloader caches a parsed tls.Certificate and re-reads the underlying
// PEM files whenever either file's mtime advances. A single mutex protects
// both the cached certificate and the observed mtimes.
type certReloader struct {
	certPath string
	keyPath  string

	mu      sync.Mutex
	cert    *tls.Certificate
	certMod time.Time
	keyMod  time.Time
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
		if r.cert != nil {
			return r.cert, nil
		}
		return nil, fmt.Errorf("stat cert %q: %w", r.certPath, err)
	}
	keyStat, err := os.Stat(r.keyPath)
	if err != nil {
		if r.cert != nil {
			return r.cert, nil
		}
		return nil, fmt.Errorf("stat key %q: %w", r.keyPath, err)
	}

	if r.cert != nil && !certStat.ModTime().After(r.certMod) && !keyStat.ModTime().After(r.keyMod) {
		return r.cert, nil
	}

	cert, err := tls.LoadX509KeyPair(r.certPath, r.keyPath)
	if err != nil {
		// Fall back to the last good certificate so a transient mid-rotation
		// failure (truncated file, key/cert mismatch window) cannot take the
		// client down. The initial load still surfaces the error.
		if r.cert != nil {
			return r.cert, nil
		}
		return nil, fmt.Errorf("load x509 keypair: %w", err)
	}

	r.cert = &cert
	r.certMod = certStat.ModTime()
	r.keyMod = keyStat.ModTime()
	return r.cert, nil
}

func loadRootCAs(caPath string) (*x509.CertPool, error) {
	// caPath is sourced from service configuration (mTLS CA bundle path),
	// never user input.
	pem, err := os.ReadFile(filepath.Clean(caPath)) //#nosec G304 -- caPath is operator-configured CA bundle location
	if err != nil {
		return nil, fmt.Errorf("read ca bundle %q: %w", caPath, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("ca bundle %q did not contain any valid PEM certificates", caPath)
	}
	return pool, nil
}

// LoadClientConfig returns a *tls.Config for an mTLS-capable HTTP client.
// The client presents its leaf cert via GetClientCertificate (re-read on
// mtime change) and trusts the supplied CA bundle as RootCAs.
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
