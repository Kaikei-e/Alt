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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeSelfSignedCert generates a self-signed certificate and key to temp files
// for inert TLS testing. No network or external CA required.
func writeSelfSignedCert(t *testing.T, cn string) (certPath, keyPath string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{cn, "localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	dir := t.TempDir()
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")

	certFile, err := os.Create(certPath)
	require.NoError(t, err)
	require.NoError(t, pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}))
	require.NoError(t, certFile.Close())

	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyFile, err := os.Create(keyPath)
	require.NoError(t, err)
	require.NoError(t, pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	require.NoError(t, keyFile.Close())

	return certPath, keyPath
}

// TestLoadServerOnlyConfig_NoClientCert verifies that the server-only TLS
// config does not require client certificates (ClientAuth = NoClientCert).
func TestLoadServerOnlyConfig_NoClientCert(t *testing.T) {
	certPath, keyPath := writeSelfSignedCert(t, "auth-hub")

	cfg, err := LoadServerOnlyConfig(certPath, keyPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, tls.NoClientCert, cfg.ClientAuth,
		"A04: frontend listener must not require client certificates")
	assert.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion,
		"A04: TLS 1.3 minimum required")
	assert.Nil(t, cfg.ClientCAs,
		"A04: no client CA pool needed for server-only TLS")
}

// TestLoadServerOnlyConfig_CertReloadable verifies that the certificate
// callback is wired so that step-ca rotations take effect.
func TestLoadServerOnlyConfig_CertReloadable(t *testing.T) {
	certPath, keyPath := writeSelfSignedCert(t, "auth-hub")

	cfg, err := LoadServerOnlyConfig(certPath, keyPath)
	require.NoError(t, err)
	require.NotNil(t, cfg.GetCertificate, "cert callback must be wired for hot-reload")

	cert, err := cfg.GetCertificate(&tls.ClientHelloInfo{})
	require.NoError(t, err)
	require.NotNil(t, cert)
}

// TestLoadServerOnlyConfig_InvalidCert verifies fail-fast on bad cert material.
func TestLoadServerOnlyConfig_InvalidCert(t *testing.T) {
	_, err := LoadServerOnlyConfig("/nonexistent/cert.pem", "/nonexistent/key.pem")
	require.Error(t, err, "A04: bad cert material must fail at load time, not at first request")
}
