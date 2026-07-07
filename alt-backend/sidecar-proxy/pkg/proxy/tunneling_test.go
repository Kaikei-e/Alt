package proxy

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alt-rss/alt-backend/sidecar-proxy/pkg/config"
)

// TestForwardToEnvoyConnect_SetsHostToTargetDomain guards against the
// regression where forwardToEnvoyConnect called
// proxyReq.Header.Set("Host", targetHost), which net/http silently ignores
// when sending the request. Envoy must see the target domain as the Host,
// not the sidecar's own Envoy-upstream dial address.
func TestForwardToEnvoyConnect_SetsHostToTargetDomain(t *testing.T) {
	var capturedHost string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHost = r.Host
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	p := &LightweightProxy{
		config: &config.ProxyConfig{
			EnvoyUpstream:      strings.TrimPrefix(upstream.URL, "http://"),
			CONNECTIdleTimeout: time.Minute,
		},
		logger: log.New(io.Discard, "", 0),
	}

	req := httptest.NewRequest(http.MethodGet, "/connect/model-hub.example.com/api", nil)
	rec := httptest.NewRecorder()

	const targetHost = "model-hub.example.com"
	if err := p.forwardToEnvoyConnect(rec, req, targetHost, "/api", "trace-1", time.Now()); err != nil {
		t.Fatalf("forwardToEnvoyConnect() error = %v", err)
	}

	if capturedHost != targetHost {
		t.Errorf("upstream received Host = %q, want %q", capturedHost, targetHost)
	}
}
