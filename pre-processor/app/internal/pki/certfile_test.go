package pki

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
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
	cert, key := newSelfSignedPEM(t, "alt-backend", nb, nb.Add(time.Hour))
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
	if got.Subject.CommonName != "alt-backend" {
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
	cert, key := newSelfSignedPEM(t, "alt-backend", nb, nb.Add(time.Hour))
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
		if e.Name() == ".pki-pair-journal" {
			t.Fatal("install journal left behind after successful write")
		}
	}
}

func TestCertFile_Load_MismatchedPairIsCorrupt(t *testing.T) {
	dir := t.TempDir()
	cf := &CertFile{CertPath: filepath.Join(dir, "svc-cert.pem"), KeyPath: filepath.Join(dir, "svc-key.pem")}
	nb := time.Now()
	certA, keyA := newSelfSignedPEM(t, "alt-backend", nb, nb.Add(time.Hour))
	if err := cf.Write(context.Background(), certA, keyA); err != nil {
		t.Fatal(err)
	}
	_, keyB := newSelfSignedPEM(t, "alt-backend", nb, nb.Add(time.Hour))
	if err := os.Remove(cf.KeyPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cf.KeyPath, keyB, 0o400); err != nil {
		t.Fatal(err)
	}
	_, err := cf.Load(context.Background())
	if !errors.Is(err, ErrCertParseFailed) {
		t.Fatalf("mismatched pair must be corrupt, got %v", err)
	}
}

func TestCertFile_Load_RestoresPreviousKeyFromJournal(t *testing.T) {
	dir := t.TempDir()
	cf := &CertFile{CertPath: filepath.Join(dir, "svc-cert.pem"), KeyPath: filepath.Join(dir, "svc-key.pem")}
	nb := time.Now()
	certA, keyA := newSelfSignedPEM(t, "alt-backend", nb, nb.Add(time.Hour))
	if err := cf.Write(context.Background(), certA, keyA); err != nil {
		t.Fatal(err)
	}
	_, keyB := newSelfSignedPEM(t, "alt-backend", nb, nb.Add(2*time.Hour))
	if err := os.WriteFile(cf.journalPath(), keyA, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(cf.KeyPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cf.KeyPath, keyB, 0o400); err != nil {
		t.Fatal(err)
	}
	got, err := cf.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Subject.CommonName != "alt-backend" {
		t.Fatalf("cn=%s", got.Subject.CommonName)
	}
	onDiskCert, err := os.ReadFile(cf.CertPath)
	if err != nil {
		t.Fatal(err)
	}
	onDiskKey, err := os.ReadFile(cf.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tls.X509KeyPair(onDiskCert, onDiskKey); err != nil {
		t.Fatalf("restored pair must match: %v", err)
	}
	if _, err := os.Stat(cf.journalPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("journal should be consumed after restore, stat=%v", err)
	}
}

func TestCertFile_Write_RollsBackKeyOnCertRenameError(t *testing.T) {
	dir := t.TempDir()
	cf := &CertFile{CertPath: filepath.Join(dir, "svc-cert.pem"), KeyPath: filepath.Join(dir, "svc-key.pem")}
	nb := time.Now()
	certA, keyA := newSelfSignedPEM(t, "alt-backend", nb, nb.Add(time.Hour))
	if err := cf.Write(context.Background(), certA, keyA); err != nil {
		t.Fatal(err)
	}
	orig := renameFile
	t.Cleanup(func() { renameFile = orig })
	renameFile = func(old, new string) error {
		if new == cf.CertPath {
			return errors.New("injected cert rename failure")
		}
		return orig(old, new)
	}
	certB, keyB := newSelfSignedPEM(t, "alt-backend", nb, nb.Add(2*time.Hour))
	if err := cf.Write(context.Background(), certB, keyB); err == nil {
		t.Fatal("cert rename failure must surface")
	}
	onDiskCert, err := os.ReadFile(cf.CertPath)
	if err != nil {
		t.Fatal(err)
	}
	onDiskKey, err := os.ReadFile(cf.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tls.X509KeyPair(onDiskCert, onDiskKey); err != nil {
		t.Fatalf("key must roll back to previous pair: %v", err)
	}
	if _, err := tls.X509KeyPair(onDiskCert, keyA); err != nil {
		t.Fatalf("rolled-back key must still be pair A: %v", err)
	}
}

func TestCertFile_Write_CrashBetweenRenamesRecoversOnLoad(t *testing.T) {
	dir := t.TempDir()
	cf := &CertFile{CertPath: filepath.Join(dir, "svc-cert.pem"), KeyPath: filepath.Join(dir, "svc-key.pem")}
	nb := time.Now()
	certA, keyA := newSelfSignedPEM(t, "alt-backend", nb, nb.Add(time.Hour))
	if err := cf.Write(context.Background(), certA, keyA); err != nil {
		t.Fatal(err)
	}
	orig := renameFile
	t.Cleanup(func() { renameFile = orig })
	renameFile = func(old, new string) error {
		if new == cf.CertPath {
			return errors.New("injected crash after key rename")
		}
		return orig(old, new)
	}
	certB, keyB := newSelfSignedPEM(t, "alt-backend", nb, nb.Add(2*time.Hour))
	if err := cf.Write(context.Background(), certB, keyB); err == nil {
		t.Fatal("simulated crash must fail Write")
	}
	got, err := cf.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.NotAfter.Equal(nb.Add(2 * time.Hour)) {
		t.Fatal("partial install must not leave the new unmatched cert as loaded")
	}
	onDiskCert, err := os.ReadFile(cf.CertPath)
	if err != nil {
		t.Fatal(err)
	}
	onDiskKey, err := os.ReadFile(cf.KeyPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tls.X509KeyPair(onDiskCert, onDiskKey); err != nil {
		t.Fatalf("startup load must restore a matching pair: %v", err)
	}
}
