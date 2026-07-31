package httpclient

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTestPKI generates a throwaway CA + leaf and writes them as the three
// PEM files pki-agent would place in /certs and /trust.
func writeTestPKI(t *testing.T, dir, cn string) (certPath, keyPath, caPath string) {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	require.NoError(t, err)

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{cn, "localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caTmpl, &leafKey.PublicKey, caKey)
	require.NoError(t, err)

	certPath = filepath.Join(dir, "svc-cert.pem")
	keyPath = filepath.Join(dir, "svc-key.pem")
	caPath = filepath.Join(dir, "ca-bundle.pem")

	writeTestPEM(t, certPath, "CERTIFICATE", leafDER)
	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	require.NoError(t, err)
	writeTestPEM(t, keyPath, "EC PRIVATE KEY", leafKeyDER)
	writeTestPEM(t, caPath, "CERTIFICATE", caDER)

	return certPath, keyPath, caPath
}

func writeTestPEM(t *testing.T, path, blockType string, der []byte) {
	t.Helper()
	f, err := os.Create(path) // #nosec G304 -- test-controlled path under t.TempDir()
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })
	require.NoError(t, pem.Encode(f, &pem.Block{Type: blockType, Bytes: der}))
}

// The whole point of this constructor is that it cannot produce a working
// client from a partial configuration. alt-data-hub verifies a peer cert on
// every request, so a client built without one would fail at handshake time
// on the first tag-cloud query rather than at startup.
func TestNewDataHubClient_IncompleteCertMaterialFailsClosed(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKI(t, dir, "rag-orchestrator")

	tests := []struct {
		name string
		cfg  DataHubTransportConfig
	}{
		{name: "all empty", cfg: DataHubTransportConfig{}},
		{name: "cert missing", cfg: DataHubTransportConfig{KeyFile: keyPath, CAFile: caPath}},
		{name: "key missing", cfg: DataHubTransportConfig{CertFile: certPath, CAFile: caPath}},
		{name: "ca missing", cfg: DataHubTransportConfig{CertFile: certPath, KeyFile: keyPath}},
		{
			name: "cert path does not exist",
			cfg: DataHubTransportConfig{
				CertFile: filepath.Join(dir, "nope.pem"),
				KeyFile:  keyPath,
				CAFile:   caPath,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewDataHubClient(tt.cfg, 5*time.Second)

			require.Error(t, err, "an unusable cert configuration must be a startup error, never a plaintext client")
			assert.Nil(t, client)
		})
	}
}

func TestNewDataHubClient_PresentsLeafAndPinsServerName(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKI(t, dir, "rag-orchestrator")

	client, err := NewDataHubClient(DataHubTransportConfig{
		CertFile:   certPath,
		KeyFile:    keyPath,
		CAFile:     caPath,
		ServerName: "alt-data-hub",
	}, 7*time.Second)
	require.NoError(t, err)
	require.NotNil(t, client)
	assert.Equal(t, 7*time.Second, client.Timeout)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok, "transport must be a configured *http.Transport, not the nil default")
	require.NotNil(t, transport.TLSClientConfig)

	tlsCfg := transport.TLSClientConfig
	assert.Equal(t, "alt-data-hub", tlsCfg.ServerName)
	assert.Equal(t, uint16(tls.VersionTLS13), tlsCfg.MinVersion)
	assert.False(t, tlsCfg.InsecureSkipVerify, "the CA bundle is the whole trust decision")
	require.NotNil(t, tlsCfg.RootCAs)
	require.NotNil(t, tlsCfg.GetClientCertificate,
		"the leaf must be re-read per handshake so pki-agent rotation needs no restart")

	cert, err := tlsCfg.GetClientCertificate(&tls.CertificateRequestInfo{})
	require.NoError(t, err)
	require.NotNil(t, cert)
	require.Len(t, cert.Certificate, 1)
}

// An empty ServerName is the compose default: crypto/tls then derives it from
// the dial host, which matches alt-data-hub's SAN. It must stay empty rather
// than be filled in with a guess.
func TestNewDataHubClient_EmptyServerNameLeftToTLS(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKI(t, dir, "rag-orchestrator")

	client, err := NewDataHubClient(DataHubTransportConfig{
		CertFile: certPath,
		KeyFile:  keyPath,
		CAFile:   caPath,
	}, time.Second)
	require.NoError(t, err)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	assert.Empty(t, transport.TLSClientConfig.ServerName)
}

// MTLS_ENFORCE gates the *other* mTLS clients in this package. It must not
// gate this one: the plaintext endpoint the data-hub client replaced no
// longer exists, so "enforce off" would mean "call a dead port".
func TestNewDataHubClient_IgnoresMTLSEnforce(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKI(t, dir, "rag-orchestrator")
	t.Setenv("MTLS_ENFORCE", "")

	client, err := NewDataHubClient(DataHubTransportConfig{
		CertFile: certPath,
		KeyFile:  keyPath,
		CAFile:   caPath,
	}, time.Second)
	require.NoError(t, err)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok, "MTLS_ENFORCE=off must not downgrade the data-hub client to plaintext")
	require.NotNil(t, transport.TLSClientConfig)
}
