package main

import (
	"errors"
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

// stubInterfaces swaps listInterfaces for the test lifetime.
func stubInterfaces(t *testing.T, stub func() ([]ifaceInfo, error)) {
	t.Helper()
	prev := listInterfaces
	listInterfaces = stub
	t.Cleanup(func() { listInterfaces = prev })
}

// TestProbeNetns_HealthyNamespace: lo + eth0 with non-loopback addr passes.
func TestProbeNetns_HealthyNamespace(t *testing.T) {
	stubInterfaces(t, func() ([]ifaceInfo, error) {
		return []ifaceInfo{
			{Name: "lo", IsUp: true, Loopback: true, Addrs: []net.IP{net.IPv4(127, 0, 0, 1)}},
			{Name: "eth0", IsUp: true, Loopback: false, Addrs: []net.IP{net.IPv4(172, 18, 0, 14)}},
		}, nil
	})
	if err := probeNetns(); err != nil {
		t.Fatalf("want nil for healthy netns, got %v", err)
	}
}

// TestProbeNetns_LoopbackOnly: only lo — the netns-orphan signature from
// ADR-000782. sidecar's parent was force-recreated; eth0 is gone.
func TestProbeNetns_LoopbackOnly(t *testing.T) {
	stubInterfaces(t, func() ([]ifaceInfo, error) {
		return []ifaceInfo{
			{Name: "lo", IsUp: true, Loopback: true, Addrs: []net.IP{net.IPv4(127, 0, 0, 1)}},
		}, nil
	})
	err := probeNetns()
	if err == nil {
		t.Fatal("want error for loopback-only netns, got nil")
	}
	if !strings.Contains(err.Error(), "orphan") {
		t.Fatalf("error should mention orphan, got: %v", err)
	}
}

// TestProbeNetns_InterfaceDown: eth0 present but FlagUp=0 → fail. The kernel
// doesn't tear down the interface struct immediately after netns teardown;
// the carrier drops first.
func TestProbeNetns_InterfaceDown(t *testing.T) {
	stubInterfaces(t, func() ([]ifaceInfo, error) {
		return []ifaceInfo{
			{Name: "lo", IsUp: true, Loopback: true, Addrs: []net.IP{net.IPv4(127, 0, 0, 1)}},
			{Name: "eth0", IsUp: false, Loopback: false, Addrs: []net.IP{net.IPv4(172, 18, 0, 14)}},
		}, nil
	})
	if err := probeNetns(); err == nil {
		t.Fatal("want error when eth0 down, got nil")
	}
}

// TestProbeNetns_NonLoopbackIfaceHasLoopbackAddrOnly: eth0 up but only
// 127.x addresses — still treated as orphan; real connectivity requires a
// routable non-loopback IP.
func TestProbeNetns_NonLoopbackIfaceHasLoopbackAddrOnly(t *testing.T) {
	stubInterfaces(t, func() ([]ifaceInfo, error) {
		return []ifaceInfo{
			{Name: "eth0", IsUp: true, Loopback: false, Addrs: []net.IP{net.IPv4(127, 0, 0, 2)}},
		}, nil
	})
	if err := probeNetns(); err == nil {
		t.Fatal("want error when non-loopback iface has only loopback addr, got nil")
	}
}

// TestProbeNetns_ListError: kernel enumeration failure must propagate.
func TestProbeNetns_ListError(t *testing.T) {
	stubInterfaces(t, func() ([]ifaceInfo, error) {
		return nil, errors.New("kernel failed")
	})
	if err := probeNetns(); err == nil {
		t.Fatal("want error propagation, got nil")
	}
}

// TestRunHealthcheck_NetnsOrphanFailsHealthcheck: when PROXY_LISTEN is set and
// the netns has degraded to loopback-only, runHealthcheck must fail even if
// probeMetrics and probeProxy would pass (because the local listener still
// lives in the stale netns alongside the sidecar itself).
func TestRunHealthcheck_NetnsOrphanFailsHealthcheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	metricsAddr := strings.TrimPrefix(srv.URL, "http://")

	proxyAddr, ln := listen(t)
	defer ln.Close()

	stubInterfaces(t, func() ([]ifaceInfo, error) {
		return []ifaceInfo{
			{Name: "lo", IsUp: true, Loopback: true, Addrs: []net.IP{net.IPv4(127, 0, 0, 1)}},
		}, nil
	})

	err := runHealthcheck(metricsAddr, proxyAddr)
	if err == nil {
		t.Fatal("want error when netns orphan, got nil")
	}
	if !strings.Contains(err.Error(), "netns") && !strings.Contains(err.Error(), "orphan") {
		t.Fatalf("error should mention netns/orphan, got: %v", err)
	}
}

// TestRunHealthcheck_HealthyWithProxy: sanity that a fully-healthy probe
// (metrics green, proxy listening, netns has eth0) returns nil. Guards against
// the probeNetns branch accidentally shadowing the happy path.
func TestRunHealthcheck_HealthyWithProxy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	metricsAddr := strings.TrimPrefix(srv.URL, "http://")

	proxyAddr, ln := listen(t)
	defer ln.Close()

	stubInterfaces(t, func() ([]ifaceInfo, error) {
		return []ifaceInfo{
			{Name: "lo", IsUp: true, Loopback: true, Addrs: []net.IP{net.IPv4(127, 0, 0, 1)}},
			{Name: "eth0", IsUp: true, Loopback: false, Addrs: []net.IP{net.IPv4(172, 18, 0, 14)}},
		}, nil
	})

	if err := runHealthcheck(metricsAddr, proxyAddr); err != nil {
		t.Fatalf("want nil with healthy netns, got %v", err)
	}
}
