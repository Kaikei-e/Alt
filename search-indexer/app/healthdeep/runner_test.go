package healthdeep

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	io_prometheus_client "github.com/prometheus/client_model/go"
)

func TestRunner_CriticalFailIs503(t *testing.T) {
	r := NewRunner(Config{
		Service: "alt-backend",
		Checks: []Check{{
			Name:     "datahub",
			Critical: true,
			Probe:    func(context.Context) error { return ErrUnavailable },
		}},
		CacheTTL: 2 * time.Second,
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/deep", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	var body Report
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != StatusFail {
		t.Errorf("status = %q, want fail", body.Status)
	}
	if body.Checks[0].Status != StatusFail || !body.Checks[0].Critical {
		t.Errorf("check = %+v", body.Checks[0])
	}
}

func TestRunner_NonCriticalFailIs200Warn(t *testing.T) {
	r := NewRunner(Config{
		Service: "alt-backend",
		Checks: []Check{{
			Name:     "optional",
			Critical: false,
			Probe:    func(context.Context) error { return ErrNotReady },
		}},
		CacheTTL: 2 * time.Second,
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/deep", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body Report
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != StatusWarn {
		t.Errorf("status = %q, want warn", body.Status)
	}
	if body.Checks[0].Status != StatusWarn {
		t.Errorf("check status = %q, want warn", body.Checks[0].Status)
	}
	if body.Checks[0].Reason != ReasonNotReady {
		t.Errorf("reason = %q, want not_ready", body.Checks[0].Reason)
	}
}

func TestRunner_PassIs200(t *testing.T) {
	r := NewRunner(Config{
		Service: "alt-backend",
		Checks: []Check{{
			Name:     "datahub",
			Critical: true,
			Probe:    func(context.Context) error { return nil },
		}},
		CacheTTL: 2 * time.Second,
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/deep", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body Report
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != StatusPass {
		t.Errorf("status = %q, want pass", body.Status)
	}
}

func TestRunner_DoesNotLeakProbeSecrets(t *testing.T) {
	//nolint:gosec // G101: leak-assertion fixture, not a live credential
	secret := "postgres://alt:hunter2@db.internal:5432/alt?sslmode=disable /var/lib/secret peer=svc-backend panic stack"
	r := NewRunner(Config{
		Service: "alt-backend",
		Checks: []Check{{
			Name:     "datahub",
			Critical: true,
			Probe: func(context.Context) error {
				return errors.New(secret)
			},
		}},
		CacheTTL: 2 * time.Second,
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/deep", nil))
	got := rec.Body.String()
	if strings.Contains(got, secret) || strings.Contains(got, "hunter2") ||
		strings.Contains(got, "postgres://") || strings.Contains(got, "/var/lib") ||
		strings.Contains(got, "peer=") || strings.Contains(got, "stack") {
		t.Fatalf("response leaked probe details: %s", got)
	}
	if !strings.Contains(got, `"reason":"unavailable"`) {
		t.Fatalf("want opaque reason, got %s", got)
	}
}

func TestRunner_RunsChecksInParallel(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	probe := func(context.Context) error {
		started <- struct{}{}
		<-release
		return nil
	}
	r := NewRunner(Config{
		Service: "alt-backend",
		Checks: []Check{
			{Name: "a", Critical: true, Probe: probe},
			{Name: "b", Critical: true, Probe: probe},
		},
		CacheTTL: 2 * time.Second,
		PerCheck: 400 * time.Millisecond,
		Budget:   800 * time.Millisecond,
	})

	done := make(chan Report, 1)
	go func() { done <- r.Run(context.Background()) }()

	waitBoth := time.After(200 * time.Millisecond)
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-waitBoth:
			t.Fatal("checks did not start in parallel")
		}
	}
	close(release)
	rep := <-done
	if rep.Status != StatusPass {
		t.Errorf("status = %q", rep.Status)
	}
}

func TestRunner_PerCheckTimeout(t *testing.T) {
	r := NewRunner(Config{
		Service: "alt-backend",
		Checks: []Check{{
			Name:     "slow",
			Critical: true,
			Probe: func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			},
		}},
		CacheTTL: 2 * time.Second,
		PerCheck: 20 * time.Millisecond,
		Budget:   200 * time.Millisecond,
	})

	start := time.Now()
	rep := r.Run(context.Background())
	if time.Since(start) > 150*time.Millisecond {
		t.Fatalf("run took %s, want per-check timeout to bound it", time.Since(start))
	}
	if rep.Status != StatusFail {
		t.Errorf("status = %q, want fail", rep.Status)
	}
	if rep.Checks[0].Reason != ReasonTimeout {
		t.Errorf("reason = %q, want timeout", rep.Checks[0].Reason)
	}
}

func TestRunner_CacheAndSingleflight(t *testing.T) {
	var calls atomic.Int32
	block := make(chan struct{})
	r := NewRunner(Config{
		Service: "alt-backend",
		Checks: []Check{{
			Name:     "datahub",
			Critical: true,
			Probe: func(context.Context) error {
				calls.Add(1)
				<-block
				return nil
			},
		}},
		CacheTTL: 2 * time.Second,
	})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); r.Run(context.Background()) }()
	go func() { defer wg.Done(); r.Run(context.Background()) }()

	// Give both goroutines time to enter Run before unblocking the probe.
	time.Sleep(20 * time.Millisecond)
	close(block)
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Fatalf("probe calls = %d, want 1 (singleflight)", got)
	}

	rep := r.Run(context.Background())
	if !rep.Cached {
		t.Fatal("third call should hit the cache")
	}
	if calls.Load() != 1 {
		t.Fatalf("cache hit re-ran the probe")
	}
}

func TestRunner_GlobalBudgetUnderOneSecond(t *testing.T) {
	r := NewRunner(Config{
		Service: "alt-backend",
		Checks: []Check{{
			Name:     "x",
			Critical: true,
			Probe:    func(context.Context) error { return nil },
		}},
		Budget: 5 * time.Second, // constructor must clamp
	})
	if r.cfg.Budget >= time.Second {
		t.Fatalf("budget = %s, want < 1s", r.cfg.Budget)
	}
}

func TestHandler_JSONContentType(t *testing.T) {
	r := NewRunner(Config{
		Service:  "alt-backend",
		Checks:   nil,
		CacheTTL: 2 * time.Second,
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/deep", nil))
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("empty checks should pass, got %d", rec.Code)
	}
	if _, err := io.ReadAll(rec.Body); err != nil {
		t.Fatal(err)
	}
}

func TestRunner_AlreadyCancelledLeaderStillRunsProbes(t *testing.T) {
	var called atomic.Bool
	r := NewRunner(Config{
		Service: "search-indexer",
		Checks: []Check{{
			Name:     "meilisearch",
			Critical: true,
			Probe: func(ctx context.Context) error {
				called.Store(true)
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return nil
			},
		}},
		CacheTTL: 2 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rep := r.Run(ctx)
	if !called.Load() {
		t.Fatal("probe was not called")
	}
	if rep.Status != StatusPass {
		t.Fatalf("status = %q, want pass (caller cancel must not fail the run)", rep.Status)
	}

	cached := r.Run(context.Background())
	if cached.Status != StatusPass {
		t.Fatalf("cache poisoned by cancelled leader: status = %q", cached.Status)
	}
}

func TestRunner_CancelledLeaderDoesNotPoisonWaiters(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	r := NewRunner(Config{
		Service: "search-indexer",
		Checks: []Check{{
			Name:     "meilisearch",
			Critical: true,
			Probe: func(ctx context.Context) error {
				close(started)
				select {
				case <-release:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			},
		}},
		CacheTTL: 2 * time.Second,
		PerCheck: 400 * time.Millisecond,
		Budget:   800 * time.Millisecond,
	})

	leaderCtx, cancel := context.WithCancel(context.Background())
	leaderDone := make(chan Report, 1)
	go func() { leaderDone <- r.Run(leaderCtx) }()
	<-started
	cancel()

	waiterDone := make(chan Report, 1)
	go func() { waiterDone <- r.Run(context.Background()) }()

	time.Sleep(20 * time.Millisecond)
	close(release)

	leaderRep := <-leaderDone
	waiterRep := <-waiterDone
	if leaderRep.Status != StatusPass {
		t.Errorf("leader status = %q, want pass", leaderRep.Status)
	}
	if waiterRep.Status != StatusPass {
		t.Errorf("waiter status = %q, want pass (must not inherit cancelled leader fail)", waiterRep.Status)
	}
}

func TestRunner_CancelledWaiterDoesNotPoisonCache(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	r := NewRunner(Config{
		Service: "search-indexer",
		Checks: []Check{{
			Name:     "meilisearch",
			Critical: true,
			Probe: func(ctx context.Context) error {
				close(started)
				<-release
				return nil
			},
		}},
		CacheTTL: 2 * time.Second,
		PerCheck: 400 * time.Millisecond,
		Budget:   800 * time.Millisecond,
	})

	leaderDone := make(chan Report, 1)
	go func() { leaderDone <- r.Run(context.Background()) }()
	<-started

	waiterCtx, cancel := context.WithCancel(context.Background())
	cancel()
	waiterRep := r.Run(waiterCtx)
	if waiterRep.Status != StatusFail {
		t.Errorf("cancelled waiter status = %q, want fail (cannot wait)", waiterRep.Status)
	}

	close(release)
	leaderRep := <-leaderDone
	if leaderRep.Status != StatusPass {
		t.Fatalf("leader status = %q, want pass", leaderRep.Status)
	}

	cached := r.Run(context.Background())
	if cached.Status != StatusPass {
		t.Fatalf("cache poisoned by cancelled waiter: status = %q", cached.Status)
	}
}

func TestRunner_ObservePublishesGauges(t *testing.T) {
	const svc = "search-indexer-observe-test"
	r := NewRunner(Config{
		Service: svc,
		Checks: []Check{{
			Name:     "meilisearch",
			Critical: true,
			Probe:    func(context.Context) error { return nil },
		}},
		CacheTTL: 2 * time.Second,
	})
	_ = r.Run(context.Background())

	mfs, err := Gatherer().Gather()
	if err != nil {
		t.Fatal(err)
	}
	foundStatus := false
	foundLatency := false
	for _, mf := range mfs {
		switch mf.GetName() {
		case "health_deep_status":
			for _, m := range mf.Metric {
				if gaugeService(m) != svc {
					continue
				}
				foundStatus = true
				if got := m.GetGauge().GetValue(); got != 2 {
					t.Errorf("health_deep_status = %v, want 2 (pass)", got)
				}
			}
		case "health_deep_latency_seconds":
			for _, m := range mf.Metric {
				if gaugeService(m) == svc {
					foundLatency = true
				}
			}
		}
	}
	if !foundStatus {
		t.Fatal("health_deep_status missing from gatherer")
	}
	if !foundLatency {
		t.Fatal("health_deep_latency_seconds missing from gatherer")
	}
}

func gaugeService(m *io_prometheus_client.Metric) string {
	for _, lp := range m.GetLabel() {
		if lp.GetName() == "service" {
			return lp.GetValue()
		}
	}
	return ""
}
