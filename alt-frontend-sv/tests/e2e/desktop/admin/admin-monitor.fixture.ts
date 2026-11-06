import type { Route } from "@playwright/test";

export function encodeConnectStream(messages: unknown[]): Buffer {
	const buffers: Buffer[] = [];
	for (const msg of messages) {
		const payload = Buffer.from(JSON.stringify(msg), "utf-8");
		const frame = Buffer.alloc(5 + payload.length);
		frame[0] = 0x00;
		frame.writeUInt32BE(payload.length, 1);
		payload.copy(frame, 5);
		buffers.push(frame);
	}
	const trailerPayload = Buffer.from("{}", "utf-8");
	const trailer = Buffer.alloc(5 + trailerPayload.length);
	trailer[0] = 0x02;
	trailer.writeUInt32BE(trailerPayload.length, 1);
	trailerPayload.copy(trailer, 5);
	buffers.push(trailer);
	return Buffer.concat(buffers);
}

export async function fulfillWatchStream(
	route: Route,
	messages: unknown[],
): Promise<void> {
	await route.fulfill({
		status: 200,
		contentType: "application/connect+json",
		headers: {
			"Connect-Content-Encoding": "identity",
			"Connect-Accept-Encoding": "identity",
		},
		body: encodeConnectStream(messages),
	});
}

function rangeSeries(
	labels: Record<string, string>,
	values: number[],
	endIso: string,
	stepMs = 30_000,
) {
	const endMs = Date.parse(endIso);
	const startMs = endMs - (values.length - 1) * stepMs;
	return {
		labels,
		points: values.map((value, idx) => ({
			time: new Date(startMs + idx * stepMs).toISOString(),
			value,
		})),
	};
}

function instantSeries(
	labels: Record<string, string>,
	value: number,
	iso: string,
) {
	return {
		labels,
		points: [{ time: iso, value }],
	};
}

export function buildAuditWatchResponse(snapshotIso?: string) {
	const iso = snapshotIso ?? new Date().toISOString();
	const snapSec = Math.floor(Date.parse(iso) / 1000);

	return {
		time: iso,
		metrics: [
			{
				key: "alerts_firing",
				kind: "SERIES_KIND_INSTANT",
				unit: "bool",
				series: [
					instantSeries(
						{
							alertname: "KnowledgeLoopRecapTopicSnapshotProducerDead",
							severity: "warning",
							job: "knowledge-sovereign",
							alertstate: "firing",
						},
						1,
						iso,
					),
					instantSeries(
						{
							alertname: "Watchdog",
							severity: "deadman",
							alertstate: "firing",
						},
						1,
						iso,
					),
				],
			},
			{
				key: "alerts_pending",
				kind: "SERIES_KIND_INSTANT",
				unit: "bool",
				series: [],
			},
			{
				key: "alerts_active_since",
				kind: "SERIES_KIND_INSTANT",
				unit: "unix_seconds",
				series: [
					instantSeries(
						{
							alertname: "KnowledgeLoopRecapTopicSnapshotProducerDead",
							severity: "warning",
							job: "knowledge-sovereign",
						},
						snapSec - 900,
						iso,
					),
					instantSeries(
						{
							alertname: "Watchdog",
							severity: "deadman",
						},
						snapSec - 86_400,
						iso,
					),
				],
			},
			{
				key: "edge_rps",
				kind: "SERIES_KIND_RANGE",
				unit: "req/s",
				series: [rangeSeries({}, [38.2, 40.1, 41.5, 42.8], iso)],
			},
			{
				key: "edge_error_ratio",
				kind: "SERIES_KIND_RANGE",
				unit: "ratio",
				series: [rangeSeries({}, [0.012, 0.015, 0.0175, 0.0187], iso)],
			},
			{
				key: "edge_latency_p95",
				kind: "SERIES_KIND_RANGE",
				unit: "seconds",
				series: [rangeSeries({}, [0.22, 0.25, 0.21, 0.24], iso)],
			},
			{
				key: "edge_burn_5m",
				kind: "SERIES_KIND_RANGE",
				unit: "ratio",
				series: [rangeSeries({}, [15.2, 17.8, 19.1, 20.0], iso)],
			},
			{
				key: "availability_burn_1h",
				kind: "SERIES_KIND_RANGE",
				unit: "ratio",
				series: [rangeSeries({}, [14.9, 16.4, 17.5, 18.7], iso)],
			},
			{
				key: "edge_burn_30m",
				kind: "SERIES_KIND_RANGE",
				unit: "ratio",
				series: [rangeSeries({}, [2.4, 2.7, 2.9, 3.0], iso)],
			},
			{
				key: "availability_burn_6h",
				kind: "SERIES_KIND_RANGE",
				unit: "ratio",
				series: [rangeSeries({}, [2.1, 2.3, 2.4, 2.5], iso)],
			},
			{
				key: "availability_services",
				kind: "SERIES_KIND_INSTANT",
				unit: "bool",
				series: [
					instantSeries({ job: "plecto-proxy" }, 1, iso),
					instantSeries({ job: "alt-backend" }, 1, iso),
					instantSeries({ job: "knowledge-sovereign" }, 1, iso),
					instantSeries({ job: "news-creator" }, 1, iso),
					instantSeries({ job: "news-creator-ops" }, 0, iso),
					instantSeries({ job: "recap-evaluator" }, 1, iso),
					instantSeries({ job: "prometheus" }, 1, iso),
				],
			},
			{
				key: "prometheus_scrape_lag",
				kind: "SERIES_KIND_INSTANT",
				unit: "seconds",
				series: [
					instantSeries({ job: "plecto-proxy" }, 3, iso),
					instantSeries({ job: "alt-backend" }, 4, iso),
					instantSeries({ job: "knowledge-sovereign" }, 5, iso),
					instantSeries({ job: "news-creator" }, 6, iso),
					instantSeries({ job: "news-creator-ops" }, 45, iso),
					instantSeries({ job: "recap-evaluator" }, 4, iso),
					instantSeries({ job: "prometheus" }, 2, iso),
				],
			},
			{
				key: "scrape_duration",
				kind: "SERIES_KIND_INSTANT",
				unit: "seconds",
				series: [
					instantSeries({ job: "alt-backend" }, 0.012, iso),
					instantSeries({ job: "news-creator" }, 0.018, iso),
					instantSeries({ job: "recap-evaluator" }, 0.009, iso),
				],
			},
			{
				key: "http_rps",
				kind: "SERIES_KIND_RANGE",
				unit: "req/s",
				series: [
					rangeSeries(
						{ job: "plecto-proxy", signal: "http_server" },
						[38.2, 40.1, 41.5, 42.8],
						iso,
					),
					rangeSeries(
						{ job: "alt-backend", signal: "http_server" },
						[14.2, 15.6, 16.1, 15.8],
						iso,
					),
					rangeSeries(
						{ job: "news-creator", signal: "http_server" },
						[1.1, 1.4, 1.2, 1.3],
						iso,
					),
				],
			},
			{
				key: "http_error_ratio",
				kind: "SERIES_KIND_RANGE",
				unit: "ratio",
				series: [
					rangeSeries(
						{ job: "plecto-proxy", signal: "http_server" },
						[0.012, 0.015, 0.0175, 0.0187],
						iso,
					),
					rangeSeries(
						{ job: "alt-backend", signal: "http_server" },
						[0.0005, 0.0008, 0.0004, 0.0006],
						iso,
					),
					rangeSeries(
						{ job: "news-creator", signal: "http_server" },
						[0, 0, 0, 0],
						iso,
					),
				],
			},
			{
				key: "http_latency_p50",
				kind: "SERIES_KIND_RANGE",
				unit: "seconds",
				series: [
					rangeSeries(
						{ job: "plecto-proxy", signal: "http_server" },
						[0.03, 0.032, 0.029, 0.031],
						iso,
					),
					rangeSeries(
						{ job: "alt-backend", signal: "http_server" },
						[0.018, 0.02, 0.019, 0.021],
						iso,
					),
					rangeSeries(
						{ job: "news-creator", signal: "http_server" },
						[0.12, 0.14, 0.13, 0.15],
						iso,
					),
				],
			},
			{
				key: "http_latency_p95",
				kind: "SERIES_KIND_RANGE",
				unit: "seconds",
				series: [
					rangeSeries(
						{ job: "plecto-proxy", signal: "http_server" },
						[0.22, 0.25, 0.21, 0.24],
						iso,
					),
					rangeSeries(
						{ job: "alt-backend", signal: "http_server" },
						[0.085, 0.092, 0.088, 0.09],
						iso,
					),
					rangeSeries(
						{ job: "news-creator", signal: "http_server" },
						[0.42, 0.48, 0.45, 0.46],
						iso,
					),
				],
			},
			{
				key: "http_latency_p99",
				kind: "SERIES_KIND_RANGE",
				unit: "seconds",
				series: [
					rangeSeries(
						{ job: "plecto-proxy", signal: "http_server" },
						[0.48, 0.52, 0.49, 0.51],
						iso,
					),
					rangeSeries(
						{ job: "alt-backend", signal: "http_server" },
						[0.19, 0.21, 0.2, 0.22],
						iso,
					),
					rangeSeries(
						{ job: "news-creator", signal: "http_server" },
						[0.85, 0.91, 0.88, 0.89],
						iso,
					),
				],
			},
			{
				key: "host_cpu_utilization",
				kind: "SERIES_KIND_RANGE",
				unit: "ratio",
				series: [rangeSeries({}, [0.06, 0.08, 0.065, 0.07], iso)],
			},
			{
				key: "host_memory_utilization",
				kind: "SERIES_KIND_RANGE",
				unit: "ratio",
				series: [rangeSeries({}, [0.54, 0.55, 0.555, 0.56], iso)],
			},
			{
				key: "host_filesystem_utilization",
				kind: "SERIES_KIND_INSTANT",
				unit: "ratio",
				series: [instantSeries({ device: "/dev/nvme0n1p2" }, 0.75, iso)],
			},
			{
				key: "cert_remaining",
				kind: "SERIES_KIND_INSTANT",
				unit: "seconds",
				series: [
					instantSeries({ job: "auth-hub" }, 7_200, iso),
					instantSeries({ job: "alt-backend" }, 64_800, iso),
					instantSeries({ job: "knowledge-sovereign" }, 72_000, iso),
					instantSeries({ job: "mq-hub" }, 79_200, iso),
				],
			},
			{
				key: "cert_healthy",
				kind: "SERIES_KIND_INSTANT",
				unit: "bool",
				series: [
					instantSeries({ job: "auth-hub" }, 1, iso),
					instantSeries({ job: "alt-backend" }, 1, iso),
					instantSeries({ job: "knowledge-sovereign" }, 1, iso),
					instantSeries({ job: "mq-hub" }, 1, iso),
				],
			},
			{
				key: "mqhub_redis",
				kind: "SERIES_KIND_INSTANT",
				unit: "bool",
				series: [instantSeries({}, 1, iso)],
			},
			{
				key: "mqhub_publish_rate",
				kind: "SERIES_KIND_RANGE",
				unit: "msg/s",
				series: [
					rangeSeries(
						{ topic: "articles.ingested" },
						[3.8, 4.1, 4.0, 4.2],
						iso,
					),
				],
			},
			{
				key: "backlog_oldest_age",
				kind: "SERIES_KIND_INSTANT",
				unit: "seconds",
				series: [
					instantSeries(
						{ job: "pre-processor", queue: "notification_outbox" },
						780,
						iso,
					),
				],
			},
			{
				key: "push_pending_count",
				kind: "SERIES_KIND_INSTANT",
				unit: "count",
				series: [instantSeries({ job: "alt-notifier" }, 3, iso)],
			},
			{
				key: "outbox_failed_24h",
				kind: "SERIES_KIND_INSTANT",
				unit: "count",
				series: [instantSeries({ job: "alt-harvester" }, 0, iso)],
			},
		],
	};
}

const RECORD_BACKED_KEYS = new Set([
	"edge_rps",
	"edge_error_ratio",
	"edge_latency_p95",
	"edge_burn_5m",
	"edge_burn_30m",
	"availability_burn_1h",
	"availability_burn_6h",
	"http_rps",
	"http_error_ratio",
	"http_latency_p50",
	"http_latency_p95",
	"http_latency_p99",
	"host_cpu_utilization",
	"host_memory_utilization",
	"host_filesystem_utilization",
]);

function buildHealthyBaseWatchResponse(
	snapshotIso: string,
	recordPointIso: string,
) {
	const snapSec = Math.floor(Date.parse(snapshotIso) / 1000);
	return {
		time: snapshotIso,
		metrics: [
			{
				key: "alerts_firing",
				kind: "SERIES_KIND_INSTANT",
				unit: "bool",
				series: [
					instantSeries(
						{
							alertname: "Watchdog",
							severity: "deadman",
							alertstate: "firing",
						},
						1,
						snapshotIso,
					),
				],
			},
			{
				key: "alerts_pending",
				kind: "SERIES_KIND_INSTANT",
				unit: "bool",
				series: [],
			},
			{
				key: "alerts_active_since",
				kind: "SERIES_KIND_INSTANT",
				unit: "unix_seconds",
				series: [
					instantSeries(
						{
							alertname: "Watchdog",
							severity: "deadman",
						},
						snapSec - 86_400,
						snapshotIso,
					),
				],
			},
			{
				key: "edge_rps",
				kind: "SERIES_KIND_RANGE",
				unit: "req/s",
				series: [rangeSeries({}, [38.2, 40.1, 41.5, 42.8], recordPointIso)],
			},
			{
				key: "edge_error_ratio",
				kind: "SERIES_KIND_RANGE",
				unit: "ratio",
				series: [
					rangeSeries({}, [0.0001, 0.0002, 0.0001, 0.0002], recordPointIso),
				],
			},
			{
				key: "edge_latency_p95",
				kind: "SERIES_KIND_RANGE",
				unit: "seconds",
				series: [rangeSeries({}, [0.22, 0.25, 0.21, 0.24], recordPointIso)],
			},
			{
				key: "edge_burn_5m",
				kind: "SERIES_KIND_RANGE",
				unit: "ratio",
				series: [rangeSeries({}, [0.2, 0.3, 0.2, 0.2], recordPointIso)],
			},
			{
				key: "availability_burn_1h",
				kind: "SERIES_KIND_RANGE",
				unit: "ratio",
				series: [rangeSeries({}, [0.3, 0.3, 0.2, 0.2], recordPointIso)],
			},
			{
				key: "edge_burn_30m",
				kind: "SERIES_KIND_RANGE",
				unit: "ratio",
				series: [rangeSeries({}, [0.4, 0.3, 0.3, 0.2], recordPointIso)],
			},
			{
				key: "availability_burn_6h",
				kind: "SERIES_KIND_RANGE",
				unit: "ratio",
				series: [rangeSeries({}, [0.3, 0.3, 0.2, 0.2], recordPointIso)],
			},
			{
				key: "availability_services",
				kind: "SERIES_KIND_INSTANT",
				unit: "bool",
				series: [
					instantSeries({ job: "plecto-proxy" }, 1, snapshotIso),
					instantSeries({ job: "alt-backend" }, 1, snapshotIso),
					instantSeries({ job: "knowledge-sovereign" }, 1, snapshotIso),
					instantSeries({ job: "news-creator" }, 1, snapshotIso),
					instantSeries({ job: "news-creator-ops" }, 1, snapshotIso),
					instantSeries({ job: "recap-evaluator" }, 1, snapshotIso),
					instantSeries({ job: "prometheus" }, 1, snapshotIso),
				],
			},
			{
				key: "prometheus_scrape_lag",
				kind: "SERIES_KIND_INSTANT",
				unit: "seconds",
				series: [
					instantSeries({ job: "plecto-proxy" }, 3, snapshotIso),
					instantSeries({ job: "alt-backend" }, 4, snapshotIso),
					instantSeries({ job: "knowledge-sovereign" }, 5, snapshotIso),
					instantSeries({ job: "news-creator" }, 6, snapshotIso),
					instantSeries({ job: "news-creator-ops" }, 7, snapshotIso),
					instantSeries({ job: "recap-evaluator" }, 4, snapshotIso),
					instantSeries({ job: "prometheus" }, 2, snapshotIso),
				],
			},
			{
				key: "scrape_duration",
				kind: "SERIES_KIND_INSTANT",
				unit: "seconds",
				series: [
					instantSeries({ job: "alt-backend" }, 0.012, snapshotIso),
					instantSeries({ job: "news-creator" }, 0.018, snapshotIso),
					instantSeries({ job: "recap-evaluator" }, 0.009, snapshotIso),
				],
			},
			{
				key: "http_rps",
				kind: "SERIES_KIND_RANGE",
				unit: "req/s",
				series: [
					rangeSeries(
						{ job: "plecto-proxy", signal: "http_server" },
						[38.2, 40.1, 41.5, 42.8],
						recordPointIso,
					),
					rangeSeries(
						{ job: "alt-backend", signal: "http_server" },
						[14.2, 15.6, 16.1, 15.8],
						recordPointIso,
					),
					rangeSeries(
						{ job: "news-creator", signal: "http_server" },
						[1.1, 1.4, 1.2, 1.3],
						recordPointIso,
					),
				],
			},
			{
				key: "http_error_ratio",
				kind: "SERIES_KIND_RANGE",
				unit: "ratio",
				series: [
					rangeSeries(
						{ job: "plecto-proxy", signal: "http_server" },
						[0.0001, 0.0002, 0.0001, 0.0002],
						recordPointIso,
					),
					rangeSeries(
						{ job: "alt-backend", signal: "http_server" },
						[0.0005, 0.0008, 0.0004, 0.0006],
						recordPointIso,
					),
					rangeSeries(
						{ job: "news-creator", signal: "http_server" },
						[0, 0, 0, 0],
						recordPointIso,
					),
				],
			},
			{
				key: "http_latency_p50",
				kind: "SERIES_KIND_RANGE",
				unit: "seconds",
				series: [
					rangeSeries(
						{ job: "plecto-proxy", signal: "http_server" },
						[0.03, 0.032, 0.029, 0.031],
						recordPointIso,
					),
					rangeSeries(
						{ job: "alt-backend", signal: "http_server" },
						[0.018, 0.02, 0.019, 0.021],
						recordPointIso,
					),
					rangeSeries(
						{ job: "news-creator", signal: "http_server" },
						[0.12, 0.14, 0.13, 0.15],
						recordPointIso,
					),
				],
			},
			{
				key: "http_latency_p95",
				kind: "SERIES_KIND_RANGE",
				unit: "seconds",
				series: [
					rangeSeries(
						{ job: "plecto-proxy", signal: "http_server" },
						[0.22, 0.25, 0.21, 0.24],
						recordPointIso,
					),
					rangeSeries(
						{ job: "alt-backend", signal: "http_server" },
						[0.085, 0.092, 0.088, 0.09],
						recordPointIso,
					),
					rangeSeries(
						{ job: "news-creator", signal: "http_server" },
						[0.42, 0.48, 0.45, 0.46],
						recordPointIso,
					),
				],
			},
			{
				key: "http_latency_p99",
				kind: "SERIES_KIND_RANGE",
				unit: "seconds",
				series: [
					rangeSeries(
						{ job: "plecto-proxy", signal: "http_server" },
						[0.48, 0.52, 0.49, 0.51],
						recordPointIso,
					),
					rangeSeries(
						{ job: "alt-backend", signal: "http_server" },
						[0.19, 0.21, 0.2, 0.22],
						recordPointIso,
					),
					rangeSeries(
						{ job: "news-creator", signal: "http_server" },
						[0.85, 0.91, 0.88, 0.89],
						recordPointIso,
					),
				],
			},
			{
				key: "host_cpu_utilization",
				kind: "SERIES_KIND_RANGE",
				unit: "ratio",
				series: [rangeSeries({}, [0.06, 0.08, 0.065, 0.07], recordPointIso)],
			},
			{
				key: "host_memory_utilization",
				kind: "SERIES_KIND_RANGE",
				unit: "ratio",
				series: [rangeSeries({}, [0.54, 0.55, 0.555, 0.56], recordPointIso)],
			},
			{
				key: "host_filesystem_utilization",
				kind: "SERIES_KIND_INSTANT",
				unit: "ratio",
				series: [
					instantSeries({ device: "/dev/nvme0n1p2" }, 0.75, recordPointIso),
				],
			},
			{
				key: "cert_remaining",
				kind: "SERIES_KIND_INSTANT",
				unit: "seconds",
				series: [
					instantSeries({ job: "auth-hub" }, 61_200, snapshotIso),
					instantSeries({ job: "alt-backend" }, 64_800, snapshotIso),
					instantSeries({ job: "knowledge-sovereign" }, 72_000, snapshotIso),
					instantSeries({ job: "mq-hub" }, 79_200, snapshotIso),
				],
			},
			{
				key: "cert_healthy",
				kind: "SERIES_KIND_INSTANT",
				unit: "bool",
				series: [
					instantSeries({ job: "auth-hub" }, 1, snapshotIso),
					instantSeries({ job: "alt-backend" }, 1, snapshotIso),
					instantSeries({ job: "knowledge-sovereign" }, 1, snapshotIso),
					instantSeries({ job: "mq-hub" }, 1, snapshotIso),
				],
			},
			{
				key: "mqhub_redis",
				kind: "SERIES_KIND_INSTANT",
				unit: "bool",
				series: [instantSeries({}, 1, snapshotIso)],
			},
			{
				key: "mqhub_publish_rate",
				kind: "SERIES_KIND_RANGE",
				unit: "msg/s",
				series: [
					rangeSeries(
						{ topic: "articles.ingested" },
						[3.8, 4.1, 4.0, 4.2],
						snapshotIso,
					),
				],
			},
			{
				key: "backlog_oldest_age",
				kind: "SERIES_KIND_INSTANT",
				unit: "seconds",
				series: [
					instantSeries(
						{ job: "pre-processor", queue: "notification_outbox" },
						45,
						snapshotIso,
					),
				],
			},
			{
				key: "push_pending_count",
				kind: "SERIES_KIND_INSTANT",
				unit: "count",
				series: [instantSeries({ job: "alt-notifier" }, 0, snapshotIso)],
			},
			{
				key: "outbox_failed_24h",
				kind: "SERIES_KIND_INSTANT",
				unit: "count",
				series: [instantSeries({ job: "alt-harvester" }, 0, snapshotIso)],
			},
		],
	};
}

export function buildNoRulesWatchResponse(snapshotIso?: string) {
	const iso = snapshotIso ?? new Date().toISOString();
	const base = buildHealthyBaseWatchResponse(iso, iso);
	return {
		...base,
		metrics: base.metrics.map((m) =>
			RECORD_BACKED_KEYS.has(m.key) ? { ...m, series: [] } : m,
		),
	};
}

export function buildStaleWatchResponse(snapshotIso?: string) {
	const iso = snapshotIso ?? new Date().toISOString();
	const staleIso = new Date(Date.parse(iso) - 20 * 60 * 1000).toISOString();
	return buildHealthyBaseWatchResponse(iso, staleIso);
}
