package pki

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestOpsListenAddr_DefaultLoopback9110(t *testing.T) {
	t.Setenv(opsListenEnv, "")
	if got := OpsListenAddr(); got != defaultOpsListenAddr {
		t.Fatalf("OpsListenAddr()=%q want %q", got, defaultOpsListenAddr)
	}
	if defaultOpsListenAddr != "127.0.0.1:9110" {
		t.Fatalf("GREEN ops bind drifted: %q", defaultOpsListenAddr)
	}
}

func TestOpsListenAddr_Override(t *testing.T) {
	t.Setenv(opsListenEnv, ":9110")
	if got := OpsListenAddr(); got != ":9110" {
		t.Fatalf("got %q", got)
	}
}

func TestNewOpsHandler_NilMetrics503(t *testing.T) {
	h := NewOpsHandler("pre-processor", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want 503", rec.Code)
	}
	health := httptest.NewRecorder()
	h.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health=%d", health.Code)
	}
}

func TestNewOpsHandler_PrivateRegistryMetrics(t *testing.T) {
	reg := prometheus.NewPedanticRegistry()
	obs := NewPromObserver("pre-processor-ops", reg)
	obs.OnClassified(StateFresh, time.Hour)
	h := NewOpsHandler("pre-processor", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `pki_enrollment_healthy{subject="pre-processor-ops"} 1`) {
		t.Fatalf("missing private-registry series:\n%s", rec.Body.String())
	}
}

func TestListenOps_ShutdownIdempotent(t *testing.T) {
	t.Setenv(opsListenEnv, "127.0.0.1:0")
	srv, err := ListenOps(context.Background(), nil, "pre-processor", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get("http://" + srv.Addr + "/health")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health=%d", resp.StatusCode)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := ShutdownOps(ctx, srv); err != nil {
		t.Fatal(err)
	}
	if err := ShutdownOps(ctx, srv); err != nil {
		t.Fatalf("second shutdown: %v", err)
	}
	if err := ShutdownOps(ctx, nil); err != nil {
		t.Fatalf("nil shutdown: %v", err)
	}
}

func TestNewOpsHandler_DoesNotUseDefaultServeMux(t *testing.T) {
	h := NewOpsHandler("pre-processor", nil)
	if h == http.DefaultServeMux {
		t.Fatal("ops mux must not be http.DefaultServeMux")
	}
}
