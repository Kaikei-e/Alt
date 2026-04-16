package handler

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
	"testing"
	"time"
)

func selfSignedPair(t *testing.T, cn string) (certPEM, keyPEM []byte) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		DNSNames:     []string{cn, "localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-1 * time.Minute),
		NotAfter:     time.Now().Add(1 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IsCA:         true,
		BasicConstraintsValid: true,
	}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, _ := x509.MarshalECPrivateKey(key)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return
}

func writeCert(t *testing.T, dir, certName, keyName string, certPEM, keyPEM []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, certName), certPEM, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, keyName), keyPEM, 0o400); err != nil {
		t.Fatal(err)
	}
}

func TestNewTLSProxy_PeerIdentityHeader(t *testing.T) {
	dir := t.TempDir()
	serverCert, serverKey := selfSignedPair(t, "test-server")
	clientCert, clientKey := selfSignedPair(t, "test-client")
	writeCert(t, dir, "svc-cert.pem", "svc-key.pem", serverCert, serverKey)
	caBundlePath := filepath.Join(dir, "ca-bundle.pem")
	_ = os.WriteFile(caBundlePath, clientCert, 0o444) // client cert is self-signed CA

	var gotPeerIdentity string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPeerIdentity = r.Header.Get("X-Alt-Peer-Identity")
		w.WriteHeader(200)
	}))
	defer upstream.Close()

	cfg := ProxyConfig{
		Listen:       ":0",
		Upstream:     upstream.URL,
		CertPath:     filepath.Join(dir, "svc-cert.pem"),
		KeyPath:      filepath.Join(dir, "svc-key.pem"),
		CAPath:       caBundlePath,
		VerifyClient: true,
		AllowedPeers: []string{"test-client"},
	}
	srv, err := NewTLSProxy(cfg)
	if err != nil {
		t.Fatal(err)
	}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", srv.TLSConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go srv.Serve(ln)

	// Build mTLS client with client cert.
	clientTLS, _ := tls.X509KeyPair(clientCert, clientKey)
	serverCAPool := x509.NewCertPool()
	serverCAPool.AppendCertsFromPEM(serverCert)
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{clientTLS},
				RootCAs:      serverCAPool,
				MinVersion:   tls.VersionTLS13,
			},
		},
	}

	resp, err := client.Get("https://" + ln.Addr().String() + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if gotPeerIdentity != "test-client" {
		t.Fatalf("X-Alt-Peer-Identity=%q want test-client", gotPeerIdentity)
	}
}

func TestNewTLSProxy_RejectsUnauthenticatedClient(t *testing.T) {
	dir := t.TempDir()
	serverCert, serverKey := selfSignedPair(t, "test-server")
	writeCert(t, dir, "svc-cert.pem", "svc-key.pem", serverCert, serverKey)
	caBundlePath := filepath.Join(dir, "ca-bundle.pem")
	_ = os.WriteFile(caBundlePath, serverCert, 0o444)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("upstream should not be reached")
		w.WriteHeader(200)
	}))
	defer upstream.Close()

	cfg := ProxyConfig{
		Listen:       ":0",
		Upstream:     upstream.URL,
		CertPath:     filepath.Join(dir, "svc-cert.pem"),
		KeyPath:      filepath.Join(dir, "svc-key.pem"),
		CAPath:       caBundlePath,
		VerifyClient: true,
		AllowedPeers: []string{"some-peer"},
	}
	srv, err := NewTLSProxy(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", srv.TLSConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go srv.Serve(ln)

	// Client WITHOUT client cert — should be rejected.
	serverCAPool := x509.NewCertPool()
	serverCAPool.AppendCertsFromPEM(serverCert)
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    serverCAPool,
				MinVersion: tls.VersionTLS13,
			},
		},
	}
	_, err = client.Get("https://" + ln.Addr().String() + "/")
	if err == nil {
		t.Fatal("expected TLS handshake to fail without client cert")
	}
}
