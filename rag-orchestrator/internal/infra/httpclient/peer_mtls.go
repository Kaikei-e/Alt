package httpclient

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"rag-orchestrator/internal/infra/tlsutil"
)

// PeerMTLSConfig is the cert material and peer identity needed to dial a
// listener that only speaks mutual TLS: alt-data-hub, and search-indexer's
// :9443. Every field but ServerName is required: those listeners always
// demand and verify a peer certificate, so a partial configuration cannot
// degrade into a working plaintext call — it can only produce a handshake
// failure on the first request, which is much harder to read than a startup
// error.
type PeerMTLSConfig struct {
	CertFile string
	KeyFile  string
	CAFile   string
	// ServerName pins tls.Config.ServerName. Empty means "derive from the
	// URL host", which is crypto/tls's own behaviour and is correct whenever
	// the dial host is one of the peer leaf's SANs (alt-data-hub and
	// search-indexer mint CERT_SANS=<service>,localhost). Override only when
	// the dial target is not the SAN (an IP, a tunnel endpoint, a staging
	// alias).
	ServerName string
}

// NewPeerMTLSClient returns an *http.Client that presents the
// rag-orchestrator leaf certificate and trusts only the alt CA.
//
// It deliberately does NOT go through NewPooledClient / loadMTLSTransport.
// Those share one process-global transport whose tls.Config carries no
// ServerName, so pinning one there would pin it for every other mTLS peer
// rag-orchestrator talks to. This builds a transport dedicated to one peer
// instead; connection pooling is preserved within it.
//
// There is no MTLS_ENFORCE escape hatch here, unlike NewPooledClient. The
// peers this dials have no plaintext business surface left: alt-backend:9102
// became an admin-only operator listener in the 3-binary split (ADR-000954
// D1), and search-indexer's plaintext :9300 answers health checks only. "mTLS
// off" would not be a degraded mode, it would be a guaranteed 404. Making the
// transport unconditional keeps a missing cert an explicit startup failure
// rather than a silent fallback onto a dead endpoint (CLAUDE.md rules 8 and 9).
//
// HTTP/1.1 is what this negotiates: Go's http.Transport only attempts HTTP/2
// automatically when TLSClientConfig is nil or ForceAttemptHTTP2 is set.
// Every call made through it is unary (Connect RPCs to alt-data-hub, REST GETs
// to search-indexer), which HTTP/1.1 carries fine.
func NewPeerMTLSClient(cfg PeerMTLSConfig, timeout time.Duration) (*http.Client, error) {
	if cfg.CertFile == "" || cfg.KeyFile == "" || cfg.CAFile == "" {
		return nil, errors.New(
			"peer mTLS client: MTLS_CERT_FILE, MTLS_KEY_FILE and MTLS_CA_FILE are all required " +
				"(the peer requires and verifies a client certificate; there is no plaintext fallback)")
	}

	// LoadClientConfig re-reads the leaf on every handshake via
	// GetClientCertificate, so in-process enrollment can rotate
	// /certs/svc-cert.pem underneath a long-lived process without a restart.
	tlsCfg, err := tlsutil.LoadClientConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("peer mTLS client (fail-closed): %w", err)
	}
	if cfg.ServerName != "" {
		tlsCfg.ServerName = cfg.ServerName
	}

	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig:     tlsCfg,
			MaxIdleConns:        20,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     120 * time.Second,
		},
	}, nil
}
