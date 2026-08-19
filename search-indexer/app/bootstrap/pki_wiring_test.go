package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"search-indexer/config"
	appOtel "search-indexer/utils/otel"
)

func TestRunWiresPKIStartBeforeTLS(t *testing.T) {
	body, err := os.ReadFile("app.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(body)
	pkiIdx := strings.Index(src, "pki.Start(")
	if pkiIdx < 0 {
		t.Fatal("bootstrap.Run must call pki.Start before TLS init")
	}
	clientIdx := strings.Index(src, "initArticleDriver(")
	serverIdx := strings.Index(src, "tlsutil.LoadServerConfig(")
	if clientIdx >= 0 && pkiIdx > clientIdx {
		t.Fatal("pki.Start must run before initArticleDriver (mTLS client)")
	}
	if serverIdx >= 0 && pkiIdx > serverIdx {
		t.Fatal("pki.Start must run before tlsutil.LoadServerConfig")
	}
	if !strings.Contains(src, `"search-indexer"`) {
		t.Fatal("pki.Start must be called with CERT_SUBJECT search-indexer")
	}
	if !strings.Contains(src, "pki.ListenOps(") {
		t.Fatal("bootstrap.Run must start the dedicated PKI ops listener")
	}
	if !strings.Contains(src, "pki.ShutdownOps(") {
		t.Fatal("bootstrap.Run must shut down the dedicated PKI ops listener")
	}
	if strings.Contains(src, "prometheus.DefaultRegisterer") {
		t.Fatal("PKI metrics must not use prometheus.DefaultRegisterer")
	}
}

func TestHTTPServer_DoesNotExposeAppMetrics(t *testing.T) {
	srv := newHTTPServer(nil, nil, appOtel.Config{}, config.RateLimitConfig{RequestsPerSecond: 100, Burst: 100},
		func(context.Context) error { return nil })
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("/metrics = %d, want 404 (PKI scrape is parent :9110)", rec.Code)
	}
}

func TestServersSource_DoesNotMountMetrics(t *testing.T) {
	body, err := os.ReadFile("servers.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(body)
	if strings.Contains(src, `mux.Handle("/metrics"`) || strings.Contains(src, "promhttp.Handler") {
		t.Fatal("application mux must not mount /metrics; PKI uses dedicated ops :9110")
	}
}
