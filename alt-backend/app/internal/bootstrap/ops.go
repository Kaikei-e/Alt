package bootstrap

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"alt/config"
)

// This file owns the one listener all three alt-backend binaries open.
//
// Before the split each process had its own answer: cmd/backend served
// /metrics off the browser-facing REST port, cmd/harvester opened
// HARVESTER_HEALTH_ADDR (:9103) and cmd/datahub DATAHUB_HEALTH_ADDR (:9104) —
// while compose, observability/prometheus/prometheus.yml and the Hurl suites
// all addressed a single :9110. Three private ports and one shared expectation
// is a stack where every container reports unhealthy and every scrape target
// is down. OPS_LISTEN is the single knob that replaced them.
//
// The surface is deliberately two routes. /health is what the healthcheck
// subcommand and compose probe read; /metrics is what Prometheus scrapes.
// Neither authenticates a caller, and neither is allowed to become a door into
// anything that would need to — see e2e/hurl/alt-harvester/
// 01-operator-surface-only.hurl and e2e/hurl/alt-data-hub/03-ops-listener.hurl,
// which assert the 404s from outside the process.

// NewOpsHandler builds the ops surface: /health and /metrics, nothing else.
//
// The mux is explicit and never http.DefaultServeMux — that is where
// net/http/pprof registers itself via init(), so reusing it would publish heap
// and goroutine dumps from the monitoring port of any binary that ever linked
// alt/internal/profiling.
func NewOpsHandler(serviceName string, metrics http.Handler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "healthy",
			"service": serviceName,
		})
	})

	if metrics != nil {
		mux.Handle("/metrics", metrics)
	} else {
		// OTel produced no Prometheus exporter. A 404 here would be
		// indistinguishable from "this binary has no metrics surface", and the
		// scrape target would simply read as down with no way to tell which.
		// 503 says the route exists and its dependency does not — the visible
		// half of CLAUDE.md rule 8, whose other half is the wiring log below.
		mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("metrics exporter is not wired: OpenTelemetry initialisation " +
				"returned no Prometheus handler (check OTEL_ENABLED)\n"))
		})
	}

	return mux
}

// NewOpsServer builds the ops listener with the shared service-listener
// timeouts. Split from NewOpsHandler so a test can exercise the routing table
// without binding a port.
func NewOpsServer(addr string, h http.Handler, cfg *config.Config) *http.Server {
	return NewServiceServer(addr, h, cfg)
}

// LogOpsWiring states the ops listener's bind, reach and metrics availability
// at startup. Compose files and Prometheus targets both encode assumptions
// about this port; this is the process saying what it actually did.
func LogOpsWiring(ctx context.Context, log *slog.Logger, addr string, metricsEnabled bool) {
	log.InfoContext(ctx, "ops_listener.wiring",
		"addr", addr,
		"reach", string(config.ListenAddrReach(addr)),
		"surfaces", "/health,/metrics",
		"auth", "none",
		"metrics_enabled", metricsEnabled,
	)
}
