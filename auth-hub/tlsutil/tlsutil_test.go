package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makePeerCert(t *testing.T, cn string, sans ...string) *x509.Certificate {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "peer-ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		DNSNames:     sans,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caTmpl, &leafKey.PublicKey, caKey)
	require.NoError(t, err)
	parsed, err := x509.ParseCertificate(leafDER)
	require.NoError(t, err)
	return parsed
}

func TestLoadServerConfig_WithAllowedPeers_PromotesDefaultToRequireAndVerifyClientCert(t *testing.T) {
	certPath, keyPath := writeSelfSignedCert(t, "auth-hub")

	cfg, err := LoadServerConfig(certPath, keyPath, certPath, WithAllowedPeers("alt-backend", "search-indexer"))
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, tls.RequireAndVerifyClientCert, cfg.ClientAuth,
		"PKI-01: WithAllowedPeers without explicit WithClientAuth must enforce RequireAndVerifyClientCert")
	require.NotNil(t, cfg.VerifyConnection)
}

func TestLoadServerConfig_WithAllowedPeers_RejectsWeakClientAuth(t *testing.T) {
	certPath, keyPath := writeSelfSignedCert(t, "auth-hub")

	weakModes := []tls.ClientAuthType{
		tls.NoClientCert,
		tls.RequestClientCert,
		tls.RequireAnyClientCert,
		tls.VerifyClientCertIfGiven,
	}

	for _, mode := range weakModes {
		t.Run(ClientAuthLabel(mode), func(t *testing.T) {
			cfg, err := LoadServerConfig(
				certPath,
				keyPath,
				certPath,
				WithClientAuth(mode),
				WithAllowedPeers("alt-backend"),
			)
			require.Error(t, err)
			assert.Nil(t, cfg)
			assert.Contains(t, err.Error(), "tlsutil: WithAllowedPeers requires RequireAndVerifyClientCert")
		})
	}
}

func TestLoadServerConfig_WithAllowedPeers_AcceptsAndRejectsPeers(t *testing.T) {
	certPath, keyPath := writeSelfSignedCert(t, "auth-hub")

	cfg, err := LoadServerConfig(
		certPath,
		keyPath,
		certPath,
		WithClientAuth(tls.RequireAndVerifyClientCert),
		WithAllowedPeers("alt-backend", "search-indexer"),
	)
	require.NoError(t, err)
	require.NotNil(t, cfg.VerifyConnection)

	allowedPeer := makePeerCert(t, "alt-backend", "alt-backend")
	assert.NoError(t, cfg.VerifyConnection(tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{allowedPeer},
	}))

	unlistedPeer := makePeerCert(t, "rogue-service", "rogue-service")
	assert.Error(t, cfg.VerifyConnection(tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{unlistedPeer},
	}))

	assert.Error(t, cfg.VerifyConnection(tls.ConnectionState{}))
}

func TestOptionsFromEnv_WithAllowedPeers_EnforcesRequireAndVerifyClientCert(t *testing.T) {
	t.Setenv("MTLS_CLIENT_AUTH", "")
	t.Setenv("MTLS_ALLOWED_PEERS", "alt-backend,search-indexer")

	certPath, keyPath := writeSelfSignedCert(t, "auth-hub")
	cfg, err := LoadServerConfig(certPath, keyPath, certPath, OptionsFromEnv()...)
	require.NoError(t, err)
	assert.Equal(t, tls.RequireAndVerifyClientCert, cfg.ClientAuth)
}
