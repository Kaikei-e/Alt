package admin_metrics_gateway

import (
	"alt/domain"
	"alt/orchestrator/driver/prometheus_client"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newProm(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func newClient(t *testing.T, url string) *prometheus_client.Client {
	t.Helper()
	c, err := prometheus_client.New(prometheus_client.Config{URL: url, Timeout: 500 * time.Millisecond})
	if err != nil {
		t.Fatalf("prom client: %v", err)
	}
	return c
}

func TestGateway_Snapshot_ResolvesAllowlistKey(t *testing.T) {
	body := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"job":"alt-backend"},"value":[1700000000,"1"]}]}}`
	var seen atomic.Int32
	srv := newProm(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	g := New(Config{Client: newClient(t, srv.URL), CacheTTL: time.Second, RateLimit: 10})
	snap, err := g.Snapshot(context.Background(), []domain.MetricKey{domain.MetricAvailability}, "", "")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if snap == nil || snap.Metrics[domain.MetricAvailability] == nil {
		t.Fatalf("missing metric in snapshot: %+v", snap)
	}
	mr := snap.Metrics[domain.MetricAvailability]
	if mr.Degraded {
		t.Fatalf("unexpected degraded=true: %+v", mr)
	}
	if len(mr.Series) != 1 || len(mr.Series[0].Points) != 1 {
		t.Fatalf("unexpected series: %+v", mr.Series)
	}
}

func TestGateway_Snapshot_RejectsUnknownKey(t *testing.T) {
	g := New(Config{Client: newClient(t, "http://127.0.0.1:1"), CacheTTL: time.Second, RateLimit: 10})
	_, err := g.Snapshot(context.Background(), []domain.MetricKey{"totally_unknown"}, "", "")
	if err == nil {
		t.Fatalf("expected error for unknown key")
	}
	if !errors.Is(err, domain.ErrUnknownMetricKey) {
		t.Fatalf("expected errors.Is(err, domain.ErrUnknownMetricKey), got: %v", err)
	}
}

func TestGateway_Snapshot_CacheHitSkipsUpstream(t *testing.T) {
	body := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"job":"alt-backend"},"value":[1700000000,"1"]}]}}`
	var seen atomic.Int32
	srv := newProm(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Add(1)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	g := New(Config{Client: newClient(t, srv.URL), CacheTTL: 10 * time.Second, RateLimit: 100})

	for i := 0; i < 5; i++ {
		if _, err := g.Snapshot(context.Background(), []domain.MetricKey{domain.MetricAvailability}, "", ""); err != nil {
			t.Fatalf("Snapshot #%d: %v", i, err)
		}
	}
	if got := seen.Load(); got != 1 {
		t.Fatalf("cache should coalesce; upstream seen=%d want 1", got)
	}
}

func TestGateway_Snapshot_PromTimeout_ReturnsDegraded(t *testing.T) {
	srv := newProm(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer srv.Close()

	c, _ := prometheus_client.New(prometheus_client.Config{URL: srv.URL, Timeout: 20 * time.Millisecond})
	g := New(Config{Client: c, CacheTTL: time.Second, RateLimit: 100})

	snap, err := g.Snapshot(context.Background(), []domain.MetricKey{domain.MetricAvailability}, "", "")
	if err != nil {
		t.Fatalf("Snapshot returned error; want degraded result: %v", err)
	}
	mr := snap.Metrics[domain.MetricAvailability]
	if mr == nil || !mr.Degraded {
		t.Fatalf("expected degraded=true: %+v", mr)
	}
	if mr.Reason != "prometheus timeout" {
		t.Fatalf("reason = %q, want %q", mr.Reason, "prometheus timeout")
	}
	if strings.Contains(mr.Reason, srv.URL) || strings.Contains(mr.Reason, "127.0.0.1") {
		t.Fatalf("reason leaked upstream URL/host: %q", mr.Reason)
	}
}

func TestGateway_Snapshot_UpstreamUnavailable_ReturnsSanitizedReason(t *testing.T) {
	t.Run("http 503", func(t *testing.T) {
		srv := newProm(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		}))
		defer srv.Close()

		g := New(Config{Client: newClient(t, srv.URL), CacheTTL: time.Second, RateLimit: 100})
		snap, err := g.Snapshot(context.Background(), []domain.MetricKey{domain.MetricAvailability}, "", "")
		if err != nil {
			t.Fatalf("Snapshot returned error; want degraded result: %v", err)
		}
		mr := snap.Metrics[domain.MetricAvailability]
		if mr == nil || !mr.Degraded {
			t.Fatalf("expected degraded=true: %+v", mr)
		}
		if mr.Reason != "prometheus unavailable" {
			t.Fatalf("reason = %q, want %q", mr.Reason, "prometheus unavailable")
		}
		if strings.Contains(mr.Reason, srv.URL) || strings.Contains(mr.Reason, "127.0.0.1") {
			t.Fatalf("reason leaked upstream URL/host: %q", mr.Reason)
		}
	})

	t.Run("connection refused", func(t *testing.T) {
		g := New(Config{Client: newClient(t, "http://127.0.0.1:1"), CacheTTL: time.Second, RateLimit: 100})
		snap, err := g.Snapshot(context.Background(), []domain.MetricKey{domain.MetricAvailability}, "", "")
		if err != nil {
			t.Fatalf("Snapshot returned error; want degraded result: %v", err)
		}
		mr := snap.Metrics[domain.MetricAvailability]
		if mr == nil || !mr.Degraded {
			t.Fatalf("expected degraded=true: %+v", mr)
		}
		if mr.Reason != "prometheus unavailable" {
			t.Fatalf("reason = %q, want %q", mr.Reason, "prometheus unavailable")
		}
		if strings.Contains(mr.Reason, "http://") || strings.Contains(mr.Reason, "127.0.0.1") {
			t.Fatalf("reason leaked upstream URL/host: %q", mr.Reason)
		}
	})
}

func TestGateway_Snapshot_RejectsUnknownWindowStep(t *testing.T) {
	body := `{"status":"success","data":{"resultType":"matrix","result":[]}}`
	srv := newProm(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	g := New(Config{Client: newClient(t, srv.URL), CacheTTL: time.Second, RateLimit: 10})

	if _, err := g.Snapshot(context.Background(), []domain.MetricKey{domain.MetricHTTPLatencyP95}, "99d", "15s"); !errors.Is(err, domain.ErrInvalidRange) {
		t.Fatalf("expected ErrInvalidRange for bogus window, got: %v", err)
	}
	if _, err := g.Snapshot(context.Background(), []domain.MetricKey{domain.MetricHTTPLatencyP95}, "5m", "7s"); !errors.Is(err, domain.ErrInvalidRange) {
		t.Fatalf("expected ErrInvalidRange for bogus step, got: %v", err)
	}
	if _, err := g.Snapshot(context.Background(), []domain.MetricKey{domain.MetricHTTPLatencyP95}, domain.RangeWindow24h, domain.Step15s); !errors.Is(err, domain.ErrInvalidRange) {
		t.Fatalf("expected ErrInvalidRange for window/step ratio > 720, got: %v", err)
	}
}

func TestGateway_Catalog_IsNonEmpty(t *testing.T) {
	g := New(Config{Client: newClient(t, "http://127.0.0.1:1"), CacheTTL: time.Second, RateLimit: 10})
	cat := g.Catalog()
	if len(cat) == 0 {
		t.Fatalf("catalog should list allowlisted metrics")
	}
	seen := map[domain.MetricKey]bool{}
	for _, e := range cat {
		if seen[e.Key] {
			t.Fatalf("duplicate key %s", e.Key)
		}
		seen[e.Key] = true
		if e.Unit == "" && e.Kind == domain.SeriesKindInstant {
			t.Fatalf("entry %s missing unit", e.Key)
		}
	}
}

// TestGateway_Catalog_ContainsRealMetrics guards against regressing the
// 2026-04-13 allowlist refresh. Each required key must be present; the
// catalog ordering is irrelevant so we lookup by key.
func TestGateway_Catalog_ContainsRealMetrics(t *testing.T) {
	g := New(Config{Client: newClient(t, "http://127.0.0.1:1"), CacheTTL: time.Second, RateLimit: 10})
	cat := g.Catalog()

	byKey := make(map[domain.MetricKey]domain.MetricCatalogEntry, len(cat))
	for _, e := range cat {
		byKey[e.Key] = e
	}

	required := []domain.MetricKey{
		domain.MetricAvailability,
		domain.MetricHTTPLatencyP50,
		domain.MetricHTTPLatencyP95,
		domain.MetricHTTPLatencyP99,
		domain.MetricHTTPRPS,
		domain.MetricHTTPErrorRatio,
		domain.MetricCPUSaturation,
		domain.MetricMemoryRSS,
		domain.MetricMQHubPublishRate,
		domain.MetricMQHubRedis,
		domain.MetricRecapDBPoolInUse,
		domain.MetricRecapWorkerRSS,
		domain.MetricRecapRequestP95,
		domain.MetricRecapSubworkerAdminSuccess,
		domain.MetricPrometheusScrapeLag,
		domain.MetricAvailabilityBurn1h,
		domain.MetricAvailabilityBurn6h,
	}
	for _, k := range required {
		if _, ok := byKey[k]; !ok {
			t.Fatalf("catalog missing required key %q", k)
		}
	}

	// Legacy keys that were retired in the refresh must NOT be present — the
	// upstream metrics do not exist so clients should get InvalidArgument
	// instead of silently degraded rows.
	for _, k := range []domain.MetricKey{"mqhub_queue_depth", "recap_worker_inflight"} {
		if _, ok := byKey[k]; ok {
			t.Fatalf("retired key %q is still in the catalog", k)
		}
	}
}

// TestGateway_Catalog_AdminMonitorDashboardKeys pins the keys required by the
// /admin/monitor dashboard. Removing one breaks the dashboard contract; adding
// the entry here forces the allowlist to keep up.
func TestGateway_Catalog_AdminMonitorDashboardKeys(t *testing.T) {
	g := New(Config{Client: newClient(t, "http://127.0.0.1:1"), CacheTTL: time.Second, RateLimit: 10})
	cat := g.Catalog()

	byKey := make(map[domain.MetricKey]domain.MetricCatalogEntry, len(cat))
	for _, e := range cat {
		byKey[e.Key] = e
	}

	want := []struct {
		key  domain.MetricKey
		kind domain.SeriesKind
		unit string
	}{
		{domain.MetricAvailability, domain.SeriesKindInstant, "bool"},
		{domain.MetricPrometheusScrapeLag, domain.SeriesKindInstant, "seconds"},
		{domain.MetricScrapeDuration, domain.SeriesKindInstant, "seconds"},
		{domain.MetricHTTPRPS, domain.SeriesKindRange, "req/s"},
		{domain.MetricHTTPErrorRatio, domain.SeriesKindRange, "ratio"},
		{domain.MetricHTTPLatencyP50, domain.SeriesKindRange, "seconds"},
		{domain.MetricHTTPLatencyP95, domain.SeriesKindRange, "seconds"},
		{domain.MetricHTTPLatencyP99, domain.SeriesKindRange, "seconds"},
		{domain.MetricAvailabilityBurn1h, domain.SeriesKindRange, "ratio"},
		{domain.MetricAvailabilityBurn6h, domain.SeriesKindRange, "ratio"},
		{domain.MetricEdgeRPS, domain.SeriesKindRange, "req/s"},
		{domain.MetricEdgeErrorRatio, domain.SeriesKindRange, "ratio"},
		{domain.MetricEdgeLatencyP95, domain.SeriesKindRange, "seconds"},
		{domain.MetricEdgeBurn5m, domain.SeriesKindRange, "ratio"},
		{domain.MetricEdgeBurn30m, domain.SeriesKindRange, "ratio"},
		{domain.MetricHostCPUUtilization, domain.SeriesKindRange, "ratio"},
		{domain.MetricHostMemoryUtilization, domain.SeriesKindRange, "ratio"},
		{domain.MetricHostFilesystemUtilization, domain.SeriesKindInstant, "ratio"},
		{domain.MetricAlertsFiring, domain.SeriesKindInstant, "bool"},
		{domain.MetricAlertsPending, domain.SeriesKindInstant, "bool"},
		{domain.MetricAlertsActiveSince, domain.SeriesKindInstant, "unix_seconds"},
		{domain.MetricCertRemaining, domain.SeriesKindInstant, "seconds"},
		{domain.MetricCertHealthy, domain.SeriesKindInstant, "bool"},
		{domain.MetricMQHubRedis, domain.SeriesKindInstant, "bool"},
		{domain.MetricMQHubPublishRate, domain.SeriesKindRange, "msg/s"},
		{domain.MetricBacklogOldestAge, domain.SeriesKindInstant, "seconds"},
		{domain.MetricPushPendingCount, domain.SeriesKindInstant, "count"},
		{domain.MetricOutboxFailed24h, domain.SeriesKindInstant, "count"},
	}

	for _, w := range want {
		e, ok := byKey[w.key]
		if !ok {
			t.Fatalf("dashboard catalog missing %q", w.key)
		}
		if e.Kind != w.kind {
			t.Fatalf("%q kind=%q, want %q", w.key, e.Kind, w.kind)
		}
		if e.Unit != w.unit {
			t.Fatalf("%q unit=%q, want %q", w.key, e.Unit, w.unit)
		}
		if e.Title == "" {
			t.Fatalf("%q missing title", w.key)
		}
		if e.Description == "" {
			t.Fatalf("%q missing description", w.key)
		}
	}
}

func TestGateway_Snapshot_InstantKeyStaysInstantWithWindowAndStep(t *testing.T) {
	body := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{"job":"alt-backend"},"value":[1700000000,"1"]}]}}`
	var paths []string
	srv := newProm(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path != "/api/v1/query" {
			http.Error(w, "expected instant query endpoint /api/v1/query", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	g := New(Config{Client: newClient(t, srv.URL), CacheTTL: time.Second, RateLimit: 10})
	snap, err := g.Snapshot(context.Background(), []domain.MetricKey{domain.MetricAvailability}, domain.RangeWindow15m, domain.Step30s)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(paths) != 1 || paths[0] != "/api/v1/query" {
		t.Fatalf("expected [/api/v1/query], got %v", paths)
	}
	mr := snap.Metrics[domain.MetricAvailability]
	if mr == nil {
		t.Fatalf("missing metric result for %q", domain.MetricAvailability)
	}
	if mr.Degraded {
		t.Fatalf("unexpected degraded=true (reason=%q)", mr.Reason)
	}
	if mr.Kind != domain.SeriesKindInstant {
		t.Fatalf("kind=%q, want %q", mr.Kind, domain.SeriesKindInstant)
	}
	if len(mr.Series) != 1 || len(mr.Series[0].Points) != 1 {
		t.Fatalf("unexpected series points: %+v", mr.Series)
	}
}

func TestGateway_Healthy_DelegatesToClient(t *testing.T) {
	srv := newProm(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/-/ready" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	g := New(Config{Client: newClient(t, srv.URL), CacheTTL: time.Second, RateLimit: 10})
	if err := g.Healthy(context.Background()); err != nil {
		t.Fatalf("Healthy: %v", err)
	}
}

func TestGateway_Snapshot_RateLimits(t *testing.T) {
	body := `{"status":"success","data":{"resultType":"vector","result":[]}}`
	var seen atomic.Int32
	srv := newProm(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Add(1)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	g := New(Config{Client: newClient(t, srv.URL), CacheTTL: 0, RateLimit: 1, RateLimitBurst: 1})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	for i := 0; i < 5; i++ {
		_, _ = g.Snapshot(ctx, []domain.MetricKey{domain.MetricAvailability}, "", "")
	}
	// With rate=1rps and burst=1 over ~200ms we expect at most 2 upstream requests.
	if got := seen.Load(); got > 2 {
		t.Fatalf("rate limit not enforced; upstream seen=%d", got)
	}
}
