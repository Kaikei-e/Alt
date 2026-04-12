package admin_metrics_gateway

import (
	"alt/domain"
	"alt/driver/prometheus_client"
	"context"
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
	if !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("error should mention unknown: %v", err)
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
	if mr.Reason == "" {
		t.Fatalf("expected degraded reason")
	}
}

func TestGateway_Snapshot_RejectsUnknownWindowStep(t *testing.T) {
	body := `{"status":"success","data":{"resultType":"matrix","result":[]}}`
	srv := newProm(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	g := New(Config{Client: newClient(t, srv.URL), CacheTTL: time.Second, RateLimit: 10})

	if _, err := g.Snapshot(context.Background(), []domain.MetricKey{domain.MetricHTTPLatencyP95}, "99d", "15s"); err == nil {
		t.Fatalf("expected error for bogus window")
	}
	if _, err := g.Snapshot(context.Background(), []domain.MetricKey{domain.MetricHTTPLatencyP95}, "5m", "7s"); err == nil {
		t.Fatalf("expected error for bogus step")
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
		domain.MetricHTTPLatencyP95,
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
