package infrastructure

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

	"pki-agent/internal/domain"
)

func newSelfSignedPEM(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(1 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, _ := x509.MarshalECPrivateKey(key)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return
}

func TestCertFile_WriteAndLoad(t *testing.T) {
	dir := t.TempDir()
	cf := &CertFile{CertPath: filepath.Join(dir, "svc-cert.pem"), KeyPath: filepath.Join(dir, "svc-key.pem")}
	cert, key := newSelfSignedPEM(t)
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
	if got.Subject.CommonName != "test" {
		t.Fatalf("cn=%s", got.Subject.CommonName)
	}
}

func TestCertFile_Load_Missing(t *testing.T) {
	dir := t.TempDir()
	cf := &CertFile{CertPath: filepath.Join(dir, "absent.pem"), KeyPath: filepath.Join(dir, "absent.key")}
	_, err := cf.Load(context.Background())
	if !errors.Is(err, domain.ErrCertNotFound) {
		t.Fatalf("want ErrCertNotFound, got %v", err)
	}
}

func TestCertFile_Load_Corrupt(t *testing.T) {
	dir := t.TempDir()
	cert := filepath.Join(dir, "svc-cert.pem")
	if err := os.WriteFile(cert, []byte("not a pem"), 0o444); err != nil {
		t.Fatal(err)
	}
	cf := &CertFile{CertPath: cert, KeyPath: filepath.Join(dir, "k")}
	_, err := cf.Load(context.Background())
	if !errors.Is(err, domain.ErrCertParseFailed) {
		t.Fatalf("want ErrCertParseFailed, got %v", err)
	}
}

func TestCertFile_MarkRotated(t *testing.T) {
	dir := t.TempDir()
	cf := &CertFile{CertPath: filepath.Join(dir, "svc-cert.pem"), KeyPath: filepath.Join(dir, "svc-key.pem")}
	if err := cf.MarkRotated(context.Background(), time.Unix(1700000000, 0)); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "rotated.marker"))
	if err != nil {
		t.Fatal(err)
	}
	if len(content) == 0 {
		t.Fatal("marker empty")
	}
}

func TestCertFile_WriteAtomic_NoPartialOnError(t *testing.T) {
	// If cert writes then key write fails (target path is a directory),
	// we expect cert to still be present but key absent. Concurrency-wise
	// the point is that no half-written file is ever visible at either path.
	dir := t.TempDir()
	keyDir := filepath.Join(dir, "key-is-a-dir")
	if err := os.Mkdir(keyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cf := &CertFile{CertPath: filepath.Join(dir, "svc-cert.pem"), KeyPath: keyDir}
	cert, key := newSelfSignedPEM(t)
	if err := cf.Write(context.Background(), cert, key); err == nil {
		t.Fatal("expected write-to-dir to fail")
	}
	// The cert write succeeded atomically, and whatever tmpfile we created
	// for the key should be cleaned up — no .pkiagent-* survivors.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" || len(e.Name()) > 10 && e.Name()[:10] == ".pkiagent-" {
			t.Fatalf("leaked tmpfile: %s", e.Name())
		}
	}
}
