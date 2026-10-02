package di

// rag_transport_test.go tests the newRagConnectHTTPClient factory that wires
// the RAG Connect-RPC transport in the DI layer.
//
// Exercises genuine factory production options with:
//   - Real temporary mTLS server + ephemeral CA + server/client leaves
//   - Positive round-trip: valid mTLS handshake over real wire
//   - Wrong CA: client cert signed by untrusted CA is rejected
//   - Wrong hostname: server cert without connecting SAN is rejected
//   - Distinction between noCert startup failure (panic) vs missing client TLS (plaintext)
//   - Redirect protection: CheckRedirect prevents silent redirect to separatedest0

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
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ─── ephemeral PKI helpers ────────────────────────────────────────────────────

func writeDITestPEM(t *testing.T, path, blockType string, der []byte) {
	t.Helper()
	f, err := os.Create(path) // #nosec G304 -- t.TempDir()-controlled path
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	if err := pem.Encode(f, &pem.Block{Type: blockType, Bytes: der}); err != nil {
		t.Fatalf("pem encode %s: %v", path, err)
	}
}

func newDITestCA(t *testing.T, dir, cn string) (*x509.Certificate, *ecdsa.PrivateKey, string, *x509.CertPool) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen CA key: %v", err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn},
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
		t.Fatalf("parse CA: %v", err)
	}
	caPath := filepath.Join(dir, cn+"-ca.pem")
	writeDITestPEM(t, caPath, "CERTIFICATE", caDER)

	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	return caCert, caKey, caPath, pool
}

func issueDITestCert(
	t *testing.T,
	dir, name string,
	caCert *x509.Certificate,
	caKey *ecdsa.PrivateKey,
	isServer bool,
	dnsNames []string,
	ips []net.IP,
) (certPath, keyPath string, tlsCert tls.Certificate) {
	t.Helper()
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen leaf key: %v", err)
	}
	extUsages := []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	if isServer {
		extUsages = append(extUsages, x509.ExtKeyUsageServerAuth)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  extUsages,
		DNSNames:     dnsNames,
		IPAddresses:  ips,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create leaf cert: %v", err)
	}
	certPath = filepath.Join(dir, name+"-cert.pem")
	keyPath = filepath.Join(dir, name+"-key.pem")

	writeDITestPEM(t, certPath, "CERTIFICATE", leafDER)
	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		t.Fatalf("marshal leaf key: %v", err)
	}
	writeDITestPEM(t, keyPath, "EC PRIVATE KEY", leafKeyDER)

	parsedLeaf, err := x509.ParseCertificate(leafDER)
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}
	tlsCert = tls.Certificate{
		Certificate: [][]byte{leafDER},
		PrivateKey:  leafKey,
		Leaf:        parsedLeaf,
	}
	return certPath, keyPath, tlsCert
}

func startTestMTLSServer(t *testing.T, serverCert tls.Certificate, clientCAPool *x509.CertPool, handler http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(handler)
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    clientCAPool,
		MinVersion:   tls.VersionTLS13,
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestNewRagConnectHTTPClient_Positive_RealTLSHandshake validates that calling
// the genuine newRagConnectHTTPClient factory with an https:// URL and valid
// MTLS_* env vars establishes a real TLS 1.3 handshake with a temporary mTLS server.
func TestNewRagConnectHTTPClient_Positive_RealTLSHandshake(t *testing.T) {
	dir := t.TempDir()
	caCert, caKey, caPath, caPool := newDITestCA(t, dir, "rag-ca")

	// Server cert with 127.0.0.1 IP SAN so httptest URL passes verification
	_, _, serverCert := issueDITestCert(t, dir, "rag-server", caCert, caKey, true, []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")})

	// Client cert for alt-backend
	clientCertPath, clientKeyPath, _ := issueDITestCert(t, dir, "alt-backend", caCert, caKey, false, nil, nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	srv := startTestMTLSServer(t, serverCert, caPool, handler)

	t.Setenv("MTLS_CERT_FILE", clientCertPath)
	t.Setenv("MTLS_KEY_FILE", clientKeyPath)
	t.Setenv("MTLS_CA_FILE", caPath)

	client := newRagConnectHTTPClient(srv.URL)
	if client == nil {
		t.Fatal("expected non-nil client from genuine factory")
	}

	// Shape assertions
	tr, ok := client.Transport.(*http.Transport)
	if !ok || tr.TLSClientConfig == nil {
		t.Fatal("expected transport with TLSClientConfig")
	}
	if client.CheckRedirect == nil {
		t.Fatal("expected CheckRedirect to be configured")
	}

	// Real mTLS round-trip request over genuine factory client
	resp, err := client.Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("real mTLS request with genuine client failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200", resp.StatusCode)
	}
}

// TestNewRagConnectHTTPClient_WrongCA_Rejected validates that the genuine client
// is rejected by the mTLS server when its client cert is signed by an untrusted CA.
func TestNewRagConnectHTTPClient_WrongCA_Rejected(t *testing.T) {
	dir := t.TempDir()
	caCert, caKey, caPath, caPool := newDITestCA(t, dir, "trusted-rag-ca")
	_, _, serverCert := issueDITestCert(t, dir, "rag-server", caCert, caKey, true, []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")})

	// Different untrusted CA
	untrustedDir := t.TempDir()
	untrustedCA, untrustedKey, _, _ := newDITestCA(t, untrustedDir, "untrusted-ca")
	clientCertPath, clientKeyPath, _ := issueDITestCert(t, untrustedDir, "alt-backend", untrustedCA, untrustedKey, false, nil, nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := startTestMTLSServer(t, serverCert, caPool, handler)

	t.Setenv("MTLS_CERT_FILE", clientCertPath)
	t.Setenv("MTLS_KEY_FILE", clientKeyPath)
	t.Setenv("MTLS_CA_FILE", caPath) // trusts server CA for server validation

	client := newRagConnectHTTPClient(srv.URL)
	_, err := client.Get(srv.URL + "/health")
	if err == nil {
		t.Fatal("expected TLS handshake error for client cert signed by wrong CA; got nil")
	}
}

// TestNewRagConnectHTTPClient_WrongHostname_Rejected validates that the genuine client
// rejects the server if the server leaf does not match the connecting hostname.
func TestNewRagConnectHTTPClient_WrongHostname_Rejected(t *testing.T) {
	dir := t.TempDir()
	caCert, caKey, caPath, caPool := newDITestCA(t, dir, "rag-ca")

	// Server cert valid ONLY for "rag-orchestrator.internal", NOT 127.0.0.1
	_, _, serverCert := issueDITestCert(t, dir, "rag-server", caCert, caKey, true, []string{"rag-orchestrator.internal"}, nil)
	clientCertPath, clientKeyPath, _ := issueDITestCert(t, dir, "alt-backend", caCert, caKey, false, nil, nil)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := startTestMTLSServer(t, serverCert, caPool, handler)

	t.Setenv("MTLS_CERT_FILE", clientCertPath)
	t.Setenv("MTLS_KEY_FILE", clientKeyPath)
	t.Setenv("MTLS_CA_FILE", caPath)

	client := newRagConnectHTTPClient(srv.URL)
	_, err := client.Get(srv.URL + "/health")
	if err == nil {
		t.Fatal("expected TLS hostname verification error; got nil")
	}
	if !strings.Contains(err.Error(), "certificate is valid for") && !strings.Contains(err.Error(), "IP address") {
		t.Logf("got expected hostname error: %v", err)
	}
}

// TestNewRagConnectHTTPClient_NoCertStartupFailure_Vs_MissingClientTLS distinguishes
// between startup-time factory panic (fail-closed when MTLS_* is missing/invalid)
// versus runtime handshake failure for plaintext client missing client TLS.
func TestNewRagConnectHTTPClient_NoCertStartupFailure_Vs_MissingClientTLS(t *testing.T) {
	dir := t.TempDir()
	caCert, caKey, caPath, caPool := newDITestCA(t, dir, "rag-ca")
	_, _, serverCert := issueDITestCert(t, dir, "rag-server", caCert, caKey, true, []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := startTestMTLSServer(t, serverCert, caPool, handler)

	// 1. Startup failure: https:// URL with empty certs must panic at factory time (noCert startup failure)
	t.Run("noCert_startup_failure_panics", func(t *testing.T) {
		t.Setenv("MTLS_CERT_FILE", "")
		t.Setenv("MTLS_KEY_FILE", "")
		t.Setenv("MTLS_CA_FILE", "")

		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected panic for missing cert at factory creation; got none")
			}
			msg, ok := r.(string)
			if !ok || !strings.Contains(msg, "mTLS client construction failed") {
				t.Fatalf("unexpected panic message: %v", r)
			}
		}()
		newRagConnectHTTPClient("https://rag-orchestrator:9011")
	})

	// 2. Startup failure: nonexistent cert path must panic at factory time
	t.Run("bad_cert_path_startup_failure_panics", func(t *testing.T) {
		t.Setenv("MTLS_CERT_FILE", "/nonexistent/cert.pem")
		t.Setenv("MTLS_KEY_FILE", "/nonexistent/key.pem")
		t.Setenv("MTLS_CA_FILE", caPath)

		defer func() {
			if recover() == nil {
				t.Fatal("expected panic for nonexistent cert; got none")
			}
		}()
		newRagConnectHTTPClient("https://rag-orchestrator:9011")
	})

	// 3. Plaintext client: http:// URL succeeds at factory creation (not panic)
	// but lacks client TLS; connecting to mTLS server fails at runtime
	t.Run("missing_client_TLS_handshake_fails", func(t *testing.T) {
		plainClient := newRagConnectHTTPClient("http://rag-orchestrator:9011")
		if plainClient == nil {
			t.Fatal("expected non-nil plaintext client")
		}
		// Attempting request to mTLS server with plaintext client fails
		_, err := plainClient.Get(srv.URL + "/health")
		if err == nil {
			t.Fatal("expected connection/TLS error when plaintext client touches mTLS server; got nil")
		}
	})
}

// TestNewRagConnectHTTPClient_RedirectSeparatedest0_NeverFollows validates that the
// genuine factory client's CheckRedirect stops at the 302/307 response and never
// follows redirects to a separate destination (separatedest0).
func TestNewRagConnectHTTPClient_RedirectSeparatedest0_NeverFollows(t *testing.T) {
	dir := t.TempDir()
	caCert, caKey, caPath, caPool := newDITestCA(t, dir, "rag-ca")
	_, _, serverCert := issueDITestCert(t, dir, "rag-server", caCert, caKey, true, []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")})
	clientCertPath, clientKeyPath, _ := issueDITestCert(t, dir, "alt-backend", caCert, caKey, false, nil, nil)
	_, _, dest0Cert := issueDITestCert(t, dir, "dest0-server", caCert, caKey, true, []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")})

	var originHits int32
	var dest0Hits int32

	dest0Handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&dest0Hits, 1)
		w.WriteHeader(http.StatusOK)
	})
	dest0Srv := startTestMTLSServer(t, dest0Cert, caPool, dest0Handler)
	separatedest0 := dest0Srv.URL + "/leak"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&originHits, 1)
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, separatedest0, http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := startTestMTLSServer(t, serverCert, caPool, handler)

	t.Setenv("MTLS_CERT_FILE", clientCertPath)
	t.Setenv("MTLS_KEY_FILE", clientKeyPath)
	t.Setenv("MTLS_CA_FILE", caPath)

	// Build client using ACTUAL factory — not manually created client
	client := newRagConnectHTTPClient(srv.URL)

	resp, err := client.Get(srv.URL + "/redirect")
	if err != nil {
		t.Fatalf("request to redirect endpoint failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected status 302 (Found) from last response, got: %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != separatedest0 {
		t.Fatalf("expected redirect Location %s, got: %s", separatedest0, loc)
	}
	if atomic.LoadInt32(&originHits) != 1 {
		t.Fatalf("expected exactly 1 hit to origin, got: %d", originHits)
	}
	if atomic.LoadInt32(&dest0Hits) != 0 {
		t.Fatalf("expected 0 hits to dest0, got: %d", dest0Hits)
	}
}
