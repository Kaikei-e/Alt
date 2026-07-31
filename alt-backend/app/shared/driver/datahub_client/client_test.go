package datahub_client

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"alt/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTestCerts mints a throwaway self-signed leaf and uses it as both the
// client identity and the trust root. The tests here are about wiring —
// "is the transport built, is ServerName pinned, does a missing path fail" —
// not about the chain, which tlsutil owns and tests.
func writeTestCerts(t *testing.T) (certPath, keyPath, caPath string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "alt-backend"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"alt-backend"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)

	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	dir := t.TempDir()
	certPath = filepath.Join(dir, "svc-cert.pem")
	keyPath = filepath.Join(dir, "svc-key.pem")
	caPath = filepath.Join(dir, "ca-bundle.pem")

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	require.NoError(t, os.WriteFile(certPath, certPEM, 0o600))
	require.NoError(t, os.WriteFile(caPath, certPEM, 0o600))
	require.NoError(t, os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600))

	return certPath, keyPath, caPath
}

func testConfig(t *testing.T) *config.DataHubClientConfig {
	t.Helper()
	certPath, keyPath, caPath := writeTestCerts(t)
	return &config.DataHubClientConfig{
		BaseURL:    "https://alt-data-hub:9443",
		ServerName: "alt-data-hub",
		CertFile:   certPath,
		KeyFile:    keyPath,
		CAFile:     caPath,
	}
}

// TestNewHTTPClientPinsServerName is the reason this package does not reuse a
// shared transport: ServerName is per-peer, and alt-data-hub's leaf carries
// alt-data-hub as its SAN.
func TestNewHTTPClientPinsServerName(t *testing.T) {
	cfg := testConfig(t)

	client, err := NewHTTPClient(cfg, 0)
	require.NoError(t, err)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok, "expected a dedicated *http.Transport")
	require.NotNil(t, transport.TLSClientConfig)
	assert.Equal(t, "alt-data-hub", transport.TLSClientConfig.ServerName)
	assert.NotNil(t, transport.TLSClientConfig.GetClientCertificate,
		"the leaf must be re-read per handshake so pki-agent can rotate it under a running process")
	assert.NotNil(t, transport.TLSClientConfig.RootCAs)
}

func TestNewHTTPClientAppliesDefaultTimeout(t *testing.T) {
	cfg := testConfig(t)

	client, err := NewHTTPClient(cfg, 0)
	require.NoError(t, err)
	assert.Equal(t, defaultTimeout, client.Timeout)

	client, err = NewHTTPClient(cfg, 5*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, client.Timeout)
}

// TestNewHTTPClientFailsClosed: every one of these produces a client that
// cannot complete a single handshake, so the only useful behaviour is to say
// so at construction. CLAUDE.md rule 9.
func TestNewHTTPClientFailsClosed(t *testing.T) {
	certPath, keyPath, caPath := writeTestCerts(t)

	tests := []struct {
		name string
		cfg  *config.DataHubClientConfig
	}{
		{name: "nil config", cfg: nil},
		{
			name: "no certificate",
			cfg:  &config.DataHubClientConfig{BaseURL: "https://alt-data-hub:9443", KeyFile: keyPath, CAFile: caPath},
		},
		{
			name: "no key",
			cfg:  &config.DataHubClientConfig{BaseURL: "https://alt-data-hub:9443", CertFile: certPath, CAFile: caPath},
		},
		{
			name: "no ca bundle",
			cfg:  &config.DataHubClientConfig{BaseURL: "https://alt-data-hub:9443", CertFile: certPath, KeyFile: keyPath},
		},
		{
			name: "certificate path does not exist",
			cfg: &config.DataHubClientConfig{
				BaseURL:  "https://alt-data-hub:9443",
				CertFile: filepath.Join(t.TempDir(), "missing.pem"),
				KeyFile:  keyPath,
				CAFile:   caPath,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewHTTPClient(tt.cfg, 0)
			require.Error(t, err)
			assert.Nil(t, client)
		})
	}
}

// TestNewReturnsUsableClient covers the composition-root entry point end to
// end: valid config in, a client with the DataHubService surface out.
func TestNewReturnsUsableClient(t *testing.T) {
	client, err := New(testConfig(t))
	require.NoError(t, err)
	require.NotNil(t, client)
}

func TestNewPropagatesConfigFailure(t *testing.T) {
	client, err := New(&config.DataHubClientConfig{BaseURL: "https://alt-data-hub:9443"})
	require.Error(t, err)
	assert.Nil(t, client)
}
