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

// allowlist reflects metric names actually emitted by the Alt stack's scrape
// targets. It is not self-validating: only the availability entry's job matcher
// is pinned to observability/prometheus/prometheus.yml by a test, so a metric
// renamed upstream still goes silently empty here. See
// docs/runbooks/admin-observability.md for the target ↔ metric inventory.
var allowlist = []allowEntry{
	{
		key:         domain.MetricAvailability,
		title:       "Service availability",
		unit:        "bool",
		description: "up{} for each scraped service.",
		kind:        domain.SeriesKindInstant,
		// Job list mirrors the active scrape_configs in
		// observability/prometheus/prometheus.yml, in declaration order. A job
		// listed here but not scraped can never return a sample; a scraped job
		// missing here is dropped from the tile entirely, so a fully-down target
		// looks identical to a healthy stack. Both directions are guarded by
		// TestAllowlist_AvailabilityJobsMatchPrometheusScrapeConfig.
		//
		// `min by (job)` collapses multi-target jobs to their worst target.
		// PKI ops jobs are one :9110 target each (`*-ops` when an app-port job
		// already exists). They are listed by exact job_name — app-port jobs
		// are not substitutes for ops :9110, and pki-agent is gone.
		//
		// alt-harvester, alt-data-hub and alt-notifier are the binaries split out
		// of alt-backend. They are listed as separate jobs, not folded into
		// alt-backend's, precisely because `min by (job)` would then hide
		// which of the processes is down.
		//
		// pre-processor's collector listener carries the notification-outbox
		// relay gauges. Its `up` tracks the listener rather than the relay loop —
		// the relay's own liveness is the last-tick gauge — but omitting a
		// scraped job here leaves the admin monitor with no row at all for it,
		// which is the failure this list exists to prevent.
		//
		// acolyte-orchestrator has no relay job: its API listener authenticates
		// nobody, so a /metrics route on it would be a new unauthenticated
		// surface, and a topology test asserts the route's absence.
		//
		// alertmanager sits next to prometheus because the notifier's own
		// death is otherwise the least visible outage in the stack: alerts
		// keep evaluating and stop being delivered, which from inside looks
		// like a quiet night.
		promql:     `min by (job) (up{job=~"prometheus|alertmanager|plecto-proxy|cadvisor|mq-hub|pre-processor|recap-worker|recap-subworker|news-creator|alt-backend|alt-harvester|alt-notifier|alt-data-hub|alt-butterfly-facade-ops|auth-hub|search-indexer|rag-orchestrator-ops|pre-processor-ops|tag-generator|recap-worker-ops|acolyte-orchestrator|recap-subworker-ops|news-creator-ops|knowledge-sovereign-ops|recap-evaluator|knowledge-sovereign|rag-orchestrator|alt-butterfly-facade"})`,
		grafanaURL: "/d/otel-overview",
	},
	{
		key:         domain.MetricHTTPLatencyP50,
		title:       "HTTP latency p50",
		unit:        "seconds",
		description: "Histogram p50 of request duration by job and signal, 5m window.",
		kind:        domain.SeriesKindRange,
		promql:      `job:alt_request_duration_seconds:p50_rate5m`,
		grafanaURL:  "/d/golden-signals",
	},
	{
		key:         domain.MetricHTTPLatencyP95,
		title:       "HTTP latency p95",
		unit:        "seconds",
		description: "Histogram p95 of request duration by job and signal, 5m window.",
		kind:        domain.SeriesKindRange,
		promql:      `job:alt_request_duration_seconds:p95_rate5m`,
		grafanaURL:  "/d/golden-signals",
	},
	{
		key:         domain.MetricHTTPLatencyP99,
		title:       "HTTP latency p99",
		unit:        "seconds",
		description: "Histogram p99 of request duration by job and signal, 5m window.",
		kind:        domain.SeriesKindRange,
		promql:      `job:alt_request_duration_seconds:p99_rate5m`,
		grafanaURL:  "/d/golden-signals",
	},
	{
		key:         domain.MetricHTTPRPS,
		title:       "HTTP requests per second",
		unit:        "req/s",
		description: "Request rate by job and signal, 5m window.",
		kind:        domain.SeriesKindRange,
		promql:      `job:alt_requests:rate5m`,
		grafanaURL:  "/d/golden-signals",
	},
	{
		key:         domain.MetricHTTPErrorRatio,
		title:       "HTTP 5xx ratio",
		unit:        "ratio",
		description: "Error requests over total by job and signal, 5m window.",
		kind:        domain.SeriesKindRange,
		promql:      `job:alt_request_error_ratio:ratio_rate5m`,
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
		promql:      `sum by (topic) (label_replace(rate(mqhub_publish_total[1m]), "topic", "$1", "stream", "(.*)"))`,
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
	{
		key:         domain.MetricPrometheusScrapeLag,
		title:       "Prometheus scrape lag",
		unit:        "seconds",
		description: "time() - last scrape timestamp per job. Surfaces stalled exporters before metrics go silently stale.",
		kind:        domain.SeriesKindInstant,
		promql:      `time() - max by (job) (timestamp(up))`,
		grafanaURL:  "/d/otel-overview",
	},
	{
		key:         domain.MetricAvailabilityBurn1h,
		title:       "Availability burn rate (1h)",
		unit:        "ratio",
		description: "Error rate over the past 1h divided by the 0.1% SLO error budget. Page-tier 1 fires at 14.4.",
		kind:        domain.SeriesKindRange,
		promql:      `alt_edge:error_budget_burn:ratio_rate1h`,
		grafanaURL:  "/d/golden-signals",
	},
	{
		key:         domain.MetricAvailabilityBurn6h,
		title:       "Availability burn rate (6h)",
		unit:        "ratio",
		description: "Error rate over the past 6h divided by the 0.1% SLO error budget. Page-tier 2 fires at 6.",
		kind:        domain.SeriesKindRange,
		promql:      `alt_edge:error_budget_burn:ratio_rate6h`,
		grafanaURL:  "/d/golden-signals",
	},
	{
		key:         domain.MetricEdgeRPS,
		title:       "Edge requests per second",
		unit:        "req/s",
		description: "Aggregate plecto-proxy request rate, 5m window.",
		kind:        domain.SeriesKindRange,
		promql:      `alt_edge:requests:rate5m`,
		grafanaURL:  "/d/golden-signals",
	},
	{
		key:         domain.MetricEdgeErrorRatio,
		title:       "Edge 5xx ratio",
		unit:        "ratio",
		description: "Aggregate plecto-proxy 5xx ratio over total requests, 5m window.",
		kind:        domain.SeriesKindRange,
		promql:      `alt_edge:request_error_ratio:ratio_rate5m`,
		grafanaURL:  "/d/golden-signals",
	},
	{
		key:         domain.MetricEdgeLatencyP95,
		title:       "Edge latency p95",
		unit:        "seconds",
		description: "Aggregate plecto-proxy request duration p95, 5m window.",
		kind:        domain.SeriesKindRange,
		promql:      `alt_edge:request_duration_seconds:p95_rate5m`,
		grafanaURL:  "/d/golden-signals",
	},
	{
		key:         domain.MetricEdgeBurn5m,
		title:       "Edge availability burn rate (5m)",
		unit:        "ratio",
		description: "Edge error rate over the past 5m divided by the 0.1% SLO error budget.",
		kind:        domain.SeriesKindRange,
		promql:      `alt_edge:error_budget_burn:ratio_rate5m`,
		grafanaURL:  "/d/golden-signals",
	},
	{
		key:         domain.MetricEdgeBurn30m,
		title:       "Edge availability burn rate (30m)",
		unit:        "ratio",
		description: "Edge error rate over the past 30m divided by the 0.1% SLO error budget.",
		kind:        domain.SeriesKindRange,
		promql:      `alt_edge:error_budget_burn:ratio_rate30m`,
		grafanaURL:  "/d/golden-signals",
	},
	{
		key:         domain.MetricHostCPUUtilization,
		title:       "Host CPU utilization",
		unit:        "ratio",
		description: "Root cgroup CPU usage rate divided by machine CPU cores, 2m window.",
		kind:        domain.SeriesKindRange,
		promql:      `alt_host:cpu_utilization:ratio_rate2m`,
		grafanaURL:  "/d/otel-overview",
	},
	{
		key:         domain.MetricHostMemoryUtilization,
		title:       "Host memory utilization",
		unit:        "ratio",
		description: "Root cgroup working set memory divided by machine memory.",
		kind:        domain.SeriesKindRange,
		promql:      `alt_host:memory_utilization:ratio`,
		grafanaURL:  "/d/otel-overview",
	},
	{
		key:         domain.MetricHostFilesystemUtilization,
		title:       "Host filesystem utilization",
		unit:        "ratio",
		description: "Root cgroup filesystem usage ratio by real block device.",
		kind:        domain.SeriesKindInstant,
		promql:      `alt_host:filesystem_utilization:ratio`,
		grafanaURL:  "/d/otel-overview",
	},
	{
		key:         domain.MetricAlertsFiring,
		title:       "Firing alerts",
		unit:        "bool",
		description: "Currently firing Prometheus alerts.",
		kind:        domain.SeriesKindInstant,
		promql:      `ALERTS{alertstate="firing"}`,
		grafanaURL:  "/d/otel-overview",
	},
	{
		key:         domain.MetricAlertsPending,
		title:       "Pending alerts",
		unit:        "bool",
		description: "Currently pending Prometheus alerts.",
		kind:        domain.SeriesKindInstant,
		promql:      `ALERTS{alertstate="pending"}`,
		grafanaURL:  "/d/otel-overview",
	},
	{
		key:         domain.MetricAlertsActiveSince,
		title:       "Alert active since",
		unit:        "unix_seconds",
		description: "Unix timestamp when each active alert entered its current state.",
		kind:        domain.SeriesKindInstant,
		promql:      `ALERTS_FOR_STATE`,
		grafanaURL:  "/d/otel-overview",
	},
	{
		key:         domain.MetricCertRemaining,
		title:       "Certificate remaining lifetime",
		unit:        "seconds",
		description: "Minimum mTLS leaf certificate remaining lifetime per job.",
		kind:        domain.SeriesKindInstant,
		promql:      `min by (job) (pki_enrollment_cert_remaining_seconds)`,
		grafanaURL:  "/d/otel-overview",
	},
	{
		key:         domain.MetricCertHealthy,
		title:       "Certificate enrollment health",
		unit:        "bool",
		description: "Minimum PKI enrollment health status per job (1=healthy).",
		kind:        domain.SeriesKindInstant,
		promql:      `min by (job) (pki_enrollment_healthy)`,
		grafanaURL:  "/d/otel-overview",
	},
	{
		key:         domain.MetricScrapeDuration,
		title:       "Scrape duration",
		unit:        "seconds",
		description: "Maximum Prometheus scrape duration per job.",
		kind:        domain.SeriesKindInstant,
		promql:      `max by (job) (scrape_duration_seconds)`,
		grafanaURL:  "/d/otel-overview",
	},
	{
		key:         domain.MetricBacklogOldestAge,
		title:       "Backlog oldest pending age",
		unit:        "seconds",
		description: "Oldest pending item age by job and queue across notification outbox and push delivery.",
		kind:        domain.SeriesKindInstant,
		promql:      `max by (job, queue) (label_replace(notification_outbox_oldest_pending_age_seconds, "queue", "notification_outbox", "", "") or label_replace(push_delivery_oldest_pending_age_seconds, "queue", "push_delivery", "", ""))`,
		grafanaURL:  "/d/otel-overview",
	},
	{
		key:         domain.MetricPushPendingCount,
		title:       "Push delivery pending count",
		unit:        "count",
		description: "Pending push deliveries per job.",
		kind:        domain.SeriesKindInstant,
		promql:      `sum by (job) (push_delivery_pending_count)`,
		grafanaURL:  "/d/otel-overview",
	},
	{
		key:         domain.MetricOutboxFailed24h,
		title:       "Outbox failed events (24h)",
		unit:        "count",
		description: "Harvester outbox events failed over the past 24h per job.",
		kind:        domain.SeriesKindInstant,
		promql:      `sum by (job) (increase(alt_harvester_outbox_events_failed_total[24h]))`,
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
