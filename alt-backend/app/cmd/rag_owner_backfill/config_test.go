package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"alt/orchestrator/gateway/rag_gateway"
	"alt/tlsutil"

	"github.com/jackc/pgx/v5"
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
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:     []string{"alt-backend", "localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
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
		assert.ErrorContains(t, err, "MTLS_CERT_FILE")
		assert.ErrorContains(t, err, "MTLS_KEY_FILE")
		assert.ErrorContains(t, err, "MTLS_CA_FILE")
	})

	t.Run("missing MTLS files on disk", func(t *testing.T) {
		t.Setenv("MTLS_CERT_FILE", "/nonexistent/cert.pem")
		t.Setenv("MTLS_KEY_FILE", "/nonexistent/key.pem")
		t.Setenv("MTLS_CA_FILE", "/nonexistent/ca.pem")

		_, err := newRAGHTTPClient("https://rag-orchestrator:9010", logger)
		require.Error(t, err)
		assert.ErrorContains(t, err, "build mTLS client")
		assert.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("unsupported scheme", func(t *testing.T) {
		_, err := newRAGHTTPClient("ftp://rag-orchestrator:9010", logger)
		require.Error(t, err)
		assert.ErrorContains(t, err, "must use https:// or http:// scheme")
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

func TestNewRAGClient_EndToEndMTLSAndBearerAuth(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKI(t, dir)

	const testToken = "rag-secret-token-at-least-24-chars"
	tokenFile := filepath.Join(dir, "rag_api_token")
	require.NoError(t, os.WriteFile(tokenFile, []byte(testToken+"\n"), 0o600))

	t.Setenv("MTLS_CERT_FILE", certPath)
	t.Setenv("MTLS_KEY_FILE", keyPath)
	t.Setenv("MTLS_CA_FILE", caPath)
	t.Setenv("RAG_API_TOKEN_FILE", tokenFile)
	t.Setenv("RAG_API_TOKEN", "")
	t.Setenv("RAG_API_AUTH", "")

	srvTLS, err := tlsutil.LoadServerConfig(
		certPath, keyPath, caPath,
		tlsutil.WithClientAuth(tls.RequireAndVerifyClientCert),
	)
	require.NoError(t, err)
	srvCert, err := tls.LoadX509KeyPair(certPath, keyPath)
	require.NoError(t, err)
	srvTLS.Certificates = []tls.Certificate{srvCert}

	var gotAuth, gotPeerCN string
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
			gotPeerCN = r.TLS.PeerCertificates[0].Subject.CommonName
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"updated":1,"already_set":0,"not_found":0}`))
	}))
	srv.TLS = srvTLS
	srv.StartTLS()
	t.Cleanup(srv.Close)

	client, err := newRAGClient(srv.URL, logger)
	require.NoError(t, err)

	resp, err := client.BackfillDocumentOwnersWithResponse(
		context.Background(),
		rag_gateway.BackfillDocumentOwnersJSONRequestBody{
			Items: []rag_gateway.OwnerBackfillItem{{ArticleId: "a1", UserId: "u1"}},
		},
	)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	assert.Equal(t, "Bearer "+testToken, gotAuth)
	assert.Equal(t, "alt-backend", gotPeerCN)
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

func TestLoadDBPassword_ErrorCases(t *testing.T) {
	t.Run("neither password var set", func(t *testing.T) {
		t.Setenv("DB_PASSWORD_FILE", "")
		t.Setenv("DB_PASSWORD", "")

		_, err := loadDBPassword()
		require.Error(t, err)
		assert.ErrorContains(t, err, "DB_PASSWORD_FILE or DB_PASSWORD is required")
	})

	t.Run("unreadable DB_PASSWORD_FILE", func(t *testing.T) {
		t.Setenv("DB_PASSWORD_FILE", "/nonexistent/postgres_password")
		t.Setenv("DB_PASSWORD", "fallback-not-used")

		_, err := loadDBPassword()
		require.Error(t, err)
		assert.ErrorContains(t, err, "read DB_PASSWORD_FILE")
		assert.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("empty DB_PASSWORD_FILE", func(t *testing.T) {
		emptyFile := filepath.Join(t.TempDir(), "empty_password")
		require.NoError(t, os.WriteFile(emptyFile, []byte("  \n\t "), 0o600))

		t.Setenv("DB_PASSWORD_FILE", emptyFile)
		t.Setenv("DB_PASSWORD", "fallback-not-used")

		_, err := loadDBPassword()
		require.Error(t, err)
		assert.ErrorContains(t, err, "empty password")
	})
}

func TestBuildPGXPoolConfig_MissingRequiredSettings_ReturnsError(t *testing.T) {
	setValidEnv := func(t *testing.T) {
		t.Helper()
		t.Setenv("DB_HOST", "pgbouncer")
		t.Setenv("DB_PORT", "6432")
		t.Setenv("DB_USER", "alt_db_user")
		t.Setenv("DB_NAME", "alt")
		t.Setenv("DB_PASSWORD_FILE", "")
		t.Setenv("DB_PASSWORD", "secret")
	}

	t.Run("missing DB_HOST", func(t *testing.T) {
		setValidEnv(t)
		t.Setenv("DB_HOST", "")

		_, err := buildPGXPoolConfig()
		require.Error(t, err)
		assert.ErrorContains(t, err, "DB_HOST")
	})

	t.Run("missing DB_USER", func(t *testing.T) {
		setValidEnv(t)
		t.Setenv("DB_USER", "")

		_, err := buildPGXPoolConfig()
		require.Error(t, err)
		assert.ErrorContains(t, err, "DB_USER")
	})

	t.Run("missing DB_NAME", func(t *testing.T) {
		setValidEnv(t)
		t.Setenv("DB_NAME", "")

		_, err := buildPGXPoolConfig()
		require.Error(t, err)
		assert.ErrorContains(t, err, "DB_NAME")
	})
}

func TestBuildPGXPoolConfig_SpecialCharactersAndSSLModePrefer(t *testing.T) {
	rawPassword := "p@ss:w/rd%20#?&=!secret"
	t.Setenv("DB_PASSWORD_FILE", "")
	t.Setenv("DB_PASSWORD", rawPassword)
	t.Setenv("DB_HOST", "pgbouncer")
	t.Setenv("DB_PORT", "")
	t.Setenv("DB_USER", "alt_db_user")
	t.Setenv("DB_NAME", "alt")

	cfg, err := buildPGXPoolConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, rawPassword, cfg.ConnConfig.Password)
	assert.Equal(t, "pgbouncer", cfg.ConnConfig.Host)
	assert.Equal(t, uint16(5432), cfg.ConnConfig.Port)
	assert.Equal(t, "alt_db_user", cfg.ConnConfig.User)
	assert.Equal(t, "alt", cfg.ConnConfig.Database)
	require.NotNil(t, cfg.ConnConfig.TLSConfig, "sslmode=prefer must set initial TLSConfig")
	require.Len(t, cfg.ConnConfig.Fallbacks, 1, "sslmode=prefer must configure one plaintext fallback")
	assert.Nil(t, cfg.ConnConfig.Fallbacks[0].TLSConfig)
	assert.Equal(t, pgx.QueryExecModeSimpleProtocol, cfg.ConnConfig.DefaultQueryExecMode)
}
