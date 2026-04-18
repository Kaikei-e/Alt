// Package httpclient provides shared HTTP client builders.
//
// mtls.go adds an env-driven helper that constructs a `*http.Client` which
// presents the rag-orchestrator leaf cert on every handshake and trusts
// only the alt-CA. Callers opt in by calling [NewMTLSClient] instead of
// building their own `&http.Client{}`.
package httpclient

import (
	"errors"
	"net/http"
	"os"
	"time"

	"rag-orchestrator/internal/infra/tlsutil"
)

// MTLSEnforced reports whether MTLS_ENFORCE=true is set.
func MTLSEnforced() bool {
	return os.Getenv("MTLS_ENFORCE") == "true"
}

// NewMTLSClient returns an `*http.Client` with the rag-orchestrator leaf cert
// loaded and the alt-CA trust store configured. When MTLS_ENFORCE!=true, the
// returned client has no TLS config (matches existing plaintext behaviour).
//
// Env vars: MTLS_CERT_FILE, MTLS_KEY_FILE, MTLS_CA_FILE. All three must be
// set when MTLSEnforced() is true; missing envs fail-closed. The leaf cert
// is re-read from disk on every handshake via GetClientCertificate, so the
// pki-agent sidecar can rotate it without a process restart.
func NewMTLSClient(timeout time.Duration) (*http.Client, error) {
	if !MTLSEnforced() {
		return &http.Client{Timeout: timeout}, nil
	}
	certFile := os.Getenv("MTLS_CERT_FILE")
	keyFile := os.Getenv("MTLS_KEY_FILE")
	caFile := os.Getenv("MTLS_CA_FILE")
	if certFile == "" || keyFile == "" || caFile == "" {
		return nil, errors.New("MTLS_ENFORCE=true but MTLS_CERT_FILE/KEY_FILE/CA_FILE not fully set")
	}
	tlsCfg, err := tlsutil.LoadClientConfig(certFile, keyFile, caFile)
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig:     tlsCfg,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}, nil
}
