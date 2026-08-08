package bootstrap

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"pre-processor/config"
	"pre-processor/metrics"
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
