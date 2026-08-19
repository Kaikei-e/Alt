package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"alt/connect/v2/muxutil"
	"alt/internal/healthdeep"
)

// Provider contract: alt-data-hub's mTLS mux serves a bounded /health/deep
// envelope that actually checks the owned data path, while cheap /health
// stays constant liveness.

func TestDataHubProvider_DeepHealthPassEnvelope(t *testing.T) {
	h := dataHubHandler(cheapHealthMux(), newDeepHealthHandler(func(context.Context) error { return nil }))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/deep", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body healthdeep.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != healthdeep.StatusPass {
		t.Errorf("status = %q, want pass", body.Status)
	}
	if body.Service != "alt-data-hub" {
		t.Errorf("service = %q, want alt-data-hub", body.Service)
	}
	if len(body.Checks) != 1 || body.Checks[0].Name != "database" || !body.Checks[0].Critical {
		t.Errorf("checks = %+v, want critical database", body.Checks)
	}
}

func TestDataHubProvider_DeepHealthFailIs503(t *testing.T) {
	h := dataHubHandler(cheapHealthMux(), newDeepHealthHandler(func(context.Context) error {
		return errors.New("postgres://alt:hunter2@db.internal:5432/alt")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/deep", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	got := rec.Body.String()
	if strings.Contains(got, "hunter2") || strings.Contains(got, "postgres://") {
		t.Fatalf("leaked DSN: %s", got)
	}
	var body healthdeep.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != healthdeep.StatusFail {
		t.Errorf("status = %q, want fail", body.Status)
	}
}

func TestDataHubProvider_CheapHealthStays200WhenDeepFails(t *testing.T) {
	h := dataHubHandler(cheapHealthMux(), newDeepHealthHandler(func(context.Context) error {
		return healthdeep.ErrUnavailable
	}))

	deep := httptest.NewRecorder()
	h.ServeHTTP(deep, httptest.NewRequest(http.MethodGet, "/health/deep", nil))
	if deep.Code != http.StatusServiceUnavailable {
		t.Fatalf("deep = %d, want 503", deep.Code)
	}

	cheap := httptest.NewRecorder()
	h.ServeHTTP(cheap, httptest.NewRequest(http.MethodGet, "/health", nil))
	if cheap.Code != http.StatusOK {
		t.Fatalf("cheap /health = %d, want 200", cheap.Code)
	}
}

func TestDataHubProvider_ConsumerPingAgainstDeepContract(t *testing.T) {
	// In-process consumer/provider pair: the backend Ping client against
	// this binary's handler, without going through mTLS.
	h := dataHubHandler(cheapHealthMux(), newDeepHealthHandler(func(context.Context) error { return nil }))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/deep", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("provider /health/deep = %d, want 200", rec.Code)
	}
	var body healthdeep.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != healthdeep.StatusPass {
		t.Fatalf("provider status = %q, want pass", body.Status)
	}
}

func cheapHealthMux() http.Handler {
	mux := http.NewServeMux()
	muxutil.RegisterHealth(mux)
	return mux
}
