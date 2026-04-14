// Package tlsutil provides shared TLS helpers for east-west mTLS.
// Canonical implementation lives in alt-backend/app/tlsutil.
package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// OptionsFromEnv derives ServerOptions from MTLS_CLIENT_AUTH and MTLS_ALLOWED_PEERS.
func OptionsFromEnv() []ServerOption {
	var opts []ServerOption
	if strings.EqualFold(os.Getenv("MTLS_CLIENT_AUTH"), "require_and_verify") {
		opts = append(opts, WithClientAuth(tls.RequireAndVerifyClientCert))
	}
	if v := os.Getenv("MTLS_ALLOWED_PEERS"); v != "" {
		parts := strings.Split(v, ",")
		cleaned := make([]string, 0, len(parts))
		for _, p := range parts {
			if s := strings.TrimSpace(p); s != "" {
				cleaned = append(cleaned, s)
			}
		}
		if len(cleaned) > 0 {
			opts = append(opts, WithAllowedPeers(cleaned...))
		}
	}
	return opts
}

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
	pem, err := os.ReadFile(filepath.Clean(caPath)) // #nosec G304 -- caPath is operator-controlled CA bundle path
	if err != nil {
		return nil, fmt.Errorf("read ca bundle %q: %w", caPath, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("ca bundle %q did not contain any valid PEM certificates", caPath)
	}
	return pool, nil
}

type ServerOption func(*serverOpts)

type serverOpts struct {
	clientAuth   tls.ClientAuthType
	allowedPeers map[string]struct{}
}

func WithClientAuth(c tls.ClientAuthType) ServerOption {
	return func(o *serverOpts) { o.clientAuth = c }
}

func WithAllowedPeers(names ...string) ServerOption {
	return func(o *serverOpts) {
		if o.allowedPeers == nil {
			o.allowedPeers = make(map[string]struct{}, len(names))
		}
		for _, n := range names {
			if n == "" {
				continue
			}
			o.allowedPeers[n] = struct{}{}
		}
	}
}

// LoadServerConfig returns a *tls.Config wired up for a mTLS-capable server.
func LoadServerConfig(certPath, keyPath, caPath string, opts ...ServerOption) (*tls.Config, error) {
	reloader, err := newCertReloader(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	clientCAs, err := loadRootCAs(caPath)
	if err != nil {
		return nil, err
	}
	o := &serverOpts{}
	for _, opt := range opts {
		opt(o)
	}
	cfg := &tls.Config{
		MinVersion: tls.VersionTLS13,
		ClientAuth: o.clientAuth,
		ClientCAs:  clientCAs,
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			return reloader.load()
		},
	}
	if len(o.allowedPeers) > 0 {
		allowed := o.allowedPeers
		cfg.VerifyConnection = func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return fmt.Errorf("tlsutil: peer presented no certificate (allowlist configured)")
			}
			leaf := cs.PeerCertificates[0]
			candidates := append([]string{leaf.Subject.CommonName}, leaf.DNSNames...)
			for _, c := range candidates {
				if _, ok := allowed[c]; ok {
					return nil
				}
			}
			return fmt.Errorf("tlsutil: peer identity %q not in allowlist", leaf.Subject.CommonName)
		}
	}
	return cfg, nil
}

// LoadClientConfig returns a *tls.Config for an mTLS-capable HTTP client.
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
// east-west mTLS: bounded IdleTimeout prevents HTTP/2 connection reuse from
// outliving a short-lived leaf certificate.
func NewMTLSHTTPServer(addr string, tlsConfig *tls.Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
