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
	"net/http/httptest"
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
// client from a partial configuration. alt-data-hub and search-indexer's
// :9443 verify a peer cert on every request, so a client built without one
// would fail at handshake time on the first query rather than at startup.
func TestNewPeerMTLSClient_IncompleteCertMaterialFailsClosed(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKI(t, dir, "rag-orchestrator")

	tests := []struct {
		name string
		cfg  PeerMTLSConfig
	}{
		{name: "all empty", cfg: PeerMTLSConfig{}},
		{name: "cert missing", cfg: PeerMTLSConfig{KeyFile: keyPath, CAFile: caPath}},
		{name: "key missing", cfg: PeerMTLSConfig{CertFile: certPath, CAFile: caPath}},
		{name: "ca missing", cfg: PeerMTLSConfig{CertFile: certPath, KeyFile: keyPath}},
		{
			name: "cert path does not exist",
			cfg: PeerMTLSConfig{
				CertFile: filepath.Join(dir, "nope.pem"),
				KeyFile:  keyPath,
				CAFile:   caPath,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewPeerMTLSClient(tt.cfg, 5*time.Second)

			require.Error(t, err, "an unusable cert configuration must be a startup error, never a plaintext client")
			assert.Nil(t, client)
		})
	}
}

func TestNewPeerMTLSClient_PresentsLeafAndPinsServerName(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKI(t, dir, "rag-orchestrator")

	client, err := NewPeerMTLSClient(PeerMTLSConfig{
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
// the dial host, which matches the peer's SAN. It must stay empty rather than
// be filled in with a guess.
func TestNewPeerMTLSClient_EmptyServerNameLeftToTLS(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKI(t, dir, "rag-orchestrator")

	client, err := NewPeerMTLSClient(PeerMTLSConfig{
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
// gate this one: the peers it dials have no plaintext business surface left,
// so "enforce off" would mean "call a dead port".
func TestNewPeerMTLSClient_IgnoresMTLSEnforce(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKI(t, dir, "rag-orchestrator")
	t.Setenv("MTLS_ENFORCE", "")

	client, err := NewPeerMTLSClient(PeerMTLSConfig{
		CertFile: certPath,
		KeyFile:  keyPath,
		CAFile:   caPath,
	}, time.Second)
	require.NoError(t, err)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok, "MTLS_ENFORCE=off must not downgrade the peer client to plaintext")
	require.NotNil(t, transport.TLSClientConfig)
}

// A listener that demands and verifies a client certificate — the shape of
// search-indexer's :9443 and alt-data-hub — must see the rag-orchestrator
// leaf, with MTLS_ENFORCE left unset.
func TestNewPeerMTLSClient_HandshakesWithMTLSOnlyListener(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKI(t, dir, "rag-orchestrator")
	t.Setenv("MTLS_ENFORCE", "")

	serverCert, err := tls.LoadX509KeyPair(certPath, keyPath)
	require.NoError(t, err)
	caPEM, err := os.ReadFile(caPath) // #nosec G304 -- test-controlled path under t.TempDir()
	require.NoError(t, err)
	clientCAs := x509.NewCertPool()
	require.True(t, clientCAs.AppendCertsFromPEM(caPEM))

	peerCN := make(chan string, 1)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peerCN <- r.TLS.PeerCertificates[0].Subject.CommonName
		w.WriteHeader(http.StatusOK)
	}))
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientCAs,
		MinVersion:   tls.VersionTLS13,
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	client, err := NewPeerMTLSClient(PeerMTLSConfig{
		CertFile: certPath,
		KeyFile:  keyPath,
		CAFile:   caPath,
	}, 5*time.Second)
	require.NoError(t, err)

	resp, err := client.Get(srv.URL + "/v1/search")
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "rag-orchestrator", <-peerCN)
}
