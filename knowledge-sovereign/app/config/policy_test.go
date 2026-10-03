package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"knowledge-sovereign/gateway/authgw"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTempTokenFile(t *testing.T, filename, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, filename)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func setupTempMTLSCerts(t *testing.T) (certPath, keyPath, caPath string, caCertPEM []byte, serverTLS *tls.Config) {
	t.Helper()
	dir := t.TempDir()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test-alt-ca",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}

	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)

	caCertPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	caPath = filepath.Join(dir, "ca-bundle.pem")
	require.NoError(t, os.WriteFile(caPath, caCertPEM, 0o600))

	// Client certificate (knowledge-sovereign)
	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			CommonName: "knowledge-sovereign",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	clientDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caTemplate, &clientKey.PublicKey, caKey)
	require.NoError(t, err)

	certPath = filepath.Join(dir, "svc-cert.pem")
	clientCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientDER})
	require.NoError(t, os.WriteFile(certPath, clientCertPEM, 0o600))

	clientKeyBytes, err := x509.MarshalECPrivateKey(clientKey)
	require.NoError(t, err)
	keyPath = filepath.Join(dir, "svc-key.pem")
	clientKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: clientKeyBytes})
	require.NoError(t, os.WriteFile(keyPath, clientKeyPEM, 0o600))

	// Server certificate (for mock auth-hub in tests)
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject: pkix.Name{
			CommonName: "auth-hub",
		},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.IPv6loopback},
		DNSNames:              []string{"localhost", "auth-hub"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caTemplate, &serverKey.PublicKey, caKey)
	require.NoError(t, err)

	serverCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER})
	serverKeyBytes, err := x509.MarshalECPrivateKey(serverKey)
	require.NoError(t, err)
	serverKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: serverKeyBytes})

	serverCert, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	require.NoError(t, err)

	clientCAPool := x509.NewCertPool()
	require.True(t, clientCAPool.AppendCertsFromPEM(caCertPEM))

	serverTLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientCAPool,
	}

	t.Setenv("MTLS_CERT_FILE", certPath)
	t.Setenv("MTLS_KEY_FILE", keyPath)
	t.Setenv("MTLS_CA_FILE", caPath)

	return certPath, keyPath, caPath, caCertPEM, serverTLS
}

func TestLoadPolicy_Success(t *testing.T) {
	tok1 := writeTempTokenFile(t, "tok1", "backend-service-super-secret-token-12345")
	tok2 := writeTempTokenFile(t, "tok2", "recap-service-super-secret-token-67890")

	policyJSON := `{
		"version": 1,
		"services": [
			{
				"name": "alt-backend",
				"token_file": "` + tok1 + `",
				"allowed_methods": ["/services.sovereign.v1.KnowledgeSovereignService/GetTrailFootprints"],
				"allowed_events": ["ArticleCreated"],
				"require_user_token": true,
				"allow_system_scope": false
			},
			{
				"name": "recap-worker",
				"token_file": "` + tok2 + `",
				"allowed_methods": ["/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent"],
				"allowed_events": ["recap.topic_snapshotted.v1"],
				"require_user_token": false,
				"allow_system_scope": false
			}
		]
	}`

	policyFile := writeTempTokenFile(t, "policy.json", policyJSON)
	policy, err := LoadPolicyFile(policyFile)
	require.NoError(t, err)
	require.NotNil(t, policy)
	require.Len(t, policy.Services, 2)

	svc1 := policy.FindServiceByName("alt-backend")
	require.NotNil(t, svc1)
	assert.Equal(t, "backend-service-super-secret-token-12345", svc1.Token)
	assert.True(t, svc1.AllowedMethods["/services.sovereign.v1.KnowledgeSovereignService/GetTrailFootprints"])
	assert.True(t, svc1.AllowedEvents["ArticleCreated"])
	assert.True(t, svc1.RequireUserToken)
	assert.False(t, svc1.AllowSystemScope)

	svc2 := policy.FindServiceByName("recap-worker")
	require.NotNil(t, svc2)
	assert.Equal(t, "recap-service-super-secret-token-67890", svc2.Token)
	assert.True(t, svc2.AllowedEvents["recap.topic_snapshotted.v1"])
}

func TestLoadPolicy_ExampleFixtureValid(t *testing.T) {
	fixturePath := filepath.Join("fixtures", "event_auth_policy.example.json")
	data, err := os.ReadFile(fixturePath)
	require.NoError(t, err, "example fixture must exist in owned config fixtures")

	// Create temp mock secrets so the example file paths or token references resolve
	dir := t.TempDir()
	backendTok := filepath.Join(dir, "backend_tok")
	operatorTok := filepath.Join(dir, "operator_tok")
	recapTok := filepath.Join(dir, "recap_tok")
	ragTok := filepath.Join(dir, "rag_tok")
	datahubTok := filepath.Join(dir, "datahub_tok")
	harvesterTok := filepath.Join(dir, "harvester_tok")

	require.NoError(t, os.WriteFile(backendTok, []byte("backend-sovereign-token-length-24-plus"), 0o600))
	require.NoError(t, os.WriteFile(operatorTok, []byte("operator-sovereign-token-24-plus-xyz"), 0o600))
	require.NoError(t, os.WriteFile(recapTok, []byte("recap-sovereign-token-length-24-plus-xyz"), 0o600))
	require.NoError(t, os.WriteFile(ragTok, []byte("rag-orchestrator-sovereign-token-24-plus"), 0o600))
	require.NoError(t, os.WriteFile(datahubTok, []byte("datahub-sovereign-token-length-24-plus-1"), 0o600))
	require.NoError(t, os.WriteFile(harvesterTok, []byte("harvester-sovereign-token-24-plus-xyz"), 0o600))

	// Replace the /run/secrets paths with test temp paths for validation
	substituted := string(data)
	substituted = replaceSecretPath(substituted, "/run/secrets/sovereign_backend_token", backendTok)
	substituted = replaceSecretPath(substituted, "/run/secrets/sovereign_operator_token", operatorTok)
	substituted = replaceSecretPath(substituted, "/run/secrets/sovereign_recap_token", recapTok)
	substituted = replaceSecretPath(substituted, "/run/secrets/sovereign_rag_token", ragTok)
	substituted = replaceSecretPath(substituted, "/run/secrets/sovereign_datahub_token", datahubTok)
	substituted = replaceSecretPath(substituted, "/run/secrets/sovereign_harvester_token", harvesterTok)

	testPolicyFile := filepath.Join(dir, "test_policy.json")
	require.NoError(t, os.WriteFile(testPolicyFile, []byte(substituted), 0o600))

	pol, err := LoadPolicyFile(testPolicyFile)
	require.NoError(t, err)
	assert.Len(t, pol.Services, 6)
}

func replaceSecretPath(content, oldPath, newPath string) string {
	return strings.ReplaceAll(content, oldPath, newPath)
}

func TestLoadPolicy_FailsOnDuplicateTokens(t *testing.T) {
	const duplicateToken = "identical-secret-shared-across-services-bad"
	policyJSON := `{
		"version": 1,
		"services": [
			{
				"name": "svc1",
				"token": "` + duplicateToken + `",
				"allowed_methods": ["/test"]
			},
			{
				"name": "svc2",
				"token": "` + duplicateToken + `",
				"allowed_methods": ["/test"]
			}
		]
	}`

	policyFile := writeTempTokenFile(t, "policy_dup.json", policyJSON)
	_, err := LoadPolicyFile(policyFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate token")
	assert.NotContains(t, err.Error(), duplicateToken, "error must be sanitized and never leak token values")
}

func TestLoadPolicy_FailsOnEmptyOrWhitespaceToken(t *testing.T) {
	tokEmpty := writeTempTokenFile(t, "empty_tok", "   \t\n  ")
	policyJSON := `{
		"version": 1,
		"services": [
			{
				"name": "svc1",
				"token_file": "` + tokEmpty + `"
			}
		]
	}`

	policyFile := writeTempTokenFile(t, "policy_empty.json", policyJSON)
	_, err := LoadPolicyFile(policyFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty token")
}

func TestLoadPolicy_FailsOnShortToken(t *testing.T) {
	const shortTok = "too-short"
	policyJSON := `{
		"version": 1,
		"services": [
			{
				"name": "svc1",
				"token": "` + shortTok + `"
			}
		]
	}`

	policyFile := writeTempTokenFile(t, "policy_short.json", policyJSON)
	_, err := LoadPolicyFile(policyFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "shorter than minimum length")
	assert.NotContains(t, err.Error(), shortTok, "error must be sanitized")
}

func TestLoadPolicy_FailsOnMissingPolicyFile(t *testing.T) {
	_, err := LoadPolicyFile("/nonexistent/path/to/policy.json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read policy file")
}

func TestLoadPolicy_FailsOnMissingTokenFile(t *testing.T) {
	policyJSON := `{
		"version": 1,
		"services": [
			{
				"name": "svc1",
				"token_file": "/nonexistent/token/file"
			}
		]
	}`

	policyFile := writeTempTokenFile(t, "policy_missing_token_file.json", policyJSON)
	_, err := LoadPolicyFile(policyFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read token_file")
}

func TestLoadConfig_PolicyPrecedenceNoLegacyEscape(t *testing.T) {
	setupTempMTLSCerts(t)

	tok := writeTempTokenFile(t, "tok", "service-token-that-is-long-enough-12345")
	policyJSON := `{
		"version": 1,
		"services": [
			{
				"name": "alt-backend",
				"token_file": "` + tok + `",
				"allowed_methods": ["/services.sovereign.v1.KnowledgeSovereignService/GetTrailFootprints"]
			}
		]
	}`
	policyFile := writeTempTokenFile(t, "policy.json", policyJSON)

	t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	t.Setenv("ADMIN_AUTH", "disabled")
	t.Setenv("EVENT_AUTH", "")
	t.Setenv("EVENT_TOKEN", "legacy-shared-token-value-12345")
	t.Setenv("EVENT_AUTH_POLICY_FILE", policyFile)

	cfg, err := Load()
	require.NoError(t, err)
	require.True(t, cfg.EventAuthEnabled)
	require.NotNil(t, cfg.AuthPolicy)
	assert.Empty(t, cfg.EventToken, "when policy is enabled, legacy shared token must be empty/disabled to prevent legacy escape")
}

func TestLoadConfig_UserJWTVerifierConfig(t *testing.T) {
	certPath, keyPath, caPath, _, _ := setupTempMTLSCerts(t)

	t.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	t.Setenv("ADMIN_AUTH", "disabled")
	t.Setenv("EVENT_AUTH", "disabled")
	t.Setenv("USER_JWT_INTROSPECTION_URL", "https://auth-hub:9443/internal/token/introspect")

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg.UserJWTVerifier)

	gw, ok := cfg.UserJWTVerifier.(*authgw.Gateway)
	require.True(t, ok)
	remoteVerifier, ok := gw.Driver().(*RemoteUserJWTVerifier)
	require.True(t, ok)
	assert.Equal(t, "https://auth-hub:9443/internal/token/introspect", remoteVerifier.URL())
	assert.Equal(t, certPath, remoteVerifier.CertPath())
	assert.Equal(t, keyPath, remoteVerifier.KeyPath())
	assert.Equal(t, caPath, remoteVerifier.CAPath())
}
