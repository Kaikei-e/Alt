package bootstrap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// stubMetrics stands in for the Prometheus handler otel hands back.
func stubMetrics() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("# HELP go_goroutines Number of goroutines.\n"))
	})
}

// The ops listener is the one surface all three binaries share, and its whole
// value is that it carries nothing else. e2e/hurl/alt-harvester/01-operator-
// surface-only.hurl and e2e/hurl/alt-data-hub/03-ops-listener.hurl assert the
// same table from outside the process; this is the in-process copy that fails
// first.
func TestNewOpsHandler(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{name: "health", method: http.MethodGet, path: "/health", wantStatus: http.StatusOK},
		{name: "metrics", method: http.MethodGet, path: "/metrics", wantStatus: http.StatusOK},
		{name: "public health route is not here", method: http.MethodGet, path: "/v1/health", wantStatus: http.StatusNotFound},
		{name: "internal REST is not here", method: http.MethodGet, path: "/v1/internal/system-user", wantStatus: http.StatusNotFound},
		{
			name:       "BackendInternalService is not here",
			method:     http.MethodPost,
			path:       "/services.backend.v1.BackendInternalService/CreateArticle",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "the admin Connect surface is not here",
			method:     http.MethodPost,
			path:       "/alt.knowledge_home.v1.KnowledgeHomeAdminService/GetProjectionHealth",
			wantStatus: http.StatusNotFound,
		},
		{name: "root", method: http.MethodGet, path: "/", wantStatus: http.StatusNotFound},
	}

	h := NewOpsHandler("alt-harvester", stubMetrics())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
			if rec.Code != tt.wantStatus {
				t.Errorf("%s %s = %d, want %d", tt.method, tt.path, rec.Code, tt.wantStatus)
			}
		})
	}
}

// The compose probe and both Hurl suites read these exact two fields.
func TestNewOpsHandlerHealthBody(t *testing.T) {
	h := NewOpsHandler("alt-data-hub", stubMetrics())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("health body is not JSON: %v (%q)", err, rec.Body.String())
	}
	if body["status"] != "healthy" {
		t.Errorf("status = %q, want healthy", body["status"])
	}
	if body["service"] != "alt-data-hub" {
		t.Errorf("service = %q, want alt-data-hub", body["service"])
	}
}

// A nil metrics handler means OTel initialisation did not produce a Prometheus
// exporter. Serving 404 there would be indistinguishable from "this binary has
// no metrics surface", and Prometheus would report the target down with no way
// to tell which. 503 says the route exists and its dependency is missing —
// the audible half of CLAUDE.md rule 8, whose other half is the
// ops_listener.wiring log the binaries emit.
func TestNewOpsHandlerWithoutMetrics(t *testing.T) {
	h := NewOpsHandler("alt-backend", nil)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("GET /metrics without an exporter = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	// /health must still answer: the container is running, only the exporter
	// is absent, and failing the probe would restart-loop a healthy process.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("GET /health without an exporter = %d, want 200", rec.Code)
	}
}

// net/http/pprof registers itself on http.DefaultServeMux via init(). If the
// ops mux were that mux, any binary that ever linked the profiling package
// would publish heap and goroutine dumps from the monitoring port.
func TestNewOpsHandlerIsNotDefaultServeMux(t *testing.T) {
	if h := NewOpsHandler("alt-backend", stubMetrics()); h == http.Handler(http.DefaultServeMux) {
		t.Fatal("ops handler is http.DefaultServeMux; pprof would be served from the ops listener")
	}
}
