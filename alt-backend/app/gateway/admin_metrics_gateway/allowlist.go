package admin_metrics_gateway

import "alt/domain"

// allowEntry binds a MetricKey to its server-rendered PromQL and metadata.
// PromQL is a compile-time literal: clients never send query strings.
type allowEntry struct {
	key         domain.MetricKey
	title       string
	unit        string
	description string
	kind        domain.SeriesKind // default shape (instant or range)
	promql      string
	grafanaURL  string
}

// allowlist reflects metric names actually emitted by the Alt stack's
// scrape targets as of 2026-04-13. See
// docs/runbooks/admin-observability.md for the target ↔ metric inventory.
var allowlist = []allowEntry{
	{
		key:         domain.MetricAvailability,
		title:       "Service availability",
		unit:        "bool",
		description: "up{} for each scraped service.",
		kind:        domain.SeriesKindInstant,
		promql:      `up{job=~"alt-backend|mq-hub|recap-worker|recap-subworker|cadvisor|nginx|prometheus"}`,
		grafanaURL:  "/d/otel-overview",
	},
	{
		key:         domain.MetricHTTPLatencyP95,
		title:       "HTTP latency p95",
		unit:        "seconds",
		description: "Histogram p95 of request duration by job (prometheus/client_golang default name).",
		kind:        domain.SeriesKindRange,
		promql:      `histogram_quantile(0.95, sum by (job, le) (rate(http_request_duration_seconds_bucket[5m])))`,
		grafanaURL:  "/d/golden-signals",
	},
	{
		key:         domain.MetricHTTPRPS,
		title:       "HTTP requests per second",
		unit:        "req/s",
		description: "Request rate by job, 1m window.",
		kind:        domain.SeriesKindRange,
		promql:      `sum by (job) (rate(http_requests_total[1m]))`,
		grafanaURL:  "/d/golden-signals",
	},
	{
		key:         domain.MetricHTTPErrorRatio,
		title:       "HTTP 5xx ratio",
		unit:        "ratio",
		description: "5xx requests over total, 5m window.",
		kind:        domain.SeriesKindRange,
		promql:      `sum by (job) (rate(http_requests_total{status=~"5.."}[5m])) / clamp_min(sum by (job) (rate(http_requests_total[5m])), 1e-9)`,
		grafanaURL:  "/d/golden-signals",
	},
	{
		key:         domain.MetricCPUSaturation,
		title:       "CPU saturation",
		unit:        "cores",
		description: "Container CPU usage rate by name, 2m window.",
		kind:        domain.SeriesKindRange,
		promql:      `sum by (name) (rate(container_cpu_usage_seconds_total{name=~".+"}[2m]))`,
		grafanaURL:  "/d/otel-overview",
	},
	{
		key:         domain.MetricMemoryRSS,
		title:       "Memory RSS",
		unit:        "bytes",
		description: "Container RSS memory by name.",
		kind:        domain.SeriesKindInstant,
		promql:      `sum by (name) (container_memory_rss{name=~".+"})`,
		grafanaURL:  "/d/otel-overview",
	},
	{
		key:         domain.MetricMQHubPublishRate,
		title:       "mq-hub publish rate",
		unit:        "msg/s",
		description: "Publish rate to mq-hub by topic, 1m window.",
		kind:        domain.SeriesKindRange,
		promql:      `sum by (topic) (rate(mqhub_publish_total[1m]))`,
		grafanaURL:  "/d/otel-overview",
	},
	{
		key:         domain.MetricMQHubRedis,
		title:       "mq-hub Redis connection",
		unit:        "bool",
		description: "mq-hub Redis connection status (1=connected).",
		kind:        domain.SeriesKindInstant,
		promql:      `mqhub_redis_connection_status`,
		grafanaURL:  "/d/otel-overview",
	},
	{
		key:         domain.MetricRecapDBPoolInUse,
		title:       "recap DB pool in-use",
		unit:        "conns",
		description: "Recap worker DB connections currently checked out.",
		kind:        domain.SeriesKindInstant,
		promql:      `sum by (pool) (recap_db_pool_checked_out)`,
		grafanaURL:  "/d/otel-overview",
	},
	{
		key:         domain.MetricRecapWorkerRSS,
		title:       "recap-worker RSS",
		unit:        "bytes",
		description: "Resident memory of recap-worker process.",
		kind:        domain.SeriesKindInstant,
		promql:      `recap_worker_rss_bytes`,
		grafanaURL:  "/d/otel-overview",
	},
	{
		key:         domain.MetricRecapRequestP95,
		title:       "recap request p95",
		unit:        "seconds",
		description: "recap-worker request processing p95, 5m window.",
		kind:        domain.SeriesKindRange,
		promql:      `histogram_quantile(0.95, sum by (le) (rate(recap_request_process_seconds_bucket[5m])))`,
		grafanaURL:  "/d/otel-overview",
	},
	{
		key:         domain.MetricRecapSubworkerAdminSuccess,
		title:       "recap-subworker admin jobs",
		unit:        "jobs/s",
		description: "Admin job status rate on recap-subworker, 5m window.",
		kind:        domain.SeriesKindRange,
		promql:      `sum by (status) (rate(recap_subworker_admin_job_status_total[5m]))`,
		grafanaURL:  "/d/otel-overview",
	},
}

func lookup(key domain.MetricKey) (allowEntry, bool) {
	for _, e := range allowlist {
		if e.key == key {
			return e, true
		}
	}
	return allowEntry{}, false
}

func catalogEntries() []domain.MetricCatalogEntry {
	out := make([]domain.MetricCatalogEntry, 0, len(allowlist))
	for _, e := range allowlist {
		out = append(out, domain.MetricCatalogEntry{
			Key:         e.key,
			Title:       e.title,
			Unit:        e.unit,
			Description: e.description,
			GrafanaURL:  e.grafanaURL,
			Kind:        e.kind,
		})
	}
	return out
}
