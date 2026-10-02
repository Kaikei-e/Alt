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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	plogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	ptrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestProvider_InitAndExport(t *testing.T) {
	originalTracer := otel.GetTracerProvider()
	originalLogger := global.GetLoggerProvider()
	defer func() {
		otel.SetTracerProvider(originalTracer)
		global.SetLoggerProvider(originalLogger)
	}()

	var traceReqCount, logReqCount int32
	var traceAuth, logAuth string
	var mu sync.Mutex
	var lastTraceBody, lastLogBody []byte

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("ReadAll r.Body failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		mu.Lock()
		defer mu.Unlock()

		switch r.URL.Path {
		case "/v1/traces":
			atomic.AddInt32(&traceReqCount, 1)
			traceAuth = r.Header.Get("Authorization")
			lastTraceBody = body
		case "/v1/logs":
			atomic.AddInt32(&logReqCount, 1)
			logAuth = r.Header.Get("Authorization")
			lastLogBody = body
		default:
			t.Errorf("Unexpected path: %s", r.URL.Path)
		}

		if r.Header.Get("Content-Type") != "application/x-protobuf" {
			t.Errorf("Expected protobuf content type, got %s", r.Header.Get("Content-Type"))
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	tokenContent := "valid-token-123="
	tmpFile, err := os.CreateTemp("", "token-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			t.Errorf("Failed to remove temp file: %v", err)
		}
	}()
	if _, err := tmpFile.Write([]byte(tokenContent)); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	t.Setenv("RASK_INGEST_TOKEN_FILE", tmpFile.Name())
	t.Setenv("RASK_INGEST_TOKEN", tokenContent)
	t.Setenv("OTEL_TRACE_SAMPLE_RATIO", "1.0")

	cfg := ConfigFromEnv()
	cfg.Enabled = true
	cfg.OTLPEndpoint = ts.URL
	cfg.IngestToken = tokenContent

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

	tracer := otel.Tracer("test-tracer")
	_, span := tracer.Start(ctx, "test-span")
	span.End()

	logger := global.GetLoggerProvider().Logger("test-logger")
	var rec log.Record
	logger.Emit(ctx, rec)

	ctxFlush, flushCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer flushCancel()

	if tp, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider); ok {
		if err := tp.ForceFlush(ctxFlush); err != nil {
			t.Errorf("tp.ForceFlush failed: %v", err)
		}
	} else {
		t.Errorf("TracerProvider is not sdktrace.TracerProvider")
	}

	if lp, ok := global.GetLoggerProvider().(*sdklog.LoggerProvider); ok {
		if err := lp.ForceFlush(ctxFlush); err != nil {
			t.Errorf("lp.ForceFlush failed: %v", err)
		}
	} else {
		t.Errorf("LoggerProvider is not sdklog.LoggerProvider")
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if traceReqCount == 0 {
		t.Errorf("Expected trace requests, got 0")
	}
	if logReqCount == 0 {
		t.Errorf("Expected log requests, got 0")
	}

	if traceAuth != "Bearer "+tokenContent {
		t.Errorf("Trace Auth mismatch: got %s", traceAuth)
	}
	if logAuth != "Bearer "+tokenContent {
		t.Errorf("Log Auth mismatch: got %s", logAuth)
	}

	var traceReq ptrace.ExportTraceServiceRequest
	if err := proto.Unmarshal(lastTraceBody, &traceReq); err != nil {
		t.Fatalf("Failed to decode trace req: %v", err)
	}
	if len(traceReq.ResourceSpans) == 0 {
		t.Errorf("Expected non-empty ResourceSpans")
	} else if len(traceReq.ResourceSpans[0].ScopeSpans) == 0 {
		t.Errorf("Expected non-empty ScopeSpans")
	} else if len(traceReq.ResourceSpans[0].ScopeSpans[0].Spans) == 0 {
		t.Errorf("Expected non-empty Spans")
	}

	var logReq plogs.ExportLogsServiceRequest
	if err := proto.Unmarshal(lastLogBody, &logReq); err != nil {
		t.Fatalf("Failed to decode log req: %v", err)
	}
	if len(logReq.ResourceLogs) == 0 {
		t.Errorf("Expected non-empty ResourceLogs")
	} else if len(logReq.ResourceLogs[0].ScopeLogs) == 0 {
		t.Errorf("Expected non-empty ScopeLogs")
	} else if len(logReq.ResourceLogs[0].ScopeLogs[0].LogRecords) == 0 {
		t.Errorf("Expected non-empty LogRecords")
	}
}

// TestProvider_RedirectFail verifies that the production HTTP client does NOT
// follow redirects. Uses separate origin + destination servers with atomic
// counters: origin ≥ 1, destination == 0.
func TestProvider_RedirectFail(t *testing.T) {
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
		http.Redirect(w, r, dest.URL+r.URL.Path, http.StatusFound)
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

	tracer := otel.Tracer("redirect-test-tracer")
	_, span := tracer.Start(ctx, "redirect-test-span")
	span.End()

	logger := global.GetLoggerProvider().Logger("redirect-test-logger")
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

func TestProvider_InvalidToken(t *testing.T) {
	cfg := ConfigFromEnv()
	cfg.Enabled = true
	cfg.OTLPEndpoint = "http://localhost:4318"
	cfg.IngestToken = "invalid token!@#" // invalid format

	ctx := context.Background()
	_, err := InitProvider(ctx, cfg)
	if err == nil {
		t.Fatalf("Expected InitProvider to fail with invalid token")
	}
}

// TestProvider_Disabled verifies disabled mode returns a no-op shutdown
// without reading any file or secret path.
func TestProvider_Disabled(t *testing.T) {
	cfg := Config{
		Enabled:         false,
		IngestTokenFile: "/nonexistent/secret/path",
	}
	ctx := context.Background()
	shutdown, err := InitProvider(ctx, cfg)
	if err != nil {
		t.Fatalf("Disabled InitProvider should not error, got: %v", err)
	}
	if shutdown == nil {
		t.Fatal("Disabled InitProvider must return non-nil shutdown")
	}
	_ = shutdown(ctx)
}

// TestProvider_TokenFileLoader_Failures tests all meaningful file-loader error
// paths for IngestTokenFile. Errors must NOT reflect file contents.
func TestProvider_TokenFileLoader_Failures(t *testing.T) {
	ctx := context.Background()

	t.Run("missing_file", func(t *testing.T) {
		cfg := Config{
			Enabled:         true,
			OTLPEndpoint:    "http://localhost:4318",
			IngestToken:     "",
			IngestTokenFile: "/nonexistent/token/file",
		}
		_, err := InitProvider(ctx, cfg)
		if err == nil {
			t.Fatal("expected error for missing token file")
		}
	})

	t.Run("unreadable_file", func(t *testing.T) {
		unreadableDir := t.TempDir()
		cfg := Config{
			Enabled:         true,
			OTLPEndpoint:    "http://localhost:4318",
			IngestToken:     "",
			IngestTokenFile: unreadableDir,
		}
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
		defer func() {
			if err := os.Remove(f.Name()); err != nil {
				t.Errorf("cannot remove temp file: %v", err)
			}
		}()
		if _, err := f.WriteString("valid-file-token-123=\n"); err != nil {
			t.Fatalf("cannot write temp file: %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("cannot close temp file: %v", err)
		}

		cfg := Config{
			Enabled:         true,
			OTLPEndpoint:    "http://localhost:4318",
			IngestToken:     "",
			IngestTokenFile: f.Name(),
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
			defer func() {
				if err := os.Remove(f.Name()); err != nil {
					t.Errorf("cannot remove temp file: %v", err)
				}
			}()
			if _, err := f.WriteString(tc.content); err != nil {
				t.Fatalf("cannot write temp file: %v", err)
			}
			if err := f.Close(); err != nil {
				t.Fatalf("cannot close temp file: %v", err)
			}

			cfg := Config{
				Enabled:         true,
				OTLPEndpoint:    "http://localhost:4318",
				IngestToken:     "",
				IngestTokenFile: f.Name(),
			}
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

func selfSignedCA(t *testing.T) (caPEM []byte, caKey *ecdsa.PrivateKey, caCert *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		IsCA:                  true,
		BasicConstraintsValid: true,
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

func serverCert(t *testing.T, caKey *ecdsa.PrivateKey, caCert *x509.Certificate, hosts []string) tls.Certificate {
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

func clientCert(t *testing.T, caKey *ecdsa.PrivateKey, caCert *x509.Certificate) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "test-client"},
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
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDer})
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("parse client key pair: %v", err)
	}
	return tlsCert
}

func writePEM(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func newTLSTestServer(t *testing.T, srvCert tls.Certificate, requireClientAuth bool, caPool *x509.CertPool) (*httptest.Server, *int32, *int32) {
	t.Helper()
	var traceReceived, logReceived int32
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/traces", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&traceReceived, 1)
		if _, err := io.ReadAll(r.Body); err != nil {
			t.Errorf("read trace body failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/v1/logs", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&logReceived, 1)
		if _, err := io.ReadAll(r.Body); err != nil {
			t.Errorf("read log body failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
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

func initAndFlushBothSignals(t *testing.T, srvURL, token string) error {
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
		ServiceName:  "tls-test",
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

	tracer := otel.Tracer("tls-test-tracer")
	_, span := tracer.Start(ctx, "tls-span")
	span.End()
	logger := global.GetLoggerProvider().Logger("tls-logger")
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

func TestProvider_TLS_PrivateCA_ValidCA(t *testing.T) {
	dir := t.TempDir()
	caPEM, caKey, caCert := selfSignedCA(t)
	caPath := writePEM(t, dir, "ca.pem", caPEM)

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caPEM)
	srvCert := serverCert(t, caKey, caCert, []string{"127.0.0.1"})
	srv, traceReceived, logReceived := newTLSTestServer(t, srvCert, false, nil)

	t.Setenv("OTEL_EXPORTER_OTLP_CERTIFICATE", caPath)

	err := initAndFlushBothSignals(t, srv.URL, "valid-token-123=")
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

func TestProvider_TLS_WrongCA(t *testing.T) {
	dir := t.TempDir()
	caPEM1, caKey1, caCert1 := selfSignedCA(t)
	caPEM2, _, _ := selfSignedCA(t)
	wrongCAPath := writePEM(t, dir, "wrong-ca.pem", caPEM2)

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caPEM1)
	srvCert := serverCert(t, caKey1, caCert1, []string{"127.0.0.1"})
	srv, traceReceived, logReceived := newTLSTestServer(t, srvCert, false, nil)

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

func TestProvider_TLS_ClientRequired_Missing(t *testing.T) {
	dir := t.TempDir()
	caPEM, caKey, caCert := selfSignedCA(t)
	caPath := writePEM(t, dir, "ca.pem", caPEM)
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caPEM)
	srvCert := serverCert(t, caKey, caCert, []string{"127.0.0.1"})
	srv, traceReceived, logReceived := newTLSTestServer(t, srvCert, true, caPool)

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

func TestProvider_TLS_ClientRequired_Valid(t *testing.T) {
	dir := t.TempDir()
	caPEM, caKey, caCert := selfSignedCA(t)
	caPath := writePEM(t, dir, "ca.pem", caPEM)
	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caPEM)
	srvCert := serverCert(t, caKey, caCert, []string{"127.0.0.1"})
	srv, traceReceived, logReceived := newTLSTestServer(t, srvCert, true, caPool)

	cliCert := clientCert(t, caKey, caCert)
	kd, _ := x509.MarshalECPrivateKey(cliCert.PrivateKey.(*ecdsa.PrivateKey))
	cliCertPath := writePEM(t, dir, "client.crt",
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cliCert.Certificate[0]}))
	cliKeyPath := writePEM(t, dir, "client.key",
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kd}))

	t.Setenv("OTEL_EXPORTER_OTLP_CERTIFICATE", caPath)
	t.Setenv("OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE", cliCertPath)
	t.Setenv("OTEL_EXPORTER_OTLP_CLIENT_KEY", cliKeyPath)

	err := initAndFlushBothSignals(t, srv.URL, "valid-token-123=")
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

func TestProvider_TLS_SignalPrecedence(t *testing.T) {
	dir := t.TempDir()
	caPEM, caKey, caCert := selfSignedCA(t)
	caPath := writePEM(t, dir, "ca.pem", caPEM)

	caPool := x509.NewCertPool()
	caPool.AppendCertsFromPEM(caPEM)
	srvCert := serverCert(t, caKey, caCert, []string{"127.0.0.1"})
	srv, traceReceived, logReceived := newTLSTestServer(t, srvCert, false, nil)

	// Set wrong CA as common CA, but valid CA for signal-specific CAs.
	caPEM2, _, _ := selfSignedCA(t)
	wrongCAPath := writePEM(t, dir, "wrong-ca.pem", caPEM2)

	t.Setenv("OTEL_EXPORTER_OTLP_CERTIFICATE", wrongCAPath)
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE", caPath)
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_CERTIFICATE", caPath)

	err := initAndFlushBothSignals(t, srv.URL, "valid-token-123=")
	if err != nil {
		t.Fatalf("Signal-specific env vars should override common CA, got: %v", err)
	}
	if atomic.LoadInt32(traceReceived) < 1 || atomic.LoadInt32(logReceived) < 1 {
		t.Errorf("Expected requests on both signals, got traces=%d logs=%d",
			atomic.LoadInt32(traceReceived), atomic.LoadInt32(logReceived))
	}
}

func TestProvider_TLS_Disabled_NoSecretRead(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_CERTIFICATE", "/nonexistent/ca.pem")
	t.Setenv("OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE", "/nonexistent/client.crt")
	t.Setenv("OTEL_EXPORTER_OTLP_CLIENT_KEY", "/nonexistent/client.key")

	cfg := Config{Enabled: false, IngestTokenFile: "/nonexistent/secret"}
	ctx := context.Background()
	shutdown, err := InitProvider(ctx, cfg)
	if err != nil {
		t.Fatalf("Disabled mode must not error on invalid TLS paths: %v", err)
	}
	_ = shutdown(ctx)
}
