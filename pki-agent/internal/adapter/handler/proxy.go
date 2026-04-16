package handler

import (
	"crypto/tls"
	"net/http"
	"net/http/httputil"
	"net/url"

	"pki-agent/internal/infrastructure"
)

// ProxyConfig describes an optional mTLS reverse proxy. When PROXY_LISTEN is
// set, pki-agent starts a TLS listener that terminates mTLS and forwards
// plaintext HTTP to PROXY_UPSTREAM. Cert hot-reload is handled by
// tlsutil.certReloader (mtime-based, checked every TLS handshake).
type ProxyConfig struct {
	Listen       string
	Upstream     string
	CertPath     string
	KeyPath      string
	CAPath       string
	VerifyClient bool
	AllowedPeers []string
}

// NewTLSProxy builds an http.Server that terminates mTLS and reverse-proxies
// to an upstream HTTP endpoint. The peer CN is injected into the
// X-Alt-Peer-Identity header so upstream (Python) handlers can audit caller
// identity — same contract as the nginx TLS sidecar it replaces.
func NewTLSProxy(cfg ProxyConfig) (*http.Server, error) {
	var opts []infrastructure.ServerOption
	if cfg.VerifyClient {
		opts = append(opts, infrastructure.WithClientAuth(tls.RequireAndVerifyClientCert))
	}
	if len(cfg.AllowedPeers) > 0 {
		opts = append(opts, infrastructure.WithAllowedPeers(cfg.AllowedPeers...))
	}
	tlsCfg, err := infrastructure.LoadServerConfig(cfg.CertPath, cfg.KeyPath, cfg.CAPath, opts...)
	if err != nil {
		return nil, err
	}

	target, err := url.Parse(cfg.Upstream)
	if err != nil {
		return nil, err
	}
	proxy := httputil.NewSingleHostReverseProxy(target)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
			r.Header.Set("X-Alt-Peer-Identity", r.TLS.PeerCertificates[0].Subject.CommonName)
		}
		r.Header.Set("X-Forwarded-Proto", "https")
		proxy.ServeHTTP(w, r)
	})

	return infrastructure.NewMTLSHTTPServer(cfg.Listen, tlsCfg, handler), nil
}
