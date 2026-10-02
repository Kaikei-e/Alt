package main

// main_tls_test.go tests the actual main production TLS server and public-health
// constructor seams (newProductionTLSServer and newHealthServer) factored out of
// cmd/server/main.go.
//
// Tests run fully in-process on real TCP wire listeners (net.Listen 127.0.0.1:0)
// using throwaway ephemeral PKI (crypto/x509, crypto/ecdsa) in t.TempDir().
//
// Exercises:
//   - Genuine production TLS server constructor (newProductionTLSServer)
//   - Real wire TLS 1.3 handshake with RequireAndVerifyClientCert
//   - Positive case: client with trusted CA cert and valid hostname succeeds
//   - Missing client cert: handshake rejected
//   - Wrong CA client cert: handshake rejected
//   - Wrong hostname / SNI: certificate verification rejected
//   - Missing cert at server constructor: startup failure returned
//   - Dedicated public health server (newHealthServer): serves ONLY /healthz
//     and /readyz, never business RPC routes (responds 404).

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ─── ephemeral PKI helpers ────────────────────────────────────────────────────

type tlsPKI struct {
	caCert   *x509.Certificate
	caKey    *ecdsa.PrivateKey
	caDER    []byte
	CertPath string
	KeyPath  string
	CAPath   string
}

func newEphemeralPKI(t *testing.T, dir, cn string) *tlsPKI {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen CA key: %v", err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca-" + cn},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA cert: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA cert: %v", err)
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen leaf key: %v", err)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:     []string{cn, "localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create leaf cert: %v", err)
	}

	certPath := filepath.Join(dir, "svc-cert.pem")
	keyPath := filepath.Join(dir, "svc-key.pem")
	caPath := filepath.Join(dir, "ca-bundle.pem")

	writePKIPEM(t, certPath, "CERTIFICATE", leafDER)
	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		t.Fatalf("marshal leaf key: %v", err)
	}
	writePKIPEM(t, keyPath, "EC PRIVATE KEY", leafKeyDER)
	writePKIPEM(t, caPath, "CERTIFICATE", caDER)

	return &tlsPKI{
		caCert:   caCert,
		caKey:    caKey,
		caDER:    caDER,
		CertPath: certPath,
		KeyPath:  keyPath,
		CAPath:   caPath,
	}
}

func (p *tlsPKI) issueLeaf(t *testing.T, dir, cn string) (certPath, keyPath string) {
	t.Helper()
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen leaf key: %v", err)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(42),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, p.caCert, &leafKey.PublicKey, p.caKey)
	if err != nil {
		t.Fatalf("create leaf: %v", err)
	}
	certPath = filepath.Join(dir, cn+"-cert.pem")
	keyPath = filepath.Join(dir, cn+"-key.pem")
	writePKIPEM(t, certPath, "CERTIFICATE", leafDER)
	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	writePKIPEM(t, keyPath, "EC PRIVATE KEY", leafKeyDER)
	return certPath, keyPath
}

func writePKIPEM(t *testing.T, path, blockType string, der []byte) {
	t.Helper()
	f, err := os.Create(path) // #nosec G304
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	if err := pem.Encode(f, &pem.Block{Type: blockType, Bytes: der}); err != nil {
		t.Fatalf("pem encode %s: %v", path, err)
	}
}

func caPool(t *testing.T, caPath string) *x509.CertPool {
	t.Helper()
	b, err := os.ReadFile(caPath) // #nosec G304
	if err != nil {
		t.Fatalf("read CA %s: %v", caPath, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(b) {
		t.Fatalf("no certs in %s", caPath)
	}
	return pool
}

// startRealWireTLSServer binds the production *http.Server to a real TCP wire listener.
func startRealWireTLSServer(t *testing.T, srv *http.Server) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on real wire: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		_ = srv.ServeTLS(ln, "", "")
	}()
	t.Cleanup(func() { _ = srv.Close() })
	return ln.Addr().String()
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestProductionTLSServer_RequireAndVerifyClientCert verifies that the genuine
// newProductionTLSServer constructor seam produces a server that accepts valid
// mTLS connections over real TCP wire.
func TestProductionTLSServer_RequireAndVerifyClientCert(t *testing.T) {
	dir := t.TempDir()
	srvPKI := newEphemeralPKI(t, dir, "rag-orchestrator")
	clientCertPath, clientKeyPath := srvPKI.issueLeaf(t, dir, "alt-backend")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	srv, err := newProductionTLSServer("127.0.0.1:0", handler, srvPKI.CertPath, srvPKI.KeyPath, srvPKI.CAPath)
	if err != nil {
		t.Fatalf("newProductionTLSServer: %v", err)
	}
	addr := startRealWireTLSServer(t, srv)

	clientCertPair, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
	if err != nil {
		t.Fatalf("load client keypair: %v", err)
	}
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      caPool(t, srvPKI.CAPath),
				Certificates: []tls.Certificate{clientCertPair},
				ServerName:   "rag-orchestrator",
				MinVersion:   tls.VersionTLS13,
			},
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get("https://" + addr + "/healthz")
	if err != nil {
		t.Fatalf("request with valid mTLS client cert failed over real wire: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
}

// TestProductionTLSServer_NoCertRejected verifies that the production TLS server
// rejects clients presenting no client certificate.
func TestProductionTLSServer_NoCertRejected(t *testing.T) {
	dir := t.TempDir()
	srvPKI := newEphemeralPKI(t, dir, "rag-orchestrator")

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv, err := newProductionTLSServer("127.0.0.1:0", handler, srvPKI.CertPath, srvPKI.KeyPath, srvPKI.CAPath)
	if err != nil {
		t.Fatalf("newProductionTLSServer: %v", err)
	}
	addr := startRealWireTLSServer(t, srv)

	noCertClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    caPool(t, srvPKI.CAPath),
				ServerName: "rag-orchestrator",
				MinVersion: tls.VersionTLS13,
			},
		},
		Timeout: 5 * time.Second,
	}
	_, err = noCertClient.Get("https://" + addr + "/healthz")
	if err == nil {
		t.Fatal("expected TLS handshake error for client without cert; got nil")
	}
}

// TestProductionTLSServer_WrongCA verifies that the production TLS server
// rejects client certs signed by an untrusted CA.
func TestProductionTLSServer_WrongCA(t *testing.T) {
	dir := t.TempDir()
	srvPKI := newEphemeralPKI(t, dir, "rag-orchestrator")

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv, err := newProductionTLSServer("127.0.0.1:0", handler, srvPKI.CertPath, srvPKI.KeyPath, srvPKI.CAPath)
	if err != nil {
		t.Fatalf("newProductionTLSServer: %v", err)
	}
	addr := startRealWireTLSServer(t, srv)

	// Issue cert with different CA
	altDir := t.TempDir()
	altPKI := newEphemeralPKI(t, altDir, "evil-service")
	altCertPair, err := tls.LoadX509KeyPair(altPKI.CertPath, altPKI.KeyPath)
	if err != nil {
		t.Fatalf("load alt keypair: %v", err)
	}

	wrongCAClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      caPool(t, srvPKI.CAPath),
				Certificates: []tls.Certificate{altCertPair},
				ServerName:   "rag-orchestrator",
				MinVersion:   tls.VersionTLS13,
			},
		},
		Timeout: 5 * time.Second,
	}
	_, err = wrongCAClient.Get("https://" + addr + "/healthz")
	if err == nil {
		t.Fatal("expected TLS error for client cert from wrong CA; got nil")
	}
}

// TestProductionTLSServer_WrongHostname verifies that certificate verification fails
// when client connects with an incorrect server name / hostname.
func TestProductionTLSServer_WrongHostname(t *testing.T) {
	dir := t.TempDir()
	srvPKI := newEphemeralPKI(t, dir, "rag-orchestrator")
	clientCertPath, clientKeyPath := srvPKI.issueLeaf(t, dir, "alt-backend")

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv, err := newProductionTLSServer("127.0.0.1:0", handler, srvPKI.CertPath, srvPKI.KeyPath, srvPKI.CAPath)
	if err != nil {
		t.Fatalf("newProductionTLSServer: %v", err)
	}
	addr := startRealWireTLSServer(t, srv)

	clientCertPair, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
	if err != nil {
		t.Fatalf("load client keypair: %v", err)
	}

	wrongHostClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      caPool(t, srvPKI.CAPath),
				Certificates: []tls.Certificate{clientCertPair},
				ServerName:   "wrong-host.internal", // does not match cert DNS or IP
				MinVersion:   tls.VersionTLS13,
			},
		},
		Timeout: 5 * time.Second,
	}

	_, err = wrongHostClient.Get("https://" + addr + "/healthz")
	if err == nil {
		t.Fatal("expected TLS hostname verification error for wrong ServerName; got nil")
	}
}

// TestProductionTLSServer_MissingCertStartupFailure verifies fail-closed startup:
// if cert files are missing/invalid, newProductionTLSServer returns an error.
func TestProductionTLSServer_MissingCertStartupFailure(t *testing.T) {
	dir := t.TempDir()
	srvPKI := newEphemeralPKI(t, dir, "rag-orchestrator")

	_, err := newProductionTLSServer(
		"127.0.0.1:0",
		nil,
		"/nonexistent/cert.pem",
		srvPKI.KeyPath,
		srvPKI.CAPath,
	)
	if err == nil {
		t.Fatal("expected error from newProductionTLSServer when cert file is missing; got nil")
	}
}

// TestHealthPort9012_PublicHealthOnly_NeverBusinessRPC validates the genuine
// public-health constructor seam (newHealthServer / newHealthMux) used by main.go.
// It verifies:
//  1. /healthz returns 200 OK without requiring client certs (plaintext HTTP).
//  2. /readyz returns 200 OK without requiring client certs.
//  3. Dedicated health port NEVER serves business RPC routes (responds 404).
func TestHealthPort9012_PublicHealthOnly_NeverBusinessRPC(t *testing.T) {
	healthSrv := newHealthServer("127.0.0.1:0", nil, nil)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on real wire for health server: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		_ = healthSrv.Serve(ln)
	}()
	t.Cleanup(func() { _ = healthSrv.Close() })

	baseURL := "http://" + ln.Addr().String()

	// 1. /healthz succeeds without cert
	respHealth, err := http.Get(baseURL + "/healthz") //nolint:noctx // test only
	if err != nil {
		t.Fatalf("GET /healthz failed: %v", err)
	}
	defer func() { _ = respHealth.Body.Close() }()
	if respHealth.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", respHealth.StatusCode)
	}

	// 2. /readyz succeeds without cert
	respReady, err := http.Get(baseURL + "/readyz") //nolint:noctx // test only
	if err != nil {
		t.Fatalf("GET /readyz failed: %v", err)
	}
	defer func() { _ = respReady.Body.Close() }()
	if respReady.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", respReady.StatusCode)
	}

	// 3. Dedicated health port MUST NEVER respond to business RPC routes (fail-closed 404)
	businessRoutes := []string{
		"/rag.v1.RagService/Retrieve",
		"/rag.v1.RagService/Answer",
		"/internal/rag/backfill",
		"/v1/rag/morning-letter",
		"/v1/rag/retrieve",
	}

	for _, route := range businessRoutes {
		resp, err := http.Post(baseURL+route, "application/json", nil) //nolint:noctx
		if err != nil {
			t.Fatalf("POST %s failed: %v", route, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("route %s on dedicated health port: got %d, want 404 (never business RPC)", route, resp.StatusCode)
		}
	}
}

// TestProductionTLSServer_ConfigShape verifies that the constructed server has
// the required security configuration: RequireAndVerifyClientCert and TLS 1.3 minimum.
func TestProductionTLSServer_ConfigShape(t *testing.T) {
	dir := t.TempDir()
	pki := newEphemeralPKI(t, dir, "rag-orchestrator")

	srv, err := newProductionTLSServer("127.0.0.1:0", nil, pki.CertPath, pki.KeyPath, pki.CAPath)
	if err != nil {
		t.Fatalf("newProductionTLSServer: %v", err)
	}

	tlsCfg := srv.TLSConfig
	if tlsCfg == nil {
		t.Fatal("TLSConfig must not be nil")
	}
	if tlsCfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Fatalf("ClientAuth: got %v, want RequireAndVerifyClientCert", tlsCfg.ClientAuth)
	}
	if tlsCfg.MinVersion != tls.VersionTLS13 {
		t.Fatalf("MinVersion: got %x, want TLS13 (%x)", tlsCfg.MinVersion, tls.VersionTLS13)
	}
	if tlsCfg.GetCertificate == nil {
		t.Fatal("GetCertificate must be set for hot-reload support (ListenAndServeTLS uses empty paths)")
	}
	if tlsCfg.ClientCAs == nil {
		t.Fatal("ClientCAs must be populated from the CA bundle")
	}
}
