package bootstrap

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pre-processor/config"
	"pre-processor/metrics"
	"pre-processor/service"
)

// metricsTestConfig mirrors the shipped defaults: the dedicated listener on
// :9201, whose Prometheus exposition lives at /metrics/prometheus.
func metricsTestConfig() config.MetricsConfig {
	return config.MetricsConfig{
		Enabled:           true,
		Port:              9201,
		Path:              "/metrics",
		UpdateInterval:    10 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

// TestBuildMetricsCollector_RegistersTheRelayGauges pins the placement: the
// notification-outbox gauges are served by the dedicated metrics listener, so
// the composition root has to register them there. A collector built without
// them is a scrape target that reports on everything except the relay.
func TestBuildMetricsCollector_RegistersTheRelayGauges(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	relayMetrics := metrics.NewOutboxRelayMetrics()
	relayMetrics.ObserveTick(0, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))

	collector, err := buildMetricsCollector(metricsTestConfig(), relayMetrics, log)
	require.NoError(t, err)

	exposition := collector.ExportPrometheus()
	require.Contains(t, exposition, "notification_outbox_oldest_pending_age_seconds 0")
	require.Contains(t, exposition, "notification_outbox_last_tick_timestamp_seconds")
}

func TestBuildMetricsCollector_RefusesUnwiredRelayMetrics(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	_, err := buildMetricsCollector(metricsTestConfig(), nil, log)
	require.Error(t, err)
}

// TestBuildRedisConsumer_DisabledIsNotAnError verifies the deliberate
// CONSUMER_ENABLED=false opt-out path succeeds (no construction/start attempted
// against a real Redis) — this is the "explicit config flag" no-op, not a
// silent failure.
func TestBuildRedisConsumer_DisabledIsNotAnError(t *testing.T) {
	t.Setenv("CONSUMER_ENABLED", "false")
	t.Setenv("REDIS_AUTH", "disabled")

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, err := buildRedisConsumer(context.Background(), nil, nil, nil, log)
	if err != nil {
		t.Fatalf("buildRedisConsumer with CONSUMER_ENABLED=false returned error: %v", err)
	}
	if c == nil {
		t.Fatal("buildRedisConsumer returned nil consumer for the disabled no-op path")
	}
}

// TestBuildRedisConsumer_ConstructionFailurePropagatesError reproduces the
// HIGH finding: a malformed REDIS_STREAMS_URL previously caused
// buildRedisConsumer to log-and-swallow the error and return nil, which the
// caller only logged — the event-driven summarization consumer died silently.
// It must now surface the error so BuildDependencies can fail startup.
func TestBuildRedisConsumer_ConstructionFailurePropagatesError(t *testing.T) {
	t.Setenv("CONSUMER_ENABLED", "true")
	t.Setenv("REDIS_AUTH", "disabled")
	t.Setenv("REDIS_STREAMS_URL", "not-a-valid-redis-url")

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, err := buildRedisConsumer(context.Background(), nil, nil, nil, log)
	if err == nil {
		t.Fatal("buildRedisConsumer with a malformed REDIS_STREAMS_URL returned nil error, want propagated construction error")
	}
	if c != nil {
		t.Fatal("buildRedisConsumer returned a non-nil consumer alongside an error")
	}
}

// TestBuildRedisConsumer_StartFailurePropagatesError reproduces the second
// half of the HIGH finding: when the consumer is enabled but Redis is
// unreachable, Start() previously failed silently (logged only) while the
// caller kept running with a half-initialized consumer.
func TestBuildRedisConsumer_StartFailurePropagatesError(t *testing.T) {
	t.Setenv("CONSUMER_ENABLED", "true")
	t.Setenv("REDIS_AUTH", "disabled")
	// Valid URL syntax but nothing listens here — Start()'s ensureConsumerGroup
	// call must fail against an unreachable broker.
	t.Setenv("REDIS_STREAMS_URL", "redis://127.0.0.1:1")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	c, err := buildRedisConsumer(ctx, nil, nil, nil, log)
	if err == nil {
		t.Fatal("buildRedisConsumer against an unreachable Redis returned nil error, want propagated start error")
	}
	if c != nil {
		t.Fatal("buildRedisConsumer returned a non-nil consumer alongside a start error")
	}
}

// writeThrowawayPKI generates a self-signed CA and a leaf cert/key pair,
// writes them as PEM files under dir, and returns their paths. Validity
// dates are fixed rather than wall-clock-relative — this is a throwaway
// test fixture, not a business timestamp, and it never needs to expire.
func writeThrowawayPKI(t *testing.T, dir string) (certPath, keyPath, caPath string) {
	t.Helper()

	notBefore := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	notAfter := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "pre-processor-test-ca"},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
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
		Subject:      pkix.Name{CommonName: "pre-processor"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caTmpl, &leafKey.PublicKey, caKey)
	require.NoError(t, err)

	certPath = filepath.Join(dir, "leaf-cert.pem")
	keyPath = filepath.Join(dir, "leaf-key.pem")
	caPath = filepath.Join(dir, "ca-bundle.pem")

	writeThrowawayPEM(t, certPath, "CERTIFICATE", leafDER)
	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	require.NoError(t, err)
	writeThrowawayPEM(t, keyPath, "EC PRIVATE KEY", leafKeyDER)
	writeThrowawayPEM(t, caPath, "CERTIFICATE", caDER)

	return certPath, keyPath, caPath
}

func writeThrowawayPEM(t *testing.T, path, blockType string, der []byte) {
	t.Helper()
	f, err := os.Create(path) // #nosec G304 -- test-only path under t.TempDir()
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()
	require.NoError(t, pem.Encode(f, &pem.Block{Type: blockType, Bytes: der}))
}

// TestBuildBackendHTTPClient_MTLSEnforced_HasTimeouts guards against the
// minor finding from the DataHub client wiring fix: switching
// repository.ExternalAPIRepository onto this shared client silently dropped
// GetSystemUserID's client-side timeouts, and GetSystemUserID runs on a
// JobRunner ticker loop with no per-run context deadline (see the doc
// comment on the mTLS branch of buildBackendHTTPClient). A stalled peer
// under the old zero-value Client/Transport would hang forever and
// permanently wedge that job's ticker loop.
func TestBuildBackendHTTPClient_MTLSEnforced_HasTimeouts(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := writeThrowawayPKI(t, dir)

	t.Setenv("MTLS_ENFORCE", "true")
	t.Setenv("MTLS_CERT_FILE", certPath)
	t.Setenv("MTLS_KEY_FILE", keyPath)
	t.Setenv("MTLS_CA_FILE", caPath)

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	client, err := buildBackendHTTPClient(log)
	require.NoError(t, err)
	require.NotNil(t, client)

	if client.Timeout <= 0 {
		t.Error("mTLS-enforced backend client has no overall Client.Timeout: a stalled peer would hang GetSystemUserID's job-loop caller forever")
	}

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok, "mTLS-enforced backend client must use *http.Transport to carry the TLS config and phase timeouts")

	if transport.DialContext == nil {
		t.Error("mTLS-enforced backend client's Transport has no DialContext dial timeout")
	}
	if transport.TLSHandshakeTimeout <= 0 {
		t.Error("mTLS-enforced backend client's Transport has no TLSHandshakeTimeout")
	}
	if transport.ResponseHeaderTimeout <= 0 {
		t.Error("mTLS-enforced backend client's Transport has no ResponseHeaderTimeout")
	}
	if transport.TLSClientConfig == nil {
		t.Error("mTLS-enforced backend client's Transport lost its TLS client config")
	}
}

// writeThrowawayPKIWithServer generates a self-signed CA, a pre-processor client leaf,
// and a server certificate with the specified SAN, all signed by the CA.
func writeThrowawayPKIWithServer(t *testing.T, dir string, san string) (certPath, keyPath, caPath string, serverCert tls.Certificate, caPool *x509.CertPool) {
	t.Helper()

	notBefore := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	notAfter := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "pre-processor-test-ca"},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
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
		Subject:      pkix.Name{CommonName: "pre-processor"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caTmpl, &leafKey.PublicKey, caKey)
	require.NoError(t, err)

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serverTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: san},
		DNSNames:     []string{san},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTmpl, caTmpl, &serverKey.PublicKey, caKey)
	require.NoError(t, err)

	certPath = filepath.Join(dir, "leaf-cert.pem")
	keyPath = filepath.Join(dir, "leaf-key.pem")
	caPath = filepath.Join(dir, "ca-bundle.pem")

	writeThrowawayPEM(t, certPath, "CERTIFICATE", leafDER)
	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	require.NoError(t, err)
	writeThrowawayPEM(t, keyPath, "EC PRIVATE KEY", leafKeyDER)
	writeThrowawayPEM(t, caPath, "CERTIFICATE", caDER)

	caParsed, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)
	caPool = x509.NewCertPool()
	caPool.AddCert(caParsed)

	serverCert = tls.Certificate{
		Certificate: [][]byte{serverDER},
		PrivateKey:  serverKey,
	}

	return certPath, keyPath, caPath, serverCert, caPool
}

func TestProductionTLSFactoryWire_PositiveHandshake(t *testing.T) {
	dir := t.TempDir()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// 1. news-creator mTLS positive test
	certPath, keyPath, caPath, newsServerCert, caPool := writeThrowawayPKIWithServer(t, dir, "news-creator")

	newsServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			http.Error(w, "missing peer client certificate", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	newsServer.TLS = &tls.Config{
		Certificates: []tls.Certificate{newsServerCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
	}
	newsServer.StartTLS()
	defer newsServer.Close()

	t.Setenv("MTLS_ENFORCE", "true")
	t.Setenv("MTLS_CERT_FILE", certPath)
	t.Setenv("MTLS_KEY_FILE", keyPath)
	t.Setenv("MTLS_CA_FILE", caPath)
	t.Setenv("NEWS_CREATOR_MTLS_SERVER_NAME", "news-creator")

	cfg := &config.Config{
		NewsCreator: config.NewsCreatorConfig{
			Host:    newsServer.URL,
			Timeout: 600 * time.Second,
		},
	}

	newsClient, err := buildNewsHTTPClient(cfg, log)
	require.NoError(t, err)
	require.NotNil(t, newsClient)
	assert.Equal(t, 600*time.Second, newsClient.Timeout)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, newsServer.URL, nil)
	require.NoError(t, err)
	resp, err := newsClient.Do(req)
	require.NoError(t, err, "news client mTLS handshake and request must succeed")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	// 2. alt-data-hub backend client mTLS positive test
	dirBackend := t.TempDir()
	certPathB, keyPathB, caPathB, backendServerCert, caPoolB := writeThrowawayPKIWithServer(t, dirBackend, "alt-data-hub")

	backendServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			http.Error(w, "missing peer client certificate", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"backend":"ok"}`))
	}))
	backendServer.TLS = &tls.Config{
		Certificates: []tls.Certificate{backendServerCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPoolB,
	}
	backendServer.StartTLS()
	defer backendServer.Close()

	t.Setenv("MTLS_CERT_FILE", certPathB)
	t.Setenv("MTLS_KEY_FILE", keyPathB)
	t.Setenv("MTLS_CA_FILE", caPathB)
	t.Setenv("BACKEND_MTLS_SERVER_NAME", "alt-data-hub")

	backendClient, err := buildBackendHTTPClient(log)
	require.NoError(t, err)
	require.NotNil(t, backendClient)
	assert.Equal(t, 30*time.Second, backendClient.Timeout)

	reqB, err := http.NewRequestWithContext(ctx, http.MethodGet, backendServer.URL, nil)
	require.NoError(t, err)
	respB, err := backendClient.Do(reqB)
	require.NoError(t, err, "backend client mTLS handshake and request must succeed")
	assert.Equal(t, http.StatusOK, respB.StatusCode)
	_ = respB.Body.Close()
}

func TestProductionTLSFactoryWire_NegativeCases(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	t.Run("wrong CA causes handshake failure", func(t *testing.T) {
		dir1 := t.TempDir()
		dir2 := t.TempDir()

		certPath1, keyPath1, caPath1, _, _ := writeThrowawayPKIWithServer(t, dir1, "news-creator")
		_, _, _, serverCert2, caPool2 := writeThrowawayPKIWithServer(t, dir2, "news-creator")

		server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		server.TLS = &tls.Config{
			Certificates: []tls.Certificate{serverCert2},
			ClientAuth:   tls.RequireAndVerifyClientCert,
			ClientCAs:    caPool2,
		}
		server.StartTLS()
		defer server.Close()

		t.Setenv("MTLS_ENFORCE", "true")
		t.Setenv("MTLS_CERT_FILE", certPath1)
		t.Setenv("MTLS_KEY_FILE", keyPath1)
		t.Setenv("MTLS_CA_FILE", caPath1) // Untrusted CA for server2
		t.Setenv("NEWS_CREATOR_MTLS_SERVER_NAME", "news-creator")

		cfg := &config.Config{
			NewsCreator: config.NewsCreatorConfig{
				Host:    server.URL,
				Timeout: 5 * time.Second,
			},
		}

		client, err := buildNewsHTTPClient(cfg, log)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		_, err = client.Do(req)
		require.Error(t, err, "request against server with untrusted CA must fail TLS handshake")
	})

	t.Run("wrong hostname causes SAN mismatch failure", func(t *testing.T) {
		dir := t.TempDir()
		certPath, keyPath, caPath, serverCert, caPool := writeThrowawayPKIWithServer(t, dir, "unrelated-hostname")

		server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		server.TLS = &tls.Config{
			Certificates: []tls.Certificate{serverCert},
			ClientAuth:   tls.RequireAndVerifyClientCert,
			ClientCAs:    caPool,
		}
		server.StartTLS()
		defer server.Close()

		t.Setenv("MTLS_ENFORCE", "true")
		t.Setenv("MTLS_CERT_FILE", certPath)
		t.Setenv("MTLS_KEY_FILE", keyPath)
		t.Setenv("MTLS_CA_FILE", caPath)
		t.Setenv("NEWS_CREATOR_MTLS_SERVER_NAME", "news-creator")

		cfg := &config.Config{
			NewsCreator: config.NewsCreatorConfig{
				Host:    server.URL,
				Timeout: 5 * time.Second,
			},
		}

		client, err := buildNewsHTTPClient(cfg, log)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		_, err = client.Do(req)
		require.Error(t, err, "request with mismatched SAN hostname must fail TLS handshake")
	})

	t.Run("missing certificate file fails client startup", func(t *testing.T) {
		dir := t.TempDir()
		_, keyPath, caPath := writeThrowawayPKI(t, dir)

		t.Setenv("MTLS_ENFORCE", "true")
		t.Setenv("MTLS_CERT_FILE", filepath.Join(dir, "missing-cert.pem"))
		t.Setenv("MTLS_KEY_FILE", keyPath)
		t.Setenv("MTLS_CA_FILE", caPath)

		cfg := &config.Config{
			NewsCreator: config.NewsCreatorConfig{
				Timeout: 5 * time.Second,
			},
		}

		_, err := buildNewsHTTPClient(cfg, log)
		require.Error(t, err, "missing cert file must fail buildNewsHTTPClient")

		_, err = buildBackendHTTPClient(log)
		require.Error(t, err, "missing cert file must fail buildBackendHTTPClient")
	})
}

func TestProductionTLSFactoryWire_HealthCheckerService(t *testing.T) {
	dir := t.TempDir()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	certPath, keyPath, caPath, serverCert, caPool := writeThrowawayPKIWithServer(t, dir, "news-creator")

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health/deep" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"pass"}`))
			return
		}
		http.NotFound(w, r)
	}))
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
	}
	server.StartTLS()
	defer server.Close()

	t.Setenv("MTLS_ENFORCE", "true")
	t.Setenv("MTLS_CERT_FILE", certPath)
	t.Setenv("MTLS_KEY_FILE", keyPath)
	t.Setenv("MTLS_CA_FILE", caPath)
	t.Setenv("NEWS_CREATOR_MTLS_SERVER_NAME", "news-creator")

	cfg := &config.Config{
		NewsCreator: config.NewsCreatorConfig{
			Host:    server.URL,
			Timeout: 5 * time.Second,
		},
	}

	newsClient, err := buildNewsHTTPClient(cfg, log)
	require.NoError(t, err)

	healthChecker := service.NewHealthCheckerServiceWithClient(newsClient, server.URL, log)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = healthChecker.CheckNewsCreatorHealth(ctx)
	require.NoError(t, err, "health check must succeed with valid TLS configuration")

	// Negative case: health checker against bad CA server must report error
	dirBad := t.TempDir()
	_, _, _, serverCertBad, caPoolBad := writeThrowawayPKIWithServer(t, dirBad, "news-creator")
	serverBad := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"pass"}`))
	}))
	serverBad.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCertBad},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPoolBad,
	}
	serverBad.StartTLS()
	defer serverBad.Close()

	healthCheckerBad := service.NewHealthCheckerServiceWithClient(newsClient, serverBad.URL, log)
	errBad := healthCheckerBad.CheckNewsCreatorHealth(ctx)
	require.Error(t, errBad, "health check against server with untrusted CA must fail")
}

func TestProductionTLSFactoryWire_RedirectAndStreamClonePolicy(t *testing.T) {
	dir := t.TempDir()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	certPath, keyPath, caPath, serverCert, caPool := writeThrowawayPKIWithServer(t, dir, "news-creator")

	var separateHits int64
	separateDest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&separateHits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer separateDest.Close()

	redirectServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, separateDest.URL+"/redirected", http.StatusFound)
	}))
	redirectServer.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
	}
	redirectServer.StartTLS()
	defer redirectServer.Close()

	t.Setenv("MTLS_ENFORCE", "true")
	t.Setenv("MTLS_CERT_FILE", certPath)
	t.Setenv("MTLS_KEY_FILE", keyPath)
	t.Setenv("MTLS_CA_FILE", caPath)
	t.Setenv("NEWS_CREATOR_MTLS_SERVER_NAME", "news-creator")

	cfg := &config.Config{
		NewsCreator: config.NewsCreatorConfig{
			Host:    redirectServer.URL,
			Timeout: 600 * time.Second,
		},
	}

	newsClient, err := buildNewsHTTPClient(cfg, log)
	require.NoError(t, err)
	assert.Equal(t, 600*time.Second, newsClient.Timeout)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. newsClient does not follow redirects
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, redirectServer.URL, nil)
	require.NoError(t, err)
	resp, err := newsClient.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusFound, resp.StatusCode)
	_ = resp.Body.Close()
	assert.Equal(t, int64(0), atomic.LoadInt64(&separateHits), "separate destination must receive 0 hits")

	// 2. streamClient clone preserves Transport and CheckRedirect policy, with Timeout 0
	streamClient := &http.Client{
		Timeout:       0,
		Transport:     newsClient.Transport,
		CheckRedirect: newsClient.CheckRedirect,
	}
	assert.Equal(t, time.Duration(0), streamClient.Timeout)
	assert.Equal(t, newsClient.Transport, streamClient.Transport)
	assert.NotNil(t, streamClient.CheckRedirect)

	reqStream, err := http.NewRequestWithContext(ctx, http.MethodGet, redirectServer.URL, nil)
	require.NoError(t, err)
	respStream, err := streamClient.Do(reqStream)
	require.NoError(t, err)
	assert.Equal(t, http.StatusFound, respStream.StatusCode)
	_ = respStream.Body.Close()
	assert.Equal(t, int64(0), atomic.LoadInt64(&separateHits), "separate destination must receive 0 hits from stream client")
}

func TestResolveServiceURLs_MTLSEnforced(t *testing.T) {
	const validDataHub = "https://alt-data-hub:9443"
	const validNews = "https://news-creator:9443"

	t.Run("valid HTTPS URLs succeed", func(t *testing.T) {
		backendURL, newsURL, err := ResolveServiceURLs(true, "http://legacy-alt-backend:9000", validDataHub, validNews)
		require.NoError(t, err)
		assert.Equal(t, validDataHub, backendURL)
		assert.Equal(t, validNews, newsURL)
	})

	t.Run("empty BACKEND_API_MTLS_URL fails closed without fallback to plaintext", func(t *testing.T) {
		_, _, err := ResolveServiceURLs(true, "http://legacy-alt-backend:9000", "", validNews)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "BACKEND_API_MTLS_URL is required and must be nonempty HTTPS when MTLS_ENFORCE=true")
	})

	t.Run("plaintext HTTP BACKEND_API_MTLS_URL rejected", func(t *testing.T) {
		_, _, err := ResolveServiceURLs(true, "", "http://alt-data-hub:9443", validNews)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must use https when mTLS is enforced")
	})

	t.Run("plaintext HTTP NewsCreator URL rejected", func(t *testing.T) {
		_, _, err := ResolveServiceURLs(true, "", validDataHub, "http://news-creator:11434")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must use https when mTLS is enforced")
	})

	t.Run("userinfo in DataHub URL rejected without leaking credentials", func(t *testing.T) {
		_, _, err := ResolveServiceURLs(true, "", "https://secretuser:secretpass@alt-data-hub:9443/path?query=secretquery", validNews)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must not contain user credentials/userinfo")
		assert.NotContains(t, err.Error(), "secretuser")
		assert.NotContains(t, err.Error(), "secretpass")
		assert.NotContains(t, err.Error(), "secretquery")
	})

	t.Run("userinfo in NewsCreator URL rejected without leaking credentials", func(t *testing.T) {
		_, _, err := ResolveServiceURLs(true, "", validDataHub, "https://admin:supersecret@news-creator:9443/path?query=secretquery")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must not contain user credentials/userinfo")
		assert.NotContains(t, err.Error(), "admin")
		assert.NotContains(t, err.Error(), "supersecret")
		assert.NotContains(t, err.Error(), "secretquery")
	})

	t.Run("malformed URL rejected without leaking input", func(t *testing.T) {
		_, _, err := ResolveServiceURLs(true, "", "https://[::1]:namedport/secretpath?secretquery\nnewline", validNews)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "malformed")
		assert.NotContains(t, err.Error(), "secretpath")
		assert.NotContains(t, err.Error(), "secretquery")
		assert.NotContains(t, err.Error(), "\n")
	})
}

func TestResolveServiceURLs_DevPlaintext(t *testing.T) {
	t.Run("HTTP allowed when MTLS_ENFORCE=false", func(t *testing.T) {
		backendURL, newsURL, err := ResolveServiceURLs(false, "http://alt-backend:9000", "", "http://news-creator:11434")
		require.NoError(t, err)
		assert.Equal(t, "http://alt-backend:9000", backendURL)
		assert.Equal(t, "http://news-creator:11434", newsURL)
	})

	t.Run("uses MTLS_URL override if present in dev mode", func(t *testing.T) {
		backendURL, newsURL, err := ResolveServiceURLs(false, "http://alt-backend:9000", "http://custom-backend:9000", "http://news-creator:11434")
		require.NoError(t, err)
		assert.Equal(t, "http://custom-backend:9000", backendURL)
		assert.Equal(t, "http://news-creator:11434", newsURL)
	})

	t.Run("missing both backend URLs fails", func(t *testing.T) {
		_, _, err := ResolveServiceURLs(false, "", "", "http://news-creator:11434")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "BACKEND_API_URL is required")
	})

	t.Run("userinfo rejected even in dev mode without leaking credentials", func(t *testing.T) {
		_, _, err := ResolveServiceURLs(false, "http://devuser:devpass@alt-backend:9000/path?q=secret", "", "http://news-creator:11434")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must not contain user credentials/userinfo")
		assert.NotContains(t, err.Error(), "devuser")
		assert.NotContains(t, err.Error(), "devpass")
		assert.NotContains(t, err.Error(), "secret")
	})
}

func TestBuildDependencies_FailsBeforeDBInitOnInvalidURLs(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Enforce mTLS but provide plaintext HTTP and no BACKEND_API_MTLS_URL.
	// Also point DB host to an invalid host: if DB connection is attempted before
	// URL validation, it would either hang or fail with a DB dial error.
	t.Setenv("MTLS_ENFORCE", "true")
	t.Setenv("BACKEND_API_URL", "http://alt-backend:9000")
	t.Setenv("BACKEND_API_MTLS_URL", "")
	t.Setenv("NEWS_CREATOR_HOST", "https://news-creator:9443")
	t.Setenv("PRE_PROCESSOR_DB_HOST", "invalid-host-should-never-be-reached")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	deps, cleanup, err := BuildDependencies(ctx, log, false)
	require.Error(t, err)
	assert.Nil(t, deps)
	assert.Nil(t, cleanup)
	assert.Contains(t, err.Error(), "service URL validation failed (fail-closed)")
	assert.Contains(t, err.Error(), "BACKEND_API_MTLS_URL is required and must be nonempty HTTPS")
}
