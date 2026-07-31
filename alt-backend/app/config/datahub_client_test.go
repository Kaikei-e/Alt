package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setDataHubClientEnv puts a complete, valid configuration in the environment
// so each test can remove exactly one thing and assert on that.
func setDataHubClientEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATA_HUB_MTLS_URL", "https://alt-data-hub:9443")
	t.Setenv("DATA_HUB_SERVER_NAME", "alt-data-hub")
	t.Setenv("MTLS_CERT_FILE", "/certs/svc-cert.pem")
	t.Setenv("MTLS_KEY_FILE", "/certs/svc-key.pem")
	t.Setenv("MTLS_CA_FILE", "/trust/ca-bundle.pem")
}

func TestLoadDataHubClientConfig(t *testing.T) {
	setDataHubClientEnv(t)

	cfg, err := LoadDataHubClientConfig()
	require.NoError(t, err)
	assert.Equal(t, "https://alt-data-hub:9443", cfg.BaseURL)
	assert.Equal(t, "alt-data-hub", cfg.ServerName)
	assert.Equal(t, "/certs/svc-cert.pem", cfg.CertFile)
	assert.Equal(t, "/certs/svc-key.pem", cfg.KeyFile)
	assert.Equal(t, "/trust/ca-bundle.pem", cfg.CAFile)
}

// TestLoadDataHubClientConfigRequiresEveryValue is CLAUDE.md rule 9 as a test.
// alt-data-hub demands a peer certificate and publishes no plaintext port, so
// every one of these missing turns into a handshake or DNS error on the first
// scheduled job tick — minutes after start, in a log line that says nothing
// about configuration.
func TestLoadDataHubClientConfigRequiresEveryValue(t *testing.T) {
	tests := []struct {
		name string
		env  string
	}{
		{name: "base url", env: "DATA_HUB_MTLS_URL"},
		{name: "client certificate", env: "MTLS_CERT_FILE"},
		{name: "client key", env: "MTLS_KEY_FILE"},
		{name: "ca bundle", env: "MTLS_CA_FILE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setDataHubClientEnv(t)
			t.Setenv(tt.env, "")

			_, err := LoadDataHubClientConfig()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.env)
		})
	}
}

// TestLoadDataHubClientConfigRejectsPlaintextURL: http:// would not be a
// degraded mode, it would be a connection refused — alt-data-hub opens no
// plaintext data-plane listener at all. Rejecting the scheme at startup names
// the mistake instead of leaving it to a dial error later.
func TestLoadDataHubClientConfigRejectsPlaintextURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{name: "plaintext http", url: "http://alt-data-hub:9443"},
		{name: "no scheme", url: "alt-data-hub:9443"},
		{name: "unrelated scheme", url: "grpc://alt-data-hub:9443"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setDataHubClientEnv(t)
			t.Setenv("DATA_HUB_MTLS_URL", tt.url)

			_, err := LoadDataHubClientConfig()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "https")
		})
	}
}

// TestLoadDataHubClientConfigServerNameDefaultsToURLHost keeps the SAN pin
// correct without requiring a second variable to be kept in sync with the
// first.
func TestLoadDataHubClientConfigServerNameDefaultsToURLHost(t *testing.T) {
	setDataHubClientEnv(t)
	t.Setenv("DATA_HUB_SERVER_NAME", "")

	cfg, err := LoadDataHubClientConfig()
	require.NoError(t, err)
	assert.Equal(t, "alt-data-hub", cfg.ServerName,
		"an unset DATA_HUB_SERVER_NAME must fall back to the URL host, not to the empty string")
}

func TestLoadDataHubClientConfigRejectsUnparseableURL(t *testing.T) {
	setDataHubClientEnv(t)
	t.Setenv("DATA_HUB_MTLS_URL", "https://")

	_, err := LoadDataHubClientConfig()
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "DATA_HUB_MTLS_URL"))
}
