// Package datahub_client dials alt.datahub.v1.DataHubService, the contract
// alt-data-hub serves for alt_db (ADR-000954 D3).
//
// It is the in-family counterpart of what rag-orchestrator, pre-processor and
// search-indexer already do from outside: alt-backend and alt-harvester are
// built from the same Go module as alt-data-hub, but after the three-binary
// split they are as much its clients as any other peer, and they reach it the
// same way — mutual TLS on :9443, Connect-RPC over HTTP/1.1, protoJSON codec.
//
// cmd/datahub does NOT build this client. A process calling its own RPC
// surface would pay two TLS handshakes and a JSON round trip to reach a
// usecase it can call directly, and would have to appear in its own
// DATAHUB_ALLOWED_PEERS — which is the entire authorisation decision for the
// data plane, and means something narrower when the service is on the list.
package datahub_client

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"alt/config"
	"alt/gen/proto/alt/datahub/v1/datahubv1connect"
	"alt/tlsutil"

	"connectrpc.com/connect"
)

// defaultTimeout bounds a single unary call. Every DataHubService procedure is
// a bounded query or a single-statement write; the batch-shaped ones cap their
// own page size. A caller with a different budget passes its own context
// deadline, which takes precedence when it is shorter.
const defaultTimeout = 30 * time.Second

// Connection pool sizing. The heaviest caller is cmd/harvester's outbox
// worker, which claims a batch every five seconds from one goroutine; the
// backend's image proxy is the widest, fanning out per request. Twenty idle
// connections to a single host is generous for both and small enough that an
// alt-data-hub restart does not leave a large pool of dead sockets behind.
const (
	maxIdleConns        = 20
	maxIdleConnsPerHost = 10
	idleConnTimeout     = 120 * time.Second
)

// NewHTTPClient returns an *http.Client that presents this binary's leaf
// certificate to alt-data-hub and trusts only the alt CA.
//
// It deliberately builds a transport of its own rather than sharing a
// process-global one. tls.Config.ServerName is pinned here, and a shared
// transport would pin it for every other mutual-TLS peer the process talks to
// — rag-orchestrator's Connect listener, in cmd/backend's case. Connection
// pooling is preserved within this transport.
//
// tlsutil.LoadClientConfig re-reads the leaf on every handshake via
// GetClientCertificate, so the pki-agent sidecar can rotate
// /certs/svc-cert.pem underneath a long-lived process without a restart.
//
// HTTP/1.1 is what this negotiates: Go's http.Transport only attempts HTTP/2
// automatically when TLSClientConfig is nil or ForceAttemptHTTP2 is set, and
// every DataHubService procedure is unary, which Connect carries over
// HTTP/1.1. This matches how the out-of-family peers dial the same listener.
func NewHTTPClient(cfg *config.DataHubClientConfig, timeout time.Duration) (*http.Client, error) {
	if cfg == nil {
		return nil, errors.New("data-hub client: configuration is nil")
	}
	if cfg.CertFile == "" || cfg.KeyFile == "" || cfg.CAFile == "" {
		return nil, errors.New(
			"data-hub client: MTLS_CERT_FILE, MTLS_KEY_FILE and MTLS_CA_FILE are all required " +
				"(alt-data-hub requires and verifies a peer certificate; there is no plaintext fallback)")
	}
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	tlsCfg, err := tlsutil.LoadClientConfig(cfg.CertFile, cfg.KeyFile, cfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("data-hub client (fail-closed): %w", err)
	}
	if cfg.ServerName != "" {
		tlsCfg.ServerName = cfg.ServerName
	}

	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig:     tlsCfg,
			MaxIdleConns:        maxIdleConns,
			MaxIdleConnsPerHost: maxIdleConnsPerHost,
			IdleConnTimeout:     idleConnTimeout,
		},
	}, nil
}

// NewServiceClient builds the Connect-RPC client every data-hub-backed gateway
// in this process shares.
//
// It exists so the codec choice is made in exactly one place: the composition
// root and the Pact consumer tests both call it, which is what makes the
// recorded contract the bytes production sends rather than a readable
// approximation of them.
//
// The codec is pinned to JSON with connect.WithProtoJSON per ADR-000764.
// connect-go otherwise defaults to application/proto, and a binary body makes
// pact interactions unverifiable — the same trap the rag-orchestrator and
// sovereign clients document.
//
// There is no interceptor and no credential in the request: alt-data-hub
// authenticates callers by peer certificate and nothing else.
func NewServiceClient(httpClient *http.Client, baseURL string) datahubv1connect.DataHubServiceClient {
	return datahubv1connect.NewDataHubServiceClient(httpClient, baseURL, connect.WithProtoJSON())
}

// New is the composition-root entry point: configuration in, ready client out.
//
// It returns an error rather than a disabled client. A nil-but-usable value
// would let a job tick succeed while writing nothing, which is the failure
// class CLAUDE.md rule 8 exists for; the callers turn this error into a
// non-zero exit.
func New(cfg *config.DataHubClientConfig) (datahubv1connect.DataHubServiceClient, error) {
	httpClient, err := NewHTTPClient(cfg, defaultTimeout)
	if err != nil {
		return nil, err
	}
	return NewServiceClient(httpClient, cfg.BaseURL), nil
}
