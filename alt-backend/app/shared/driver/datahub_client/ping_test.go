package datahub_client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Consumer contract: Ping talks to alt-data-hub's mTLS /health/deep envelope
// (pass|warn|fail), never the cheap /health liveness route.

func TestPing_PassEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/health/deep" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "pass",
			"service": "alt-data-hub",
			"checks": []map[string]any{{
				"name": "database", "status": "pass", "critical": true, "latency_ms": 1,
			}},
			"latency_ms": 2,
		})
	}))
	defer srv.Close()

	if err := Ping(context.Background(), srv.Client(), srv.URL); err != nil {
		t.Fatalf("Ping() = %v, want nil", err)
	}
}

func TestPing_WarnEnvelopeIsOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health/deep" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"warn","service":"alt-data-hub","checks":[],"latency_ms":1}`))
	}))
	defer srv.Close()

	if err := Ping(context.Background(), srv.Client(), srv.URL); err != nil {
		t.Fatalf("Ping() = %v, want nil (warn is still a live data path)", err)
	}
}

func TestPing_FailEnvelopeIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health/deep" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"fail","service":"alt-data-hub","checks":[{"name":"database","status":"fail","critical":true,"reason":"unavailable"}],"latency_ms":1}`))
	}))
	defer srv.Close()

	err := Ping(context.Background(), srv.Client(), srv.URL)
	if !errors.Is(err, errUnavailable) {
		t.Fatalf("Ping() = %v, want errUnavailable", err)
	}
}

func TestPing_DoesNotTreatCheapHealthAsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"healthy","service":"connect-rpc"}`))
		case "/health/deep":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"fail","service":"alt-data-hub","checks":[{"name":"database","status":"fail","critical":true,"reason":"unavailable"}],"latency_ms":1}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	err := Ping(context.Background(), srv.Client(), srv.URL)
	if err == nil {
		t.Fatal("Ping succeeded on cheap /health while /health/deep failed; must probe the deep contract")
	}
	if !errors.Is(err, errUnavailable) {
		t.Fatalf("Ping() = %v, want errUnavailable", err)
	}
}

func TestPing_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := Ping(ctx, srv.Client(), srv.URL)
	if err == nil {
		t.Fatal("expected timeout")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Ping() = %v, want deadline exceeded", err)
	}
}

func TestPing_NilClient(t *testing.T) {
	if err := Ping(context.Background(), nil, "https://example.invalid"); !errors.Is(err, errUnavailable) {
		t.Fatalf("Ping() = %v, want errUnavailable", err)
	}
}

func TestPing_ErrorDoesNotContainURL(t *testing.T) {
	err := Ping(context.Background(), http.DefaultClient, "https://127.0.0.1:1")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "127.0.0.1") || strings.Contains(err.Error(), "https://") {
		t.Fatalf("error leaked dial target: %v", err)
	}
}

func TestPing_InvalidJSONIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health/deep" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	err := Ping(context.Background(), srv.Client(), srv.URL)
	if !errors.Is(err, errUnavailable) {
		t.Fatalf("Ping() = %v, want errUnavailable", err)
	}
}
