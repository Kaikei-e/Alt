package httpclient

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"rag-orchestrator/internal/infra/tlsutil"
)

// DataHubTransportConfig is the cert material and peer identity needed to
// dial alt-data-hub's mTLS listener. Every field is required: alt-data-hub
// (cmd/datahub) always demands and verifies a peer certificate, so a partial
// configuration cannot degrade into a working plaintext call — it can only
// produce a handshake failure on the first request, which is much harder to
// read than a startup error.
type DataHubTransportConfig struct {
	CertFile string
	KeyFile  string
	CAFile   string
	// ServerName pins tls.Config.ServerName. Empty means "derive from the
	// URL host", which is crypto/tls's own behaviour and is correct whenever
	// DATAHUB_MTLS_URL is https://alt-data-hub:9443 — alt-data-hub's leaf is
	// minted with CERT_SANS=alt-data-hub. Override only when the dial target
	// is not the SAN (an IP, a tunnel endpoint, a staging alias).
	ServerName string
}

// NewDataHubClient returns an *http.Client that presents the rag-orchestrator
// leaf certificate to alt-data-hub and trusts only the alt CA.
//
// It deliberately does NOT go through NewPooledClient / loadMTLSTransport.
// Those share one process-global transport whose tls.Config carries no
// ServerName, so pinning one there would pin it for every other mTLS peer
// rag-orchestrator talks to. This builds a transport dedicated to the
// data-hub connection instead; connection pooling is preserved within it.
//
// There is no MTLS_ENFORCE escape hatch here, unlike the pre-processor and
// search-indexer backend clients. Those still have a plaintext alt-backend
// port to fall back to; rag-orchestrator does not. The plaintext internal
// surface it used to call (alt-backend:9102) became an admin-only operator
// listener in the 3-binary split (ADR-000954 D1), so "mTLS off" is not a
// degraded mode, it is a guaranteed 404. Making the transport unconditional
// keeps a missing cert an explicit startup failure rather than a silent
// fallback onto a dead endpoint (CLAUDE.md rules 8 and 9).
//
// HTTP/1.1 is what this negotiates: Go's http.Transport only attempts HTTP/2
// automatically when TLSClientConfig is nil or ForceAttemptHTTP2 is set, and
// every DataHubService RPC rag-orchestrator calls is unary, which Connect
// carries fine over HTTP/1.1. This matches how pre-processor and
// search-indexer dial the same listener.
func NewDataHubClient(cfg DataHubTransportConfig, timeout time.Duration) (*http.Client, error) {
	if cfg.CertFile == "" || cfg.KeyFile == "" || cfg.CAFile == "" {
		return nil, errors.New(
			"data-hub mTLS client: MTLS_CERT_FILE, MTLS_KEY_FILE and MTLS_CA_FILE are all required " +
				"(alt-data-hub requires and verifies a peer certificate; there is no plaintext fallback)")
	}

	// LoadClientConfig re-reads the leaf on every handshake via
	// GetClientCertificate, so pki-agent-rag-orchestrator can rotate
	// /certs/svc-cert.pem underneath a long-lived process without a restart.
	tlsCfg, err := tlsutil.LoadClientConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("data-hub mTLS client (fail-closed): %w", err)
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
