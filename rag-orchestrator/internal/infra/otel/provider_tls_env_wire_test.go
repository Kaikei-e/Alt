package otel

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
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

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// TestWire_RedirectFail verifies that the production HTTP client does NOT
// follow redirects. It uses a separate destination server with an atomic
// counter so we can assert origin ≥ 1 and destination == 0.
func TestWire_RedirectFail(t *testing.T) {
	originalTracer := otel.GetTracerProvider()
	originalLogger := global.GetLoggerProvider()
	defer func() {
		otel.SetTracerProvider(originalTracer)
		global.SetLoggerProvider(originalLogger)
	}()

	var destCount int32
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&destCount, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer dest.Close()

	var originCount int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&originCount, 1)
		http.Redirect(w, r, dest.URL+r.URL.Path, http.StatusFound) // #nosec G710 -- redirect to a controlled test server verifies exporter never follows it
	}))
	defer origin.Close()

	t.Setenv("OTEL_TRACE_SAMPLE_RATIO", "1.0")
	cfg := ConfigFromEnv()
	cfg.Enabled = true
	cfg.OTLPEndpoint = origin.URL
	cfg.IngestToken = "valid-token-123="

	ctx := context.Background()
	shutdown, err := InitProvider(ctx, cfg)
	if err != nil {
		t.Fatalf("InitProvider failed: %v", err)
	}
	defer func() {
		ctxShut, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = shutdown(ctxShut)
	}()

	tracer := otel.Tracer("wire-redirect-tracer")
	_, span := tracer.Start(ctx, "wire-redirect-span")
	span.End()

	logger := global.GetLoggerProvider().Logger("wire-redirect-logger")
	var rec log.Record
	logger.Emit(ctx, rec)

	ctxFlush, flushCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer flushCancel()

	if tp, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider); ok {
		_ = tp.ForceFlush(ctxFlush)
	} else {
		t.Errorf("TracerProvider is not sdktrace.TracerProvider")
	}

	if lp, ok := global.GetLoggerProvider().(*sdklog.LoggerProvider); ok {
		_ = lp.ForceFlush(ctxFlush)
	} else {
		t.Errorf("LoggerProvider is not sdklog.LoggerProvider")
	}

	time.Sleep(100 * time.Millisecond)

	if got := atomic.LoadInt32(&originCount); got < 1 {
		t.Errorf("Expected origin ≥ 1 requests, got %d", got)
	}
	if got := atomic.LoadInt32(&destCount); got != 0 {
		t.Errorf("Expected destination exactly 0 requests (redirect not followed), got %d", got)
	}
}

// TestWire_TokenFileLoader_Failures tests meaningful file-loader error paths
// for ResolveRaskIngestToken. Errors must NOT reflect file contents.
func TestWire_TokenFileLoader_Failures(t *testing.T) {
	ctx := context.Background()

	t.Setenv("RASK_INGEST_TOKEN", "")

	t.Run("missing_file", func(t *testing.T) {
		t.Setenv("RASK_INGEST_TOKEN_FILE", "/nonexistent/token/file")
		cfg := Config{
			Enabled:      true,
			OTLPEndpoint: "http://localhost:4318",
		}
		_, err := InitProvider(ctx, cfg)
		if err == nil {
			t.Fatal("expected error for missing token file")
		}
	})

	t.Run("unreadable_file", func(t *testing.T) {
		unreadableDir := t.TempDir()
		t.Setenv("RASK_INGEST_TOKEN", "")
		t.Setenv("RASK_INGEST_TOKEN_FILE", unreadableDir)
		cfg := Config{Enabled: true, OTLPEndpoint: "http://localhost:4318"}
		_, err := InitProvider(ctx, cfg)
		if err == nil {
			t.Fatal("expected error for unreadable token file (directory)")
		}
	})

	t.Run("valid_file", func(t *testing.T) {
		f, err := os.CreateTemp("", "token-valid-*")
		if err != nil {
			t.Fatalf("cannot create temp file: %v", err)
		}
		if _, err := f.WriteString("valid-file-token-123=\n"); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = os.Remove(f.Name()) }()

		t.Setenv("RASK_INGEST_TOKEN", "")
		t.Setenv("RASK_INGEST_TOKEN_FILE", f.Name())
		cfg := Config{
			Enabled:      true,
			OTLPEndpoint: "http://localhost:4318",
		}
		shutdown, err := InitProvider(ctx, cfg)
		if err != nil {
			t.Fatalf("expected success loading valid token from file: %v", err)
		}
		_ = shutdown(ctx)
	})

	emptyStyleCases := []struct {
		name    string
		content string
	}{
		{"empty_file", ""},
		{"crlf_only", "\r\n"},
		{"space_only", "   "},
		{"dollar_only", "$"},
		{"padding_only", "===="},
		{"non_ascii", "\xff\xfe"},
		{"control_chars", "\x00\x01\x02"},
	}
	for _, tc := range emptyStyleCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			f, err := os.CreateTemp("", "token-bad-*")
			if err != nil {
				t.Fatalf("cannot create temp file: %v", err)
			}
			if _, err := f.WriteString(tc.content); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = os.Remove(f.Name()) }()

			t.Setenv("RASK_INGEST_TOKEN_FILE", f.Name())
			cfg := Config{Enabled: true, OTLPEndpoint: "http://localhost:4318"}
			_, err = InitProvider(ctx, cfg)
			if err == nil {
				t.Fatalf("expected error for content %q", tc.content)
			}
			if tc.content != "" && strings.Contains(err.Error(), tc.content) {
				t.Errorf("error must not reflect token content: %v", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TLS wire tests (Fix 3)
// ---------------------------------------------------------------------------

func wireSelfSignedCA(t *testing.T) (caPEM []byte, caKey *ecdsa.PrivateKey, caCert *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "wire-test-ca"},
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create CA cert: %v", err)
	}
	cert, _ := x509.ParseCertificate(der)
	caPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return caPEMBytes, key, cert
}

func wireServerCert(t *testing.T, caKey *ecdsa.PrivateKey, caCert *x509.Certificate, hosts []string) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: hosts[0]},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create server cert: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse server cert: %v", err)
	}
	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
		Leaf:        cert,
	}
}

func wireClientCert(t *testing.T, caKey *ecdsa.PrivateKey, caCert *x509.Certificate) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "wire-test-client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create client cert: %v", err)
	}
	keyDer, _ := x509.MarshalECPrivateKey(key)
	tlsCert, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDer}),
	)
	if err != nil {
		t.Fatalf("parse client key pair: %v", err)
	}
	return tlsCert
}

func wireWritePEM(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func wireNewTLSTestServer(t *testing.T, srvCert tls.Certificate, requireClientAuth bool, caPool *x509.CertPool) (*httptest.Server, *int32, *int32) {
	t.Helper()
	var traceReceived, logReceived int32
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/traces", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&traceReceived, 1)
		if _, err := io.ReadAll(r.Body); err != nil {
			t.Errorf("read exported telemetry: %v", err)
			http.Error(w, "invalid telemetry body", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/v1/logs", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&logReceived, 1)
		if _, err := io.ReadAll(r.Body); err != nil {
			t.Errorf("read exported telemetry: %v", err)
			http.Error(w, "invalid telemetry body", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewUnstartedServer(mux)
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{srvCert}}
	if requireClientAuth {
		srv.TLS.ClientAuth = tls.RequireAndVerifyClientCert
		srv.TLS.ClientCAs = caPool
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv, &traceReceived, &logReceived
}

func wireInitAndFlushBothSignals(t *testing.T, srvURL, token string) error {
	t.Helper()
	origTracer := otel.GetTracerProvider()
	origLogger := global.GetLoggerProvider()
	t.Cleanup(func() {
		otel.SetTracerProvider(origTracer)
		global.SetLoggerProvider(origLogger)
	})

	cfg := Config{
		Enabled:      true,
		OTLPEndpoint: srvURL,
		IngestToken:  token,
		ServiceName:  "wire-tls-test",
	}
	ctx := context.Background()
	shutdown, err := InitProvider(ctx, cfg)
	if err != nil {
		return err
	}
	t.Cleanup(func() {
		ctxShut, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = shutdown(ctxShut)
	})

	tracer := otel.Tracer("wire-tls-test-tracer")
	_, span := tracer.Start(ctx, "tls-span")
	span.End()
	logger := global.GetLoggerProvider().Logger("wire-tls-logger")
	var rec log.Record
	logger.Emit(ctx, rec)

	ctxFlush, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if tp, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider); ok {
		if err := tp.ForceFlush(ctxFlush); err != nil {
			return fmt.Errorf("trace force flush: %w", err)
		}
	}
	if lp, ok := global.GetLoggerProvider().(*sdklog.LoggerProvider); ok {
		if err := lp.ForceFlush(ctxFlush); err != nil {
			return fmt.Errorf("log force flush: %w", err)
		}
	}
	return nil
}

func TestWire_TLS_PrivateCA_ValidCA(t *testing.T) {
	dir := t.TempDir()
	caPEM, caKey, caCert := wireSelfSignedCA(t)
	caPath := wireWritePEM(t, dir, "ca.pem", caPEM)

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caPEM)
	srvCert := wireServerCert(t, caKey, caCert, []string{"127.0.0.1"})
	srv, traceReceived, logReceived := wireNewTLSTestServer(t, srvCert, false, nil)

	t.Setenv("OTEL_EXPORTER_OTLP_CERTIFICATE", caPath)

	err := wireInitAndFlushBothSignals(t, srv.URL, "valid-token-123=")
	if err != nil {
		t.Fatalf("Expected success with valid private CA, got: %v", err)
	}
	if atomic.LoadInt32(traceReceived) < 1 {
		t.Errorf("Expected >= 1 trace requests over TLS wire, got %d", atomic.LoadInt32(traceReceived))
	}
	if atomic.LoadInt32(logReceived) < 1 {
		t.Errorf("Expected >= 1 log requests over TLS wire, got %d", atomic.LoadInt32(logReceived))
	}
}

func TestWire_TLS_WrongCA(t *testing.T) {
	dir := t.TempDir()
	caPEM1, caKey1, caCert1 := wireSelfSignedCA(t)
	caPEM2, _, _ := wireSelfSignedCA(t)
	wrongCAPath := wireWritePEM(t, dir, "wrong-ca.pem", caPEM2)

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caPEM1)
	srvCert := wireServerCert(t, caKey1, caCert1, []string{"127.0.0.1"})
	srv, traceReceived, logReceived := wireNewTLSTestServer(t, srvCert, false, nil)

	t.Setenv("OTEL_EXPORTER_OTLP_CERTIFICATE", wrongCAPath)

	origTracer := otel.GetTracerProvider()
	origLogger := global.GetLoggerProvider()
	defer func() {
		otel.SetTracerProvider(origTracer)
		global.SetLoggerProvider(origLogger)
	}()

	cfg := Config{Enabled: true, OTLPEndpoint: srv.URL, IngestToken: "valid-token-123=", ServiceName: "tls-wrongca"}
	ctx := context.Background()
	shutdown, err := InitProvider(ctx, cfg)
	if err != nil {
		return
	}
	defer func() {
		ctxShut, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = shutdown(ctxShut)
	}()

	tracer := otel.Tracer("tls-wrongca")
	_, span := tracer.Start(ctx, "span")
	span.End()
	ctxFlush, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var flushErr error
	if tp, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider); ok {
		flushErr = tp.ForceFlush(ctxFlush)
	}
	if lp, ok := global.GetLoggerProvider().(*sdklog.LoggerProvider); ok {
		if e := lp.ForceFlush(ctxFlush); e != nil && flushErr == nil {
			flushErr = e
		}
	}
	if flushErr == nil {
		t.Error("Expected TLS error with wrong CA, but export succeeded")
	}
	if atomic.LoadInt32(traceReceived) != 0 || atomic.LoadInt32(logReceived) != 0 {
		t.Errorf("Expected 0 requests on TLS wire with wrong CA, got traces=%d logs=%d",
			atomic.LoadInt32(traceReceived), atomic.LoadInt32(logReceived))
	}
}

func TestWire_TLS_ClientRequired_Missing(t *testing.T) {
	dir := t.TempDir()
	caPEM, caKey, caCert := wireSelfSignedCA(t)
	caPath := wireWritePEM(t, dir, "ca.pem", caPEM)
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caPEM)
	srvCert := wireServerCert(t, caKey, caCert, []string{"127.0.0.1"})
	srv, traceReceived, logReceived := wireNewTLSTestServer(t, srvCert, true, caPool)

	t.Setenv("OTEL_EXPORTER_OTLP_CERTIFICATE", caPath)

	origTracer := otel.GetTracerProvider()
	origLogger := global.GetLoggerProvider()
	defer func() {
		otel.SetTracerProvider(origTracer)
		global.SetLoggerProvider(origLogger)
	}()

	cfg := Config{Enabled: true, OTLPEndpoint: srv.URL, IngestToken: "valid-token-123=", ServiceName: "mtls-missing"}
	ctx := context.Background()
	shutdown, err := InitProvider(ctx, cfg)
	if err != nil {
		return
	}
	defer func() {
		ctxShut, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = shutdown(ctxShut)
	}()

	tracer := otel.Tracer("mtls-missing")
	_, span := tracer.Start(ctx, "span")
	span.End()
	logger := global.GetLoggerProvider().Logger("logger")
	var rec log.Record
	logger.Emit(ctx, rec)
	ctxFlush, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var flushErr error
	if tp, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider); ok {
		flushErr = tp.ForceFlush(ctxFlush)
	}
	if lp, ok := global.GetLoggerProvider().(*sdklog.LoggerProvider); ok {
		if e := lp.ForceFlush(ctxFlush); e != nil && flushErr == nil {
			flushErr = e
		}
	}
	if flushErr == nil {
		t.Error("Expected mTLS error when client cert missing, but export succeeded")
	}
	if atomic.LoadInt32(traceReceived) != 0 || atomic.LoadInt32(logReceived) != 0 {
		t.Errorf("Expected 0 requests on mTLS wire when client cert missing, got traces=%d logs=%d",
			atomic.LoadInt32(traceReceived), atomic.LoadInt32(logReceived))
	}
}

func TestWire_TLS_ClientRequired_Valid(t *testing.T) {
	dir := t.TempDir()
	caPEM, caKey, caCert := wireSelfSignedCA(t)
	caPath := wireWritePEM(t, dir, "ca.pem", caPEM)
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caPEM)
	srvCert := wireServerCert(t, caKey, caCert, []string{"127.0.0.1"})
	srv, traceReceived, logReceived := wireNewTLSTestServer(t, srvCert, true, caPool)

	cliCert := wireClientCert(t, caKey, caCert)
	kd, _ := x509.MarshalECPrivateKey(cliCert.PrivateKey.(*ecdsa.PrivateKey))
	cliCertPath := wireWritePEM(t, dir, "client.crt",
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cliCert.Certificate[0]}))
	cliKeyPath := wireWritePEM(t, dir, "client.key",
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kd}))

	t.Setenv("OTEL_EXPORTER_OTLP_CERTIFICATE", caPath)
	t.Setenv("OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE", cliCertPath)
	t.Setenv("OTEL_EXPORTER_OTLP_CLIENT_KEY", cliKeyPath)

	err := wireInitAndFlushBothSignals(t, srv.URL, "valid-token-123=")
	if err != nil {
		t.Fatalf("Expected mTLS success with valid client cert, got: %v", err)
	}
	if atomic.LoadInt32(traceReceived) < 1 {
		t.Errorf("Expected >= 1 trace requests over mTLS wire, got %d", atomic.LoadInt32(traceReceived))
	}
	if atomic.LoadInt32(logReceived) < 1 {
		t.Errorf("Expected >= 1 log requests over mTLS wire, got %d", atomic.LoadInt32(logReceived))
	}
}

func TestWire_TLS_SignalPrecedence(t *testing.T) {
	dir := t.TempDir()
	caPEM, caKey, caCert := wireSelfSignedCA(t)
	caPath := wireWritePEM(t, dir, "ca.pem", caPEM)

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caPEM)
	srvCert := wireServerCert(t, caKey, caCert, []string{"127.0.0.1"})
	srv, traceReceived, logReceived := wireNewTLSTestServer(t, srvCert, false, nil)

	// Set wrong CA as common CA, but valid CA for signal-specific CAs.
	caPEM2, _, _ := wireSelfSignedCA(t)
	wrongCAPath := wireWritePEM(t, dir, "wrong-ca.pem", caPEM2)

	t.Setenv("OTEL_EXPORTER_OTLP_CERTIFICATE", wrongCAPath)
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE", caPath)
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_CERTIFICATE", caPath)

	err := wireInitAndFlushBothSignals(t, srv.URL, "valid-token-123=")
	if err != nil {
		t.Fatalf("Signal-specific env vars should override common CA, got: %v", err)
	}
	if atomic.LoadInt32(traceReceived) < 1 || atomic.LoadInt32(logReceived) < 1 {
		t.Errorf("Expected requests on both signals, got traces=%d logs=%d",
			atomic.LoadInt32(traceReceived), atomic.LoadInt32(logReceived))
	}
}

func TestWire_TLS_Disabled_NoSecretRead(t *testing.T) {
	t.Setenv("RASK_INGEST_TOKEN", "")
	t.Setenv("RASK_INGEST_TOKEN_FILE", "/nonexistent/secret")
	t.Setenv("OTEL_EXPORTER_OTLP_CERTIFICATE", "/nonexistent/ca.pem")
	t.Setenv("OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE", "/nonexistent/client.crt")
	t.Setenv("OTEL_EXPORTER_OTLP_CLIENT_KEY", "/nonexistent/client.key")

	cfg := Config{Enabled: false}
	ctx := context.Background()
	shutdown, err := InitProvider(ctx, cfg)
	if err != nil {
		t.Fatalf("Disabled mode must not error on invalid TLS paths: %v", err)
	}
	_ = shutdown(ctx)
}
