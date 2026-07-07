package proxy

import (
	"io"
	"log"
	"net"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/alt-rss/alt-backend/sidecar-proxy/pkg/config"
)

// TestBuildEnvoyRequest_SetsHostField guards against the regression where
// buildEnvoyRequest called req.Header.Set("Host", hostname), which net/http
// silently ignores when sending the request — the outgoing Host line (and,
// over HTTP/2, the :authority pseudo-header) comes from req.Host. Without
// this fix, Envoy would see the sidecar's own upstream address as the Host
// instead of the target domain, defeating "THE MAGIC HEADERS".
func TestBuildEnvoyRequest_SetsHostField(t *testing.T) {
	p := &LightweightProxy{
		config: &config.ProxyConfig{EnvoyUpstream: "localhost:10000"},
		logger: log.New(io.Discard, "", 0),
	}

	originalReq := httptest.NewRequest("GET", "/proxy/https://zenn.dev/feed", nil)
	targetURL, err := url.Parse("https://zenn.dev/feed")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	envoyReq, err := p.buildEnvoyRequest(originalReq, targetURL, net.ParseIP("93.184.216.34"), "trace-1")
	if err != nil {
		t.Fatalf("buildEnvoyRequest() error = %v", err)
	}

	if envoyReq.Host != "zenn.dev" {
		t.Errorf("envoyReq.Host = %q, want %q (Header.Set(\"Host\", ...) does not affect the wire Host line)", envoyReq.Host, "zenn.dev")
	}
}
