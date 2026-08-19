package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"search-indexer/healthdeep"
)

func TestOpsMetricsHandlerIncludesDeepHealth(t *testing.T) {
	r := healthdeep.NewRunner(healthdeep.Config{
		Service: "search-indexer-ops-metrics",
		Checks: []healthdeep.Check{{
			Name:     "meilisearch",
			Critical: true,
			Probe:    func(context.Context) error { return nil },
		}},
		CacheTTL: healthdeep.DefaultCacheTTL,
	})
	_ = r.Run(context.Background())

	rec := httptest.NewRecorder()
	opsMetricsHandler(nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "health_deep_status") {
		t.Fatalf("ops scrape missing health_deep_status:\n%s", body)
	}
	if !strings.Contains(body, "health_deep_latency_seconds") {
		t.Fatalf("ops scrape missing health_deep_latency_seconds:\n%s", body)
	}
}
