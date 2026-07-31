package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Environment variables that configure the outbound connection to
// alt-data-hub. The MTLS_* trio is shared with the other mutual-TLS hops
// cmd/backend and cmd/harvester make (rag-orchestrator's Connect listener),
// because pki-agent mints one leaf per container and every peer verifies it
// against the same CA — a second copy of the same three paths under a
// DATA_HUB_ prefix would be three more places to get out of sync with the
// sidecar's mount, for no additional expressiveness.
const (
	dataHubURLEnv        = "DATA_HUB_MTLS_URL"
	dataHubServerNameEnv = "DATA_HUB_SERVER_NAME"
	mtlsCertFileEnv      = "MTLS_CERT_FILE"
	mtlsKeyFileEnv       = "MTLS_KEY_FILE"
	mtlsCAFileEnv        = "MTLS_CA_FILE"
)

// DataHubClientConfig is what cmd/backend and cmd/harvester need to reach
// alt.datahub.v1.DataHubService.
//
// alt-data-hub is not an optional upstream for either binary: after ADR-000954
// Wave 3 it owns alt_db, so "no data-hub client" is "no database". There is
// therefore no enabled flag here and no partial configuration — see
// LoadDataHubClientConfig.
type DataHubClientConfig struct {
	// BaseURL is the Connect-RPC base, e.g. https://alt-data-hub:9443.
	BaseURL string
	// ServerName pins tls.Config.ServerName. Defaults to BaseURL's host,
	// which is what crypto/tls would derive on its own and what
	// alt-data-hub's leaf carries as its SAN. Override only when the dial
	// target is not the SAN (an IP, a tunnel, a staging alias).
	ServerName string
	CertFile   string
	KeyFile    string
	CAFile     string
}

// LoadDataHubClientConfig reads the data-hub client configuration, failing on
// any missing or non-https value (CLAUDE.md rule 9).
//
// Both halves of that check earn their place:
//
//   - Missing certificate material cannot degrade into a working plaintext
//     call, because alt-data-hub requires and verifies a peer certificate on
//     its only data-plane listener. It can only produce a TLS handshake
//     failure on the first request, which for cmd/harvester is a scheduled job
//     tick minutes after the process reported healthy.
//   - A plaintext URL cannot degrade either: cmd/datahub opens no plaintext
//     data-plane listener, so http:// is a guaranteed connection refused
//     rather than an insecure-but-working mode. Rejecting the scheme here
//     names the mistake while the operator is still reading startup logs.
func LoadDataHubClientConfig() (*DataHubClientConfig, error) {
	rawURL, err := requiredEnv(dataHubURLEnv)
	if err != nil {
		return nil, err
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%s=%q is not a valid URL: %w", dataHubURLEnv, rawURL, err)
	}
	if parsed.Scheme != "https" {
		return nil, fmt.Errorf(
			"%s=%q must use the https scheme: alt-data-hub serves its data plane over mutual TLS only, "+
				"so any other scheme is a connection refused rather than a degraded mode",
			dataHubURLEnv, rawURL)
	}
	if parsed.Hostname() == "" {
		return nil, fmt.Errorf("%s=%q has no host", dataHubURLEnv, rawURL)
	}

	certFile, err := requiredEnv(mtlsCertFileEnv)
	if err != nil {
		return nil, fmt.Errorf("data-hub client: %w", err)
	}
	keyFile, err := requiredEnv(mtlsKeyFileEnv)
	if err != nil {
		return nil, fmt.Errorf("data-hub client: %w", err)
	}
	caFile, err := requiredEnv(mtlsCAFileEnv)
	if err != nil {
		return nil, fmt.Errorf("data-hub client: %w", err)
	}

	serverName := strings.TrimSpace(os.Getenv(dataHubServerNameEnv))
	if serverName == "" {
		serverName = parsed.Hostname()
	}

	return &DataHubClientConfig{
		BaseURL:    strings.TrimRight(rawURL, "/"),
		ServerName: serverName,
		CertFile:   certFile,
		KeyFile:    keyFile,
		CAFile:     caFile,
	}, nil
}
