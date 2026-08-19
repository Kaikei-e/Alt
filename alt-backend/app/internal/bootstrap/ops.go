package bootstrap

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

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
// Cheap /health is what the healthcheck subcommand and compose probe read;
// /metrics is what Prometheus scrapes. /health/deep is optional (cmd/backend
// wires it) and is never a compose probe — see docs/runbooks/health-deep-contract.md.
// Neither cheap route authenticates a caller. Deep health lives here because
// the public REST DoS whitelist must not include it.
//
// The mux is explicit and never http.DefaultServeMux — that is where
// net/http/pprof registers itself via init(), so reusing it would publish heap
// and goroutine dumps from the monitoring port of any binary that ever linked
// alt/internal/profiling.

// OpsOption customises NewOpsHandler.
type OpsOption func(*opsOptions)

type opsOptions struct {
	deep http.Handler
}

// WithDeepHealth mounts GET /health/deep on the ops listener. Pass nil to
// leave the route absent (404), which is what harvester and data-hub do.
func WithDeepHealth(h http.Handler) OpsOption {
	return func(o *opsOptions) { o.deep = h }
}

// NewOpsHandler builds the ops surface: /health and /metrics, plus /health/deep
// when WithDeepHealth is given.
func NewOpsHandler(serviceName string, metrics http.Handler, opts ...OpsOption) http.Handler {
	var o opsOptions
	for _, opt := range opts {
		opt(&o)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "healthy",
			"service": serviceName,
		})
	})

	if o.deep != nil {
		mux.Handle("/health/deep", o.deep)
	}

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
func LogOpsWiring(ctx context.Context, log *slog.Logger, addr string, metricsEnabled bool, extraSurfaces ...string) {
	surfaces := "/health,/metrics"
	if len(extraSurfaces) > 0 {
		surfaces = surfaces + "," + strings.Join(extraSurfaces, ",")
	}
	log.InfoContext(ctx, "ops_listener.wiring",
		"addr", addr,
		"reach", string(config.ListenAddrReach(addr)),
		"surfaces", surfaces,
		"auth", "none",
		"metrics_enabled", metricsEnabled,
	)
}
