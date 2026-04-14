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
)

// smoke test: LoadServerConfig must accept a valid PEM pair and CA bundle.
func TestLoadServerConfig_Smoke(t *testing.T) {
	dir := t.TempDir()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, _ := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "auth-hub"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"auth-hub"},
	}
	leafDER, _ := x509.CreateCertificate(rand.Reader, leafTmpl, caTmpl, &leafKey.PublicKey, caKey)
	leafKeyDER, _ := x509.MarshalECPrivateKey(leafKey)

	cert := filepath.Join(dir, "svc-cert.pem")
	key := filepath.Join(dir, "svc-key.pem")
	ca := filepath.Join(dir, "ca.pem")
	_ = os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}), 0600)
	_ = os.WriteFile(key, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: leafKeyDER}), 0600)
	_ = os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0600)

	cfg, err := LoadServerConfig(cert, key, ca)
	if err != nil {
		t.Fatalf("LoadServerConfig error: %v", err)
	}
	if cfg.MinVersion != tls.VersionTLS13 {
		t.Fatalf("expected TLS 1.3 min, got %x", cfg.MinVersion)
	}
	if _, err := cfg.GetCertificate(&tls.ClientHelloInfo{ServerName: "auth-hub"}); err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
}
