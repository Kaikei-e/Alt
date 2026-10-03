package datahub

import (
	"context"
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
	"strings"
	"testing"
	"time"

	"alt/config"
	"alt/tlsutil"
)

func writeTestPKI(t *testing.T, dir string, cn string) (certPath, keyPath, caPath string) {
	return writeTestPKIWithHosts(t, dir, cn, []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")})
}

func writeTestPKIWithHosts(t *testing.T, dir string, cn string, dnsNames []string, ipAddrs []net.IP) (certPath, keyPath, caPath string) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "peer-ca-" + cn},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caPath = filepath.Join(dir, "ca.pem")
	caOut, err := os.Create(caPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := pem.Encode(caOut, &pem.Block{Type: "CERTIFICATE", Bytes: caDER}); err != nil {
		t.Fatal(err)
	}
	if err := caOut.Close(); err != nil {
		t.Fatal(err)
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:     dnsNames,
		IPAddresses:  ipAddrs,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caTmpl, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	certPath = filepath.Join(dir, "cert.pem")
	certOut, err := os.Create(certPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: leafDER}); err != nil {
		t.Fatal(err)
	}
	if err := certOut.Close(); err != nil {
		t.Fatal(err)
	}

	keyPath = filepath.Join(dir, "key.pem")
	keyOut, err := os.Create(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	keyBytes, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}); err != nil {
		t.Fatal(err)
	}
	if err := keyOut.Close(); err != nil {
		t.Fatal(err)
	}

	return certPath, keyPath, caPath
}

func TestDataHubComponents_KratosClientPresentsInternalAuthSecret(t *testing.T) {
	const (
		backendTokenSecret = "hs256-signing-key-that-must-not-reach-a-plaintext-header"
		internalAuthSecret = "the-separate-internal-shared-bearer-value"
	)

	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKI(t, dir, "DataHub")

	tlsCfg, err := tlsutil.LoadServerConfig(certPath, keyPath, caPath, tlsutil.WithClientAuth(tls.RequireAndVerifyClientCert))
	if err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	var presented string
	srv := tlsutil.NewMTLSHTTPServer(ln.Addr().String(), tlsCfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		presented = r.Header.Get("X-Internal-Auth")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user_id":"11111111-2222-3333-4444-555555555555"}`))
	}))
	go func() { _ = srv.Serve(tls.NewListener(ln, tlsCfg)) }()
	defer func() {
		if err := srv.Close(); err != nil {
			t.Errorf("srv.Close() error = %v", err)
		}
	}()

	cfg := &config.Config{
		AppEnv:    "development",
		AuthHub:   config.AuthHubConfig{URL: "https://" + ln.Addr().String()},
		Auth:      config.AuthConfig{BackendTokenSecret: backendTokenSecret, InternalAuthSecret: internalAuthSecret},
		MQHub:     config.MQHubConfig{Enabled: true, ConnectURL: "http://mq-hub:9500"},
		Sovereign: config.SovereignConfig{URL: "http://knowledge-sovereign:9500"},
		Recap:     config.RecapConfig{DefaultPageSize: 500, MaxPageSize: 2000, MaxRangeDays: 8},
	}

	client, err := tlsutil.NewMTLSClient(certPath, keyPath, caPath)
	if err != nil {
		t.Fatal(err)
	}

	components := NewDataHubComponents(nil, cfg, client)

	if _, err := components.KratosClient.GetFirstIdentityID(context.Background()); err != nil {
		t.Fatalf("GetFirstIdentityID() error = %v", err)
	}

	if presented != internalAuthSecret {
		t.Errorf("X-Internal-Auth = %q, want INTERNAL_AUTH_SECRET %q", presented, internalAuthSecret)
	}

	if presented == backendTokenSecret {
		t.Errorf("X-Internal-Auth must not equal backend token secret (HMAC)")
	}
}

func TestDataHubComponents_MTLSClientValidWire(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKI(t, dir, "DataHub")

	tlsCfg, err := tlsutil.LoadServerConfig(certPath, keyPath, caPath, tlsutil.WithClientAuth(tls.RequireAndVerifyClientCert))
	if err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := tlsutil.NewMTLSHTTPServer(ln.Addr().String(), tlsCfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		presented := r.Header.Get("X-Internal-Auth")
		if presented != "test-internal-secret" {
			t.Errorf("wrong internal auth")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user_id":"11111111-2222-3333-4444-555555555555"}`))
	}))
	go func() { _ = srv.Serve(tls.NewListener(ln, tlsCfg)) }()
	defer func() {
		if err := srv.Close(); err != nil {
			t.Errorf("srv.Close() error = %v", err)
		}
	}()

	cfg := &config.Config{
		AppEnv:    "development",
		AuthHub:   config.AuthHubConfig{URL: "https://" + ln.Addr().String()},
		Auth:      config.AuthConfig{BackendTokenSecret: "a", InternalAuthSecret: "test-internal-secret"},
		MQHub:     config.MQHubConfig{Enabled: true, ConnectURL: "http://mq-hub:9500"},
		Sovereign: config.SovereignConfig{URL: "http://knowledge-sovereign:9500"},
		Recap:     config.RecapConfig{DefaultPageSize: 500, MaxPageSize: 2000, MaxRangeDays: 8},
	}

	client, err := tlsutil.NewMTLSClient(certPath, keyPath, caPath)
	if err != nil {
		t.Fatal(err)
	}

	components := NewDataHubComponents(nil, cfg, client)
	id, err := components.KratosClient.GetFirstIdentityID(context.Background())
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if id != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("unexpected id %q", id)
	}
}

func TestDataHubComponents_MTLSClientRejectsWrongCA(t *testing.T) {
	// Server uses CA A
	dirA := t.TempDir()
	certPathA, keyPathA, caPathA := writeTestPKI(t, dirA, "ServerHub")
	tlsCfg, err := tlsutil.LoadServerConfig(certPathA, keyPathA, caPathA, tlsutil.WithClientAuth(tls.RequireAndVerifyClientCert))
	if err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := tlsutil.NewMTLSHTTPServer(ln.Addr().String(), tlsCfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user_id":"11111111-2222-3333-4444-555555555555"}`))
	}))
	go func() { _ = srv.Serve(tls.NewListener(ln, tlsCfg)) }()
	defer func() {
		if err := srv.Close(); err != nil {
			t.Errorf("srv.Close() error = %v", err)
		}
	}()

	// Client uses CA B (foreign CA, not trusted by server and server not trusted by client)
	dirB := t.TempDir()
	certPathB, keyPathB, caPathB := writeTestPKI(t, dirB, "DataHubClient")
	clientB, err := tlsutil.NewMTLSClient(certPathB, keyPathB, caPathB)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		AppEnv:    "development",
		AuthHub:   config.AuthHubConfig{URL: "https://" + ln.Addr().String()},
		Auth:      config.AuthConfig{BackendTokenSecret: "a", InternalAuthSecret: "b"},
		MQHub:     config.MQHubConfig{Enabled: true, ConnectURL: "http://mq-hub:9500"},
		Sovereign: config.SovereignConfig{URL: "http://knowledge-sovereign:9500"},
		Recap:     config.RecapConfig{DefaultPageSize: 500, MaxPageSize: 2000, MaxRangeDays: 8},
	}

	components := NewDataHubComponents(nil, cfg, clientB)
	_, err = components.KratosClient.GetFirstIdentityID(context.Background())
	if err == nil {
		t.Fatalf("expected wire mTLS error when connecting with wrong CA, got nil")
	}
}

func TestDataHubComponents_MTLSClientRejectsWrongHostname(t *testing.T) {
	// Server cert has SAN for "otherhost.internal" only, NOT localhost or 127.0.0.1
	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKIWithHosts(t, dir, "ServerHub", []string{"otherhost.internal"}, nil)
	tlsCfg, err := tlsutil.LoadServerConfig(certPath, keyPath, caPath, tlsutil.WithClientAuth(tls.RequireAndVerifyClientCert))
	if err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := tlsutil.NewMTLSHTTPServer(ln.Addr().String(), tlsCfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user_id":"11111111-2222-3333-4444-555555555555"}`))
	}))
	go func() { _ = srv.Serve(tls.NewListener(ln, tlsCfg)) }()
	defer func() {
		if err := srv.Close(); err != nil {
			t.Errorf("srv.Close() error = %v", err)
		}
	}()

	// Client dials by IP address 127.0.0.1
	client, err := tlsutil.NewMTLSClient(certPath, keyPath, caPath)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		AppEnv:    "development",
		AuthHub:   config.AuthHubConfig{URL: "https://" + ln.Addr().String()},
		Auth:      config.AuthConfig{BackendTokenSecret: "a", InternalAuthSecret: "b"},
		MQHub:     config.MQHubConfig{Enabled: true, ConnectURL: "http://mq-hub:9500"},
		Sovereign: config.SovereignConfig{URL: "http://knowledge-sovereign:9500"},
		Recap:     config.RecapConfig{DefaultPageSize: 500, MaxPageSize: 2000, MaxRangeDays: 8},
	}

	components := NewDataHubComponents(nil, cfg, client)
	_, err = components.KratosClient.GetFirstIdentityID(context.Background())
	if err == nil {
		t.Fatalf("expected wire TLS hostname mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "certificate") && !strings.Contains(err.Error(), "IP") && !strings.Contains(err.Error(), "name") {
		t.Errorf("expected hostname verification error, got: %v", err)
	}
}

func TestDataHubComponents_MTLSClientRejectsMissingClientCert(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := writeTestPKI(t, dir, "DataHub")

	tlsCfg, err := tlsutil.LoadServerConfig(certPath, keyPath, caPath, tlsutil.WithClientAuth(tls.RequireAndVerifyClientCert))
	if err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := tlsutil.NewMTLSHTTPServer(ln.Addr().String(), tlsCfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user_id":"11111111-2222-3333-4444-555555555555"}`))
	}))
	go func() { _ = srv.Serve(tls.NewListener(ln, tlsCfg)) }()
	defer func() {
		if err := srv.Close(); err != nil {
			t.Errorf("srv.Close() error = %v", err)
		}
	}()

	// Client trusts CA, but presents NO client certificate (standard TLS client)
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		t.Fatal(err)
	}
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(caPEM)

	clientWithoutCert := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: certPool,
			},
		},
	}

	cfg := &config.Config{
		AppEnv:    "development",
		AuthHub:   config.AuthHubConfig{URL: "https://" + ln.Addr().String()},
		Auth:      config.AuthConfig{BackendTokenSecret: "a", InternalAuthSecret: "b"},
		MQHub:     config.MQHubConfig{Enabled: true, ConnectURL: "http://mq-hub:9500"},
		Sovereign: config.SovereignConfig{URL: "http://knowledge-sovereign:9500"},
		Recap:     config.RecapConfig{DefaultPageSize: 500, MaxPageSize: 2000, MaxRangeDays: 8},
	}

	components := NewDataHubComponents(nil, cfg, clientWithoutCert)
	_, err = components.KratosClient.GetFirstIdentityID(context.Background())
	if err == nil {
		t.Fatalf("expected wire mTLS error when client cert is missing, got nil")
	}
}

func TestDataHubComponents_ProductionConstructorFailsClosed(t *testing.T) {
	t.Run("panics when AuthHub URL is plaintext HTTP", func(t *testing.T) {
		cfg := &config.Config{
			AppEnv:    "development",
			AuthHub:   config.AuthHubConfig{URL: "http://auth-hub:8888"},
			Auth:      config.AuthConfig{BackendTokenSecret: "a", InternalAuthSecret: "b"},
			MQHub:     config.MQHubConfig{Enabled: true, ConnectURL: "http://mq-hub:9500"},
			Sovereign: config.SovereignConfig{URL: "http://knowledge-sovereign:9500"},
			Recap:     config.RecapConfig{DefaultPageSize: 500, MaxPageSize: 2000, MaxRangeDays: 8},
		}

		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("expected panic on plaintext HTTP URL, got none")
			}
		}()

		_ = NewDataHubComponents(nil, cfg, &http.Client{})
	})

	t.Run("panics when mtlsClient is nil", func(t *testing.T) {
		cfg := &config.Config{
			AppEnv:    "development",
			AuthHub:   config.AuthHubConfig{URL: "https://auth-hub:9443"},
			Auth:      config.AuthConfig{BackendTokenSecret: "a", InternalAuthSecret: "b"},
			MQHub:     config.MQHubConfig{Enabled: true, ConnectURL: "http://mq-hub:9500"},
			Sovereign: config.SovereignConfig{URL: "http://knowledge-sovereign:9500"},
			Recap:     config.RecapConfig{DefaultPageSize: 500, MaxPageSize: 2000, MaxRangeDays: 8},
		}

		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("expected panic on nil mtlsClient, got none")
			}
		}()

		_ = NewDataHubComponents(nil, cfg, nil)
	})
}
