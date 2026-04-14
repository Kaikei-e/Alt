package tlsutil

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

// writeTestPKI generates a throwaway self-signed CA, a leaf cert/key for the
// given Subject CN, and writes them (cert, key, ca-bundle) as PEM files into
// dir. Returns the paths.
func writeTestPKI(t *testing.T, dir, cn string) (certPath, keyPath, caPath string) {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
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
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:     []string{cn, "localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caTmpl, &leafKey.PublicKey, caKey)
	require.NoError(t, err)

	certPath = filepath.Join(dir, "svc-cert.pem")
	keyPath = filepath.Join(dir, "svc-key.pem")
	caPath = filepath.Join(dir, "ca-bundle.pem")

	writePEM(t, certPath, "CERTIFICATE", leafDER)
	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	require.NoError(t, err)
	writePEM(t, keyPath, "EC PRIVATE KEY", leafKeyDER)
	writePEM(t, caPath, "CERTIFICATE", caDER)

	return certPath, keyPath, caPath
}

func writePEM(t *testing.T, path, blockType string, der []byte) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()
	require.NoError(t, pem.Encode(f, &pem.Block{Type: blockType, Bytes: der}))
}

func TestLoadServerConfig_ReturnsUsableConfig(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKI(t, dir, "alt-backend")

	cfg, err := LoadServerConfig(certPath, keyPath, caPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion, "min TLS version must be 1.3")
	require.NotNil(t, cfg.GetCertificate, "must use GetCertificate for hot reload")

	hello := &tls.ClientHelloInfo{ServerName: "alt-backend"}
	cert, err := cfg.GetCertificate(hello)
	require.NoError(t, err)
	require.NotNil(t, cert)
	require.Len(t, cert.Certificate, 1)
}

func TestLoadServerConfig_ReloadsOnFileChange(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKI(t, dir, "alt-backend")

	cfg, err := LoadServerConfig(certPath, keyPath, caPath)
	require.NoError(t, err)

	cert1, err := cfg.GetCertificate(&tls.ClientHelloInfo{ServerName: "alt-backend"})
	require.NoError(t, err)

	// Rotate the cert file on disk (simulate step-ca renewal) with advanced
	// mtime so the cached reader picks up the new file.
	time.Sleep(10 * time.Millisecond)
	_, _, _ = writeTestPKI(t, dir, "alt-backend")
	future := time.Now().Add(2 * time.Second)
	require.NoError(t, os.Chtimes(certPath, future, future))
	require.NoError(t, os.Chtimes(keyPath, future, future))

	cert2, err := cfg.GetCertificate(&tls.ClientHelloInfo{ServerName: "alt-backend"})
	require.NoError(t, err)
	assert.NotEqual(t, cert1.Certificate[0], cert2.Certificate[0], "cert must be reloaded when file mtime advances")
}

func TestLoadServerConfig_MissingFiles(t *testing.T) {
	_, err := LoadServerConfig("/nope/cert.pem", "/nope/key.pem", "/nope/ca.pem")
	require.Error(t, err)
}

// When the cert file on disk becomes temporarily invalid (e.g. mid-renewal
// the renewer wrote a truncated file), subsequent handshakes must keep
// working by falling back to the last good cached certificate.
func TestLoadServerConfig_ReloadFailure_FallsBackToCached(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKI(t, dir, "alt-backend")

	cfg, err := LoadServerConfig(certPath, keyPath, caPath)
	require.NoError(t, err)

	good, err := cfg.GetCertificate(&tls.ClientHelloInfo{ServerName: "alt-backend"})
	require.NoError(t, err)

	// Corrupt the cert file after advancing mtime: a parse error at reload
	// time should NOT kill the listener; the cached cert must be returned.
	time.Sleep(10 * time.Millisecond)
	require.NoError(t, os.WriteFile(certPath, []byte("not a pem"), 0o644))
	future := time.Now().Add(2 * time.Second)
	require.NoError(t, os.Chtimes(certPath, future, future))

	got, err := cfg.GetCertificate(&tls.ClientHelloInfo{ServerName: "alt-backend"})
	require.NoError(t, err, "reload failure must fall back to cached cert, not surface an error")
	assert.Equal(t, good.Certificate[0], got.Certificate[0], "must keep serving the last good cert on reload error")
}

func TestLoadClientConfig_ReturnsUsableConfig(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKI(t, dir, "pre-processor")

	cfg, err := LoadClientConfig(certPath, keyPath, caPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion)
	require.NotNil(t, cfg.GetClientCertificate, "must use GetClientCertificate for hot reload")
	require.NotNil(t, cfg.RootCAs, "root CAs must be populated")

	cert, err := cfg.GetClientCertificate(&tls.CertificateRequestInfo{})
	require.NoError(t, err)
	require.NotNil(t, cert)
	require.Len(t, cert.Certificate, 1)
}

func TestNewMTLSHTTPServer_SetsBoundedTimeouts(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKI(t, dir, "alt-backend")

	tlsCfg, err := LoadServerConfig(certPath, keyPath, caPath)
	require.NoError(t, err)

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	srv := NewMTLSHTTPServer(":0", tlsCfg, h)

	require.NotNil(t, srv)
	assert.LessOrEqual(t, srv.IdleTimeout, 60*time.Second, "idle timeout must be bounded (<=60s) so connection reuse cannot outlive a leaf cert")
	assert.Greater(t, srv.IdleTimeout, time.Duration(0), "idle timeout must be set")
	assert.Equal(t, tlsCfg, srv.TLSConfig)
}

// makePeerCert returns a parsed *x509.Certificate with the given CN and SANs,
// signed by an ephemeral throwaway CA. Used to drive cfg.VerifyConnection.
func makePeerCert(t *testing.T, cn string, sans ...string) *x509.Certificate {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "peer-ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	_, err = x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		DNSNames:     sans,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caTmpl, &leafKey.PublicKey, caKey)
	require.NoError(t, err)
	parsed, err := x509.ParseCertificate(leafDER)
	require.NoError(t, err)
	return parsed
}

func TestLoadServerConfig_DefaultsToNoClientCert(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKI(t, dir, "alt-backend")

	cfg, err := LoadServerConfig(certPath, keyPath, caPath)
	require.NoError(t, err)
	assert.Equal(t, tls.NoClientCert, cfg.ClientAuth)
	assert.Nil(t, cfg.VerifyConnection, "allowlist must not be attached when no WithAllowedPeers option given")
}

func TestLoadServerConfig_WithClientAuth_SetsField(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKI(t, dir, "alt-backend")

	cfg, err := LoadServerConfig(certPath, keyPath, caPath, WithClientAuth(tls.RequireAndVerifyClientCert))
	require.NoError(t, err)
	assert.Equal(t, tls.RequireAndVerifyClientCert, cfg.ClientAuth)
}

func TestLoadServerConfig_WithAllowedPeers_AcceptsListed(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKI(t, dir, "alt-backend")

	cfg, err := LoadServerConfig(certPath, keyPath, caPath,
		WithClientAuth(tls.RequireAndVerifyClientCert),
		WithAllowedPeers("pre-processor", "search-indexer"),
	)
	require.NoError(t, err)
	require.NotNil(t, cfg.VerifyConnection)

	peer := makePeerCert(t, "pre-processor", "pre-processor", "pre-processor.alt-network")
	err = cfg.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{peer}})
	assert.NoError(t, err)
}

func TestLoadServerConfig_WithAllowedPeers_RejectsUnlisted(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKI(t, dir, "alt-backend")

	cfg, err := LoadServerConfig(certPath, keyPath, caPath,
		WithClientAuth(tls.RequireAndVerifyClientCert),
		WithAllowedPeers("pre-processor"),
	)
	require.NoError(t, err)
	require.NotNil(t, cfg.VerifyConnection)

	peer := makePeerCert(t, "tag-generator", "tag-generator")
	err = cfg.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{peer}})
	require.Error(t, err, "peer with CN not on allowlist must be rejected by VerifyConnection")
}

func TestLoadServerConfig_WithAllowedPeers_RejectsMissingPeer(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKI(t, dir, "alt-backend")

	cfg, err := LoadServerConfig(certPath, keyPath, caPath,
		WithClientAuth(tls.RequireAndVerifyClientCert),
		WithAllowedPeers("pre-processor"),
	)
	require.NoError(t, err)

	err = cfg.VerifyConnection(tls.ConnectionState{})
	require.Error(t, err, "empty peer certs must be rejected when an allowlist is configured")
}

func TestOptionsFromEnv(t *testing.T) {
	t.Run("empty env → no options", func(t *testing.T) {
		t.Setenv("MTLS_CLIENT_AUTH", "")
		t.Setenv("MTLS_ALLOWED_PEERS", "")
		opts := OptionsFromEnv()
		assert.Len(t, opts, 0)
	})

	t.Run("require_and_verify + allowlist", func(t *testing.T) {
		t.Setenv("MTLS_CLIENT_AUTH", "require_and_verify")
		t.Setenv("MTLS_ALLOWED_PEERS", "pre-processor, search-indexer ,alt-backend")
		opts := OptionsFromEnv()

		dir := t.TempDir()
		cert, key, ca := writeTestPKI(t, dir, "svc")
		cfg, err := LoadServerConfig(cert, key, ca, opts...)
		require.NoError(t, err)
		assert.Equal(t, tls.RequireAndVerifyClientCert, cfg.ClientAuth)
		require.NotNil(t, cfg.VerifyConnection)

		peer := makePeerCert(t, "pre-processor", "pre-processor")
		assert.NoError(t, cfg.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{peer}}))
		rogue := makePeerCert(t, "news-creator", "news-creator")
		assert.Error(t, cfg.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{rogue}}))
	})
}

func TestEndToEnd_ServerAccepts_WithNoClientCertRequired(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKI(t, dir, "127.0.0.1")

	tlsCfg, err := LoadServerConfig(certPath, keyPath, caPath)
	require.NoError(t, err)

	srv := NewMTLSHTTPServer("127.0.0.1:0", tlsCfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	tlsLn := tls.NewListener(ln, tlsCfg)
	go func() { _ = srv.Serve(tlsLn) }()
	t.Cleanup(func() { _ = srv.Close() })

	caPEM, err := os.ReadFile(caPath)
	require.NoError(t, err)
	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(caPEM))

	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS13}},
	}
	resp, err := client.Get("https://" + ln.Addr().String() + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
