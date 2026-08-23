// Package tlsutil provides shared TLS helpers for the Alt platform's east-west
// mTLS rollout. Certificates are loaded from disk via GetCertificate /
// GetClientCertificate callbacks so that a step-ca renewer sidecar can rotate
// the underlying files without restarting the process.
//
// Canonical implementation lives in alt-backend/app/tlsutil. This copy is kept
// in sync for the auth-hub Go module.
package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// OptionsFromEnv derives Phase 2 ServerOptions from env:
//   - MTLS_CLIENT_AUTH=require_and_verify
//   - MTLS_ALLOWED_PEERS=csv
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

// ClientAuthLabel returns an operator-readable name for a tls.ClientAuthType,
// so the startup log can state the effective client-auth mode of the mTLS
// listener rather than an opaque integer.
func ClientAuthLabel(mode tls.ClientAuthType) string {
	switch mode {
	case tls.NoClientCert:
		return "no_client_cert"
	case tls.RequestClientCert:
		return "request_client_cert"
	case tls.RequireAnyClientCert:
		return "require_any_client_cert"
	case tls.VerifyClientCertIfGiven:
		return "verify_client_cert_if_given"
	case tls.RequireAndVerifyClientCert:
		return "require_and_verify_client_cert"
	default:
		return fmt.Sprintf("unknown(%d)", int(mode))
	}
}

// ClientAuthEnforced reports whether mode both requires a client certificate
// and verifies it against the configured CAs. Only RequireAndVerifyClientCert
// closes the door on unauthenticated peers; every other mode (including the
// zero-value NoClientCert default) lets an unauthenticated client complete the
// handshake, which is what the startup warning must surface.
func ClientAuthEnforced(mode tls.ClientAuthType) bool {
	return mode == tls.RequireAndVerifyClientCert
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
// a mTLS rollout. IdleTimeout is bounded to 60s so that HTTP/2 connection
// reuse cannot outlive a 24h leaf certificate by more than a negligible window.
func NewMTLSHTTPServer(addr string, tlsConfig *tls.Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
