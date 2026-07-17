// Package httpclient provides shared HTTP client builders.
//
// mtls.go adds an env-driven helper that constructs a `*http.Client` which
// presents the rag-orchestrator leaf cert on every handshake and trusts
// only the alt-CA. Callers opt in by calling [NewMTLSClient] instead of
// building their own `&http.Client{}`.
package httpclient

import (
	"net/http"
	"os"
	"time"
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
// set when MTLSEnforced() is true; missing envs fail-closed. Cert loading is
// delegated to [loadMTLSTransport] so NewMTLSClient and NewPooledClient share
// one code path (and the same connection-pooled transport when enforced).
func NewMTLSClient(timeout time.Duration) (*http.Client, error) {
	if !MTLSEnforced() {
		return &http.Client{Timeout: timeout}, nil
	}
	t, err := loadMTLSTransport()
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: t,
	}, nil
}
