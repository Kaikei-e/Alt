package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"knowledge-sovereign/config"
)

func TestNewHTTPServer(t *testing.T) {
	mux := http.NewServeMux()
	srv := newHTTPServer(":9500", mux, 30*time.Second)

	if srv.Addr != ":9500" {
		t.Errorf("Addr = %s, want :9500", srv.Addr)
	}
	if srv.Handler != mux {
		t.Error("Handler not properly set")
	}
	if srv.ReadHeaderTimeout != 10*time.Second {
		t.Errorf("ReadHeaderTimeout = %v, want 10s", srv.ReadHeaderTimeout)
	}
	if srv.ReadTimeout != 30*time.Second {
		t.Errorf("ReadTimeout = %v, want 30s", srv.ReadTimeout)
	}
	if srv.WriteTimeout != 30*time.Second {
		t.Errorf("WriteTimeout = %v, want 30s", srv.WriteTimeout)
	}
	if srv.IdleTimeout != 120*time.Second {
		t.Errorf("IdleTimeout = %v, want 120s", srv.IdleTimeout)
	}
	if srv.MaxHeaderBytes != 1<<20 {
		t.Errorf("MaxHeaderBytes = %d, want %d", srv.MaxHeaderBytes, 1<<20)
	}
}

func TestBuildRPCMux_HealthEndpoint(t *testing.T) {
	cfg := &config.Config{
		EventToken:       "test-event-token",
		EventAuthEnabled: false,
	}
	mux := buildRPCMux(testSovereignHandler{}, cfg)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for health endpoint, got %d", rec.Code)
	}
}
