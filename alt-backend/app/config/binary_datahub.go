package config

import (
	"fmt"
	"strings"
)

// DataHubConfig is the mTLS listener configuration for cmd/datahub. Every
// field is required: data-hub is the only writer of alt-db on behalf of other
// services, and it has no plaintext surface to fall back to.
type DataHubConfig struct {
	ListenAddr   string
	CertFile     string
	KeyFile      string
	CAFile       string
	AllowedPeers []string
}

// LoadDataHubConfig reads the data-hub listener configuration, failing on any
// missing value.
//
// This deliberately does not use tlsutil.OptionsFromEnv, which fails open
// twice: MTLS_CLIENT_AUTH unset yields tls.NoClientCert (TLS that never
// verifies the caller), and MTLS_ALLOWED_PEERS unset yields no allowlist (any
// certificate the shared CA issued is accepted, i.e. any service may
// impersonate any other). Client auth is not configurable here at all —
// cmd/datahub always requires and verifies — and the allowlist is required
// config.
func LoadDataHubConfig() (*DataHubConfig, error) {
	listenAddr, err := LoadDataHubListenAddr()
	if err != nil {
		return nil, err
	}
	certFile, err := requiredEnv("DATAHUB_TLS_CERT_FILE")
	if err != nil {
		return nil, err
	}
	keyFile, err := requiredEnv("DATAHUB_TLS_KEY_FILE")
	if err != nil {
		return nil, err
	}
	caFile, err := requiredEnv("DATAHUB_TLS_CA_FILE")
	if err != nil {
		return nil, err
	}
	rawPeers, err := requiredEnv("DATAHUB_ALLOWED_PEERS")
	if err != nil {
		return nil, err
	}

	peers := make([]string, 0, strings.Count(rawPeers, ",")+1)
	for _, p := range strings.Split(rawPeers, ",") {
		if s := strings.TrimSpace(p); s != "" {
			peers = append(peers, s)
		}
	}
	if len(peers) == 0 {
		return nil, fmt.Errorf("DATAHUB_ALLOWED_PEERS contains no usable entries: " +
			"an empty allowlist accepts every certificate the shared CA issued")
	}

	return &DataHubConfig{
		ListenAddr:   listenAddr,
		CertFile:     certFile,
		KeyFile:      keyFile,
		CAFile:       caFile,
		AllowedPeers: peers,
	}, nil
}

// LoadDataHubListenAddr reads just the mutual-TLS listener's bind address.
//
// It is split out of LoadDataHubConfig because the healthcheck subcommand
// needs it and nothing else: the probe dials this port to prove the mTLS
// listener goroutine is alive, and a probe that had to load certificates and
// the peer allowlist first would fail for reasons unrelated to liveness
// (ADR-000784).
func LoadDataHubListenAddr() (string, error) {
	addr, err := requiredEnv("DATAHUB_LISTEN_ADDR")
	if err != nil {
		return "", err
	}
	if err := validateHostPort("DATAHUB_LISTEN_ADDR", addr); err != nil {
		return "", err
	}
	return addr, nil
}

// ValidateDataHubConfig checks the upstreams DataHubService calls.
//
// INTERNAL_AUTH_SECRET is on this list because di/datahub hands it to
// kratos_client, which puts it verbatim into the X-Internal-Auth header of
// every auth-hub /internal/system-user call. Unset is not a disabled feature:
// alt-data-hub would report healthy while auth-hub answered 401 to every
// GetSystemUser call. NewConfig only catches INTERNAL_AUTH_SECRET_FILE that
// resolves to empty, so the plain-unset case has to fail here (CLAUDE.md
// rule 9).
func ValidateDataHubConfig(cfg *Config) error {
	required := []struct {
		env   string
		value string
	}{
		{"AUTH_HUB_URL", cfg.AuthHub.URL},
		{"BACKEND_TOKEN_SECRET", cfg.Auth.BackendTokenSecret},
		{"INTERNAL_AUTH_SECRET", cfg.Auth.InternalAuthSecret},
		{"SOVEREIGN_URL", cfg.Sovereign.URL},
		{"MQHUB_CONNECT_URL", cfg.MQHub.ConnectURL},
	}
	if err := requireAll("datahub", required); err != nil {
		return err
	}

	// mqhub_connect.Client no-ops every publish when disabled, so the article
	// RPCs would answer 200 while emitting no events at all — the exact shape
	// of ADR-000928. Development may opt out explicitly; nothing else may.
	if !cfg.MQHub.Enabled && cfg.AppEnv != "development" {
		return fmt.Errorf("datahub config: MQHUB_ENABLED=false in APP_ENV=%s would make every "+
			"DataHubService article RPC succeed while publishing no events", cfg.AppEnv)
	}
	return nil
}
