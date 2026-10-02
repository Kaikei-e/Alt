package authhub

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTempMTLSCerts(t *testing.T) (certPath, keyPath, caPath string, caCert *x509.Certificate, serverTLS *tls.Config) {
	t.Helper()
	dir := t.TempDir()

	caPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "Alt Test CA",
			Organization: []string{"Alt Test"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	caBytes, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caPriv.PublicKey, caPriv)
	require.NoError(t, err)

	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caBytes})
	caPath = filepath.Join(dir, "ca-bundle.pem")
	require.NoError(t, os.WriteFile(caPath, caPEM, 0o600))

	genCert := func(cn string, isClient, isServer bool) (string, string) {
		priv, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		var extUsage []x509.ExtKeyUsage
		if isClient {
			extUsage = append(extUsage, x509.ExtKeyUsageClientAuth)
		}
		if isServer {
			extUsage = append(extUsage, x509.ExtKeyUsageServerAuth)
		}

		tmpl := &x509.Certificate{
			SerialNumber: big.NewInt(2),
			Subject: pkix.Name{
				CommonName:   cn,
				Organization: []string{"Alt Test"},
			},
			NotBefore:   time.Now().Add(-time.Hour),
			NotAfter:    time.Now().Add(24 * time.Hour),
			KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
			ExtKeyUsage: extUsage,
			IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.IPv6loopback},
			DNSNames:    []string{"localhost", "auth-hub"},
		}

		certDer, err := x509.CreateCertificate(rand.Reader, tmpl, caTemplate, &priv.PublicKey, caPriv)
		require.NoError(t, err)

		cPath := filepath.Join(dir, cn+".crt")
		kPath := filepath.Join(dir, cn+".key")

		cPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDer})
		kPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

		require.NoError(t, os.WriteFile(cPath, cPEM, 0o600))
		require.NoError(t, os.WriteFile(kPath, kPEM, 0o600))

		return cPath, kPath
	}

	certPath, keyPath = genCert("knowledge-sovereign", true, false)
	serverCertPath, serverKeyPath := genCert("localhost", false, true)

	serverCert, err := tls.LoadX509KeyPair(serverCertPath, serverKeyPath)
	require.NoError(t, err)

	clientCAPool := x509.NewCertPool()
	require.True(t, clientCAPool.AppendCertsFromPEM(caPEM))

	serverTLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    clientCAPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}

	return certPath, keyPath, caPath, caTemplate, serverTLS
}

func TestRemoteUserJWTVerifier_LiveMTLS(t *testing.T) {
	certPath, keyPath, caPath, _, serverTLS := setupTempMTLSCerts(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/internal/token/introspect", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if len(r.TLS.PeerCertificates) == 0 {
			http.Error(w, "missing peer certificate", http.StatusForbidden)
			return
		}
		peerCN := r.TLS.PeerCertificates[0].Subject.CommonName
		if peerCN != "knowledge-sovereign" {
			http.Error(w, "forbidden peer", http.StatusForbidden)
			return
		}

		var req introspectReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		switch req.Token {
		case "valid-token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"active":    true,
				"sub":       "a0000000-0000-0000-0000-000000000001",
				"tenant_id": "b0000000-0000-0000-0000-000000000002",
				"exp":       time.Now().Add(time.Hour).Unix(),
			})
		case "expired-token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"active":    true,
				"sub":       "a0000000-0000-0000-0000-000000000001",
				"tenant_id": "b0000000-0000-0000-0000-000000000002",
				"exp":       time.Now().Add(-time.Hour).Unix(),
			})
		case "inactive-token":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"active": false,
			})
		case "invalid-uuid":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"active":    true,
				"sub":       "not-a-uuid",
				"tenant_id": "b0000000-0000-0000-0000-000000000002",
				"exp":       time.Now().Add(time.Hour).Unix(),
			})
		case "redirect-me":
			http.Redirect(w, r, "/internal/token/introspect-redirected", http.StatusFound)
		case "valid-prefix-with-junk":
			_, _ = w.Write([]byte(`{"active":true,"sub":"a0000000-0000-0000-0000-000000000001","tenant_id":"b0000000-0000-0000-0000-000000000002","exp":` +
				time.Now().Add(time.Hour).Format("150405") + `} trailing junk data`))
		case "second-json-object":
			_, _ = w.Write([]byte(`{"active":true,"sub":"a0000000-0000-0000-0000-000000000001","tenant_id":"b0000000-0000-0000-0000-000000000002","exp":9999999999}{"active":false}`))
		case "oversized-suffix":
			validPrefix := `{"active":true,"sub":"a0000000-0000-0000-0000-000000000001","tenant_id":"b0000000-0000-0000-0000-000000000002","exp":9999999999}`
			padding := strings.Repeat(" ", 70*1024)
			_, _ = w.Write([]byte(validPrefix + padding))
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"active": false,
			})
		}
	})

	server := httptest.NewUnstartedServer(mux)
	server.TLS = serverTLS
	server.StartTLS()
	defer server.Close()

	verifier, err := NewRemoteUserJWTVerifier(Config{
		URL:      server.URL + "/internal/token/introspect",
		CertPath: certPath,
		KeyPath:  keyPath,
		CAPath:   caPath,
	})
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("valid token introspects successfully", func(t *testing.T) {
		claims, err := verifier.ValidateToken(ctx, "valid-token")
		require.NoError(t, err)
		assert.Equal(t, "a0000000-0000-0000-0000-000000000001", claims.Sub)
		assert.Equal(t, "b0000000-0000-0000-0000-000000000002", claims.TenantID)
	})

	t.Run("expired token rejected", func(t *testing.T) {
		_, err := verifier.ValidateToken(ctx, "expired-token")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expired")
	})

	t.Run("inactive token rejected", func(t *testing.T) {
		_, err := verifier.ValidateToken(ctx, "inactive-token")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "inactive")
	})

	t.Run("non-UUID subject rejected", func(t *testing.T) {
		_, err := verifier.ValidateToken(ctx, "invalid-uuid")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "UUID")
	})

	t.Run("empty token rejected", func(t *testing.T) {
		_, err := verifier.ValidateToken(ctx, "")
		require.Error(t, err)
	})

	t.Run("redirects prohibited to prevent JWT leakage", func(t *testing.T) {
		_, err := verifier.ValidateToken(ctx, "redirect-me")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "redirects are prohibited")
	})

	t.Run("valid prefix with trailing junk rejected", func(t *testing.T) {
		_, err := verifier.ValidateToken(ctx, "valid-prefix-with-junk")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "trailing")
	})

	t.Run("second json object rejected", func(t *testing.T) {
		_, err := verifier.ValidateToken(ctx, "second-json-object")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "trailing")
	})

	t.Run("oversized suffix rejected", func(t *testing.T) {
		_, err := verifier.ValidateToken(ctx, "oversized-suffix")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "maximum allowed size")
	})
}

func TestRemoteUserJWTVerifier_StrictHTTPS(t *testing.T) {
	_, err := NewRemoteUserJWTVerifier(Config{
		URL: "http://auth-hub:9443/internal/token/introspect",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "strict HTTPS")
}
