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
	f, err := os.Create(path) // #nosec G304 -- test-controlled path under t.TempDir()
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })
	require.NoError(t, pem.Encode(f, &pem.Block{Type: blockType, Bytes: der}))
}

func TestLoadClientConfig_ReturnsUsableConfig(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKI(t, dir, "rag-orchestrator")

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

func TestLoadClientConfig_ReloadsOnFileChange(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKI(t, dir, "rag-orchestrator")

	cfg, err := LoadClientConfig(certPath, keyPath, caPath)
	require.NoError(t, err)

	cert1, err := cfg.GetClientCertificate(&tls.CertificateRequestInfo{})
	require.NoError(t, err)

	// Rotate the cert file on disk (simulate step-ca renewal) with advanced
	// mtime so the cached reader picks up the new file.
	time.Sleep(10 * time.Millisecond)
	_, _, _ = writeTestPKI(t, dir, "rag-orchestrator")
	future := time.Now().Add(2 * time.Second)
	require.NoError(t, os.Chtimes(certPath, future, future))
	require.NoError(t, os.Chtimes(keyPath, future, future))

	cert2, err := cfg.GetClientCertificate(&tls.CertificateRequestInfo{})
	require.NoError(t, err)
	assert.NotEqual(t, cert1.Certificate[0], cert2.Certificate[0], "cert must be reloaded when file mtime advances")
}

func TestLoadClientConfig_ReloadFailure_FallsBackToCached(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKI(t, dir, "rag-orchestrator")

	cfg, err := LoadClientConfig(certPath, keyPath, caPath)
	require.NoError(t, err)

	good, err := cfg.GetClientCertificate(&tls.CertificateRequestInfo{})
	require.NoError(t, err)

	// Corrupt the cert file after advancing mtime: a parse error at reload
	// time should NOT kill the client; the cached cert must be returned.
	time.Sleep(10 * time.Millisecond)
	require.NoError(t, os.WriteFile(certPath, []byte("not a pem"), 0o600))
	future := time.Now().Add(2 * time.Second)
	require.NoError(t, os.Chtimes(certPath, future, future))

	got, err := cfg.GetClientCertificate(&tls.CertificateRequestInfo{})
	require.NoError(t, err, "reload failure must fall back to cached cert")
	assert.Equal(t, good.Certificate[0], got.Certificate[0], "must keep serving the last good cert on reload error")
}

func TestLoadClientConfig_MissingFiles(t *testing.T) {
	_, err := LoadClientConfig("/nope/cert.pem", "/nope/key.pem", "/nope/ca.pem")
	require.Error(t, err)
}
