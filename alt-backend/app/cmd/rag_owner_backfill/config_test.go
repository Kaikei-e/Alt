package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTestPKI(t *testing.T, dir string) (certPath, keyPath, caPath string) {
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
		Subject:      pkix.Name{CommonName: "alt-backend"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		DNSNames:     []string{"alt-backend"},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caTmpl, &leafKey.PublicKey, caKey)
	require.NoError(t, err)

	certPath = filepath.Join(dir, "svc-cert.pem")
	keyPath = filepath.Join(dir, "svc-key.pem")
	caPath = filepath.Join(dir, "ca-bundle.pem")

	require.NoError(t, os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}), 0o600))
	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: leafKeyDER}), 0o600))
	require.NoError(t, os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0o600))

	return certPath, keyPath, caPath
}

func TestResolveRAGOrchestratorURL_DefaultsToHTTPS(t *testing.T) {
	t.Setenv("RAG_ORCHESTRATOR_URL", "")

	got := resolveRAGOrchestratorURL()
	assert.Equal(t, "https://rag-orchestrator:9010", got)
}

func TestNewRAGHTTPClient_HTTPSWithoutMTLSFiles_ReturnsError(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	t.Run("unset MTLS env vars", func(t *testing.T) {
		t.Setenv("MTLS_CERT_FILE", "")
		t.Setenv("MTLS_KEY_FILE", "")
		t.Setenv("MTLS_CA_FILE", "")

		_, err := newRAGHTTPClient("https://rag-orchestrator:9010", logger)
		require.Error(t, err)
	})

	t.Run("missing MTLS files on disk", func(t *testing.T) {
		t.Setenv("MTLS_CERT_FILE", "/nonexistent/cert.pem")
		t.Setenv("MTLS_KEY_FILE", "/nonexistent/key.pem")
		t.Setenv("MTLS_CA_FILE", "/nonexistent/ca.pem")

		_, err := newRAGHTTPClient("https://rag-orchestrator:9010", logger)
		require.Error(t, err)
	})
}

func TestNewRAGHTTPClient_HTTPSWithValidMTLSFiles_ReturnsMTLSClient(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	certPath, keyPath, caPath := writeTestPKI(t, t.TempDir())

	t.Setenv("MTLS_CERT_FILE", certPath)
	t.Setenv("MTLS_KEY_FILE", keyPath)
	t.Setenv("MTLS_CA_FILE", caPath)

	client, err := newRAGHTTPClient("https://rag-orchestrator:9010", logger)
	require.NoError(t, err)
	require.NotNil(t, client)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok, "expected *http.Transport")
	require.NotNil(t, transport.TLSClientConfig)
	assert.NotNil(t, transport.TLSClientConfig.GetClientCertificate)
	assert.NotNil(t, transport.TLSClientConfig.RootCAs)
}

func TestNewRAGHTTPClient_PlainHTTP_LogsWarningAndSucceedsWithoutMTLS(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	t.Setenv("MTLS_CERT_FILE", "")
	t.Setenv("MTLS_KEY_FILE", "")
	t.Setenv("MTLS_CA_FILE", "")

	client, err := newRAGHTTPClient("http://rag-orchestrator:9010", logger)
	require.NoError(t, err)
	require.NotNil(t, client)
	assert.Contains(t, buf.String(), "WARN")
	assert.Contains(t, buf.String(), "rag_mtls_disabled")
}

func TestLoadDBPassword_ReadsAndTrimsPasswordFile(t *testing.T) {
	secretFile := filepath.Join(t.TempDir(), "postgres_password")
	require.NoError(t, os.WriteFile(secretFile, []byte("  s3cr3t-from-file\n\t "), 0o600))

	t.Setenv("DB_PASSWORD_FILE", secretFile)
	t.Setenv("DB_PASSWORD", "fallback-ignored")

	pw, err := loadDBPassword()
	require.NoError(t, err)
	assert.Equal(t, "s3cr3t-from-file", pw)
}

func TestLoadDBPassword_NeitherPasswordVar_ReturnsError(t *testing.T) {
	t.Setenv("DB_PASSWORD_FILE", "")
	t.Setenv("DB_PASSWORD", "")

	_, err := loadDBPassword()
	require.Error(t, err)

	_, err = buildPGXPoolConfig()
	require.Error(t, err)
}

func TestBuildPGXPoolConfig_SpecialCharactersInPasswordSurvive(t *testing.T) {
	rawPassword := "p@ss:w/rd%20#?&=!secret"
	t.Setenv("DB_PASSWORD_FILE", "")
	t.Setenv("DB_PASSWORD", rawPassword)
	t.Setenv("DB_HOST", "pgbouncer")
	t.Setenv("DB_PORT", "6432")
	t.Setenv("DB_USER", "alt_db_user")
	t.Setenv("DB_NAME", "alt")

	cfg, err := buildPGXPoolConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, rawPassword, cfg.ConnConfig.Password)
	assert.Equal(t, "pgbouncer", cfg.ConnConfig.Host)
	assert.Equal(t, uint16(6432), cfg.ConnConfig.Port)
	assert.Equal(t, "alt_db_user", cfg.ConnConfig.User)
	assert.Equal(t, "alt", cfg.ConnConfig.Database)
}
