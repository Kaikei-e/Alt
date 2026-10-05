package authhub

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"knowledge-sovereign/internal/pki"
)

func newLeafPEM(t *testing.T, cn string) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: cn},
		DNSNames:     []string{cn},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
}

// bumpMtime moves both files forward so the reloader's mtime comparison does
// not depend on the filesystem's timestamp granularity.
func bumpMtime(t *testing.T, step int, paths ...string) {
	t.Helper()
	ts := time.Now().Add(time.Duration(step) * time.Second)
	for _, p := range paths {
		require.NoError(t, os.Chtimes(p, ts, ts))
	}
}

func leafDER(t *testing.T, certPEM []byte) []byte {
	t.Helper()
	block, _ := pem.Decode(certPEM)
	require.NotNil(t, block)
	return block.Bytes
}

func TestCertReloader_PresentsLeafRotatedByInProcessEnrollment(t *testing.T) {
	dir := t.TempDir()
	files := &pki.CertFile{
		CertPath: filepath.Join(dir, "svc-cert.pem"),
		KeyPath:  filepath.Join(dir, "svc-key.pem"),
	}
	ctx := context.Background()

	firstCert, firstKey := newLeafPEM(t, "knowledge-sovereign")
	require.NoError(t, files.Write(ctx, firstCert, firstKey))
	bumpMtime(t, 1, files.CertPath, files.KeyPath)

	r, err := newCertReloader(files.CertPath, files.KeyPath)
	require.NoError(t, err)
	got, err := r.load()
	require.NoError(t, err)
	require.True(t, bytes.Equal(got.Certificate[0], leafDER(t, firstCert)))

	secondCert, secondKey := newLeafPEM(t, "knowledge-sovereign")
	require.NoError(t, files.Write(ctx, secondCert, secondKey))
	bumpMtime(t, 2, files.CertPath, files.KeyPath)

	got, err = r.load()
	require.NoError(t, err)
	require.True(t, bytes.Equal(got.Certificate[0], leafDER(t, secondCert)),
		"next handshake must present the renewed leaf without a restart")
}

// pki.CertFile renames the key before the cert, so a crash between the two
// leaves a mismatched pair on disk until the next Tick reissues. The client
// must keep handing out the last good pair instead of failing the handshake.
func TestCertReloader_KeepsLastGoodAcrossInterruptedInstall(t *testing.T) {
	dir := t.TempDir()
	files := &pki.CertFile{
		CertPath: filepath.Join(dir, "svc-cert.pem"),
		KeyPath:  filepath.Join(dir, "svc-key.pem"),
	}
	ctx := context.Background()

	goodCert, goodKey := newLeafPEM(t, "knowledge-sovereign")
	require.NoError(t, files.Write(ctx, goodCert, goodKey))
	bumpMtime(t, 1, files.CertPath, files.KeyPath)

	r, err := newCertReloader(files.CertPath, files.KeyPath)
	require.NoError(t, err)

	_, strayKey := newLeafPEM(t, "knowledge-sovereign")
	staged := files.KeyPath + ".staged"
	require.NoError(t, os.WriteFile(staged, strayKey, 0o400))
	require.NoError(t, os.Rename(staged, files.KeyPath))
	bumpMtime(t, 2, files.CertPath, files.KeyPath)

	got, err := r.load()
	require.NoError(t, err)
	require.True(t, bytes.Equal(got.Certificate[0], leafDER(t, goodCert)))
}

func TestNewRemoteUserJWTVerifier_FailsFastWithoutEnrolledLeaf(t *testing.T) {
	dir := t.TempDir()
	_, err := NewRemoteUserJWTVerifier(Config{
		URL:      "https://auth-hub:9443/internal/token/introspect",
		CertPath: filepath.Join(dir, "svc-cert.pem"),
		KeyPath:  filepath.Join(dir, "svc-key.pem"),
		CAPath:   filepath.Join(dir, "ca-bundle.pem"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "load mTLS client cert/key")
}
