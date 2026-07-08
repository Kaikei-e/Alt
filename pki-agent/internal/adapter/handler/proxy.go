package handler

import (
	"crypto/tls"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

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
	proxy.Transport = &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 5 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Error("proxy upstream request failed", "upstream", cfg.Upstream, "error", err)
		w.WriteHeader(http.StatusBadGateway)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always strip caller-supplied identity header first: only a
		// verified client certificate may set it, otherwise an
		// unauthenticated caller could forge X-Alt-Peer-Identity and have
		// it forwarded to upstream as-is.
		r.Header.Del("X-Alt-Peer-Identity")
		if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
			r.Header.Set("X-Alt-Peer-Identity", r.TLS.PeerCertificates[0].Subject.CommonName)
		}
		r.Header.Set("X-Forwarded-Proto", "https")
		proxy.ServeHTTP(w, r)
	})

	return infrastructure.NewMTLSHTTPServer(cfg.Listen, tlsCfg, handler), nil
}
