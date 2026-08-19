package pki

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newSelfSignedPEM(t *testing.T, cn string, nb, na time.Time) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    nb,
		NotAfter:     na,
		DNSNames:     []string{cn},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return
}

func TestCertFile_WriteAndLoad(t *testing.T) {
	dir := t.TempDir()
	cf := &CertFile{CertPath: filepath.Join(dir, "svc-cert.pem"), KeyPath: filepath.Join(dir, "svc-key.pem")}
	nb := time.Now().Add(-time.Minute)
	cert, key := newSelfSignedPEM(t, "auth-hub", nb, nb.Add(time.Hour))
	if err := cf.Write(context.Background(), cert, key); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(cf.CertPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o444 {
		t.Fatalf("cert perm = %o", info.Mode().Perm())
	}
	info, err = os.Stat(cf.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o400 {
		t.Fatalf("key perm = %o", info.Mode().Perm())
	}
	got, err := cf.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Subject.CommonName != "auth-hub" {
		t.Fatalf("cn=%s", got.Subject.CommonName)
	}
}

func TestCertFile_Load_Missing(t *testing.T) {
	dir := t.TempDir()
	cf := &CertFile{CertPath: filepath.Join(dir, "absent.pem"), KeyPath: filepath.Join(dir, "absent.key")}
	_, err := cf.Load(context.Background())
	if !errors.Is(err, ErrCertNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestCertFile_Write_LeavesNoTempOnSuccess(t *testing.T) {
	dir := t.TempDir()
	cf := &CertFile{CertPath: filepath.Join(dir, "svc-cert.pem"), KeyPath: filepath.Join(dir, "svc-key.pem")}
	nb := time.Now()
	cert, key := newSelfSignedPEM(t, "auth-hub", nb, nb.Add(time.Hour))
	if err := cf.Write(context.Background(), cert, key); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if len(e.Name()) >= 12 && e.Name()[:12] == ".pki-enroll-" {
			t.Fatalf("temp left behind: %s", e.Name())
		}
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
