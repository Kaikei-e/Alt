package handler

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

type stubDeepPinger struct {
	dbErr     error
	projReady bool
	projErr   error
}

func (s stubDeepPinger) PingDB(context.Context) error { return s.dbErr }
func (s stubDeepPinger) PingProjectors(context.Context, []string) (bool, error) {
	return s.projReady, s.projErr
}

func TestDeepHealth_DBFailIs503(t *testing.T) {
	h := NewDeepHealthHandler(stubDeepPinger{
		dbErr: errors.New("postgres://alt:hunter2@db.internal/sovereign"),
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/deep", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "hunter2") || strings.Contains(body, "postgres://") {
		t.Fatalf("leaked DSN: %s", body)
	}
	var report map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report["status"] != "fail" {
		t.Errorf("status = %v", report["status"])
	}
}

func TestDeepHealth_ProjectorNotReadyIs200Warn(t *testing.T) {
	h := NewDeepHealthHandler(stubDeepPinger{projReady: false})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/deep", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var report struct {
		Status string `json:"status"`
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Reason string `json:"reason"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "warn" {
		t.Errorf("status = %q, want warn", report.Status)
	}
	found := false
	for _, c := range report.Checks {
		if c.Name == "projector" {
			found = true
			if c.Status != "warn" || c.Reason != "not_ready" {
				t.Errorf("projector = %+v", c)
			}
		}
	}
	if !found {
		t.Fatal("missing projector check")
	}
}

func TestDeepHealth_AllPass(t *testing.T) {
	h := NewDeepHealthHandler(stubDeepPinger{projReady: true})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/deep", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"pass"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestHealthHandler_CheapLivenessUnchanged(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	HealthHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cheap /health = %d", rec.Code)
	}
}

type blockingDeepPinger struct {
	started chan struct{}
	release chan struct{}
}

func (b blockingDeepPinger) PingDB(ctx context.Context) error {
	close(b.started)
	select {
	case <-b.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (b blockingDeepPinger) PingProjectors(context.Context, []string) (bool, error) {
	return true, nil
}

func TestDeepHealth_CancelledLeaderDoesNotPoisonCache(t *testing.T) {
	pinger := blockingDeepPinger{started: make(chan struct{}), release: make(chan struct{})}
	h := NewDeepHealthHandler(pinger)

	leaderCtx, cancel := context.WithCancel(context.Background())
	leaderDone := make(chan int, 1)
	go func() {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/deep", nil).WithContext(leaderCtx))
		leaderDone <- rec.Code
	}()
	<-pinger.started
	cancel()

	waiterDone := make(chan int, 1)
	go func() {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/deep", nil))
		waiterDone <- rec.Code
	}()

	time.Sleep(20 * time.Millisecond)
	close(pinger.release)

	if got := <-leaderDone; got != http.StatusOK {
		t.Errorf("leader = %d, want 200", got)
	}
	if got := <-waiterDone; got != http.StatusOK {
		t.Errorf("waiter = %d, want 200 (must not inherit cancelled leader fail)", got)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/deep", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("cache poisoned: %d", rec.Code)
	}
}
