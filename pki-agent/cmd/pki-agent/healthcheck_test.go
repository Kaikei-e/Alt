package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// listen opens an ephemeral TCP listener and returns its :port string (with leading colon).
func listen(t *testing.T) (string, net.Listener) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	return ":" + port, ln
}

// TestRunHealthcheck_MetricsAlive_NoProxy: the legacy path. Only the metrics
// /healthz matters. Proxy listener probe is skipped when PROXY_LISTEN is empty.
func TestRunHealthcheck_MetricsAlive_NoProxy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	metricsAddr := strings.TrimPrefix(srv.URL, "http://")

	if err := runHealthcheck(metricsAddr, ""); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

// TestRunHealthcheck_MetricsDead: /healthz 5xx or unreachable must fail fast.
func TestRunHealthcheck_MetricsDead(t *testing.T) {
	if err := runHealthcheck("127.0.0.1:1", ""); err == nil {
		t.Fatal("want error when metrics unreachable, got nil")
	}
}

// TestRunHealthcheck_ProxyAlive: when PROXY_LISTEN is set and dial succeeds,
// we return nil. This guards against reading the env var incorrectly.
func TestRunHealthcheck_ProxyAlive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	metricsAddr := strings.TrimPrefix(srv.URL, "http://")

	proxyAddr, ln := listen(t)
	defer ln.Close()

	if err := runHealthcheck(metricsAddr, proxyAddr); err != nil {
		t.Fatalf("want nil with alive proxy, got %v", err)
	}
}

// TestRunHealthcheck_ProxyDead: when PROXY_LISTEN is configured but nothing
// is listening there, healthcheck must fail so Docker marks the container
// unhealthy — this is the scenario we hit in production when the reverse
// proxy goroutine died silently.
func TestRunHealthcheck_ProxyDead(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	metricsAddr := strings.TrimPrefix(srv.URL, "http://")

	// Bind + immediately close to reserve a port that is guaranteed dead.
	_, ln := listen(t)
	addr := ln.Addr().String()
	ln.Close()
	_, port, _ := net.SplitHostPort(addr)
	proxyAddr := ":" + port

	err := runHealthcheck(metricsAddr, proxyAddr)
	if err == nil {
		t.Fatal("want error when proxy listener dead, got nil")
	}
	if !strings.Contains(err.Error(), "proxy") {
		t.Fatalf("error should mention proxy, got: %v", err)
	}
}

// TestRunHealthcheck_ProxyColonOnly: the env convention is ":9443" (no host).
// The probe must default to 127.0.0.1:<port> — exactly matching the loopback
// semantics of the reverse-proxy listener.
func TestRunHealthcheck_ProxyColonOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	metricsAddr := strings.TrimPrefix(srv.URL, "http://")

	proxyAddr, ln := listen(t) // ":NNNNN"
	defer ln.Close()

	if !strings.HasPrefix(proxyAddr, ":") {
		t.Fatalf("test expected bare :port, got %q", proxyAddr)
	}
	if err := runHealthcheck(metricsAddr, proxyAddr); err != nil {
		t.Fatalf("want nil for :port dial, got %v", err)
	}
}

// Compile-time sanity: ensure fmt/errors imports used elsewhere do not drift
// away without us noticing. Safe to remove once the package has more code.
var _ = fmt.Errorf
