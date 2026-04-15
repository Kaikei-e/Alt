// Package httpclient provides shared HTTP client builders.
//
// mtls.go adds an env-driven helper that constructs a `*http.Client` which
// presents the rag-orchestrator leaf cert on every handshake and trusts
// only the alt-CA. Callers opt in by calling [NewMTLSClient] instead of
// building their own `&http.Client{}`.
package httpclient

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
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
// set when MTLSEnforced() is true; missing envs fail-closed.
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
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load leaf cert: %w", err)
	}
	caBytes, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read CA bundle: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caBytes) {
		return nil, fmt.Errorf("no certs parsed from CA bundle %s", caFile)
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
				RootCAs:      pool,
				MinVersion:   tls.VersionTLS13,
			},
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}, nil
}
