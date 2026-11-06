import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import {
	type MetricResult,
	MetricResultSchema,
	PointSchema,
	RangeWindow,
	SeriesKind,
	SeriesSchema,
	Step,
} from "#lib/gen/alt/admin_monitor/v1/admin_monitor_pb.js";
import {
	alertRowSeverity,
	alertsAvailability,
	BURN,
	badge,
	buildAlertRows,
	buildAttention,
	buildCertRows,
	buildEdgeSignals,
	buildHostResources,
	buildPipelines,
	buildServiceRows,
	certsAvailability,
	clampStep,
	classify,
	edgeEmptyText,
	freshness,
	indexMetrics,
	latestValue,
	MONITOR_KEYS,
	maxPointAgeSeconds,
	optToStepEnum,
	STALE_AFTER_SECONDS,
	seriesPoints,
	stepEnumToOpt,
	THRESHOLDS,
	validSteps,
	windowEnumToOpt,
} from "./model";

function makeSeries(
	labels: Record<string, string>,
	points: Array<{ time: string; value: number }>,
) {
	return create(SeriesSchema, {
		labels,
		points: points.map((p) => create(PointSchema, p)),
	});
}

function makeMetric(
	key: string,
	series: ReturnType<typeof makeSeries>[],
	opts?: {
		kind?: SeriesKind;
		unit?: string;
		degraded?: boolean;
		reason?: string;
	},
): MetricResult {
	return create(MetricResultSchema, {
		key,
		kind: opts?.kind ?? SeriesKind.INSTANT,
		unit: opts?.unit ?? "",
		series,
		degraded: opts?.degraded ?? false,
		reason: opts?.reason ?? "",
	});
}

describe("badge", () => {
	it("maps all severities to their glyph and uppercase label", () => {
		expect(badge("ok")).toEqual({ severity: "ok", glyph: "●", label: "OK" });
		expect(badge("warn")).toEqual({
			severity: "warn",
			glyph: "▲",
			label: "WARN",
		});
		expect(badge("crit")).toEqual({
			severity: "crit",
			glyph: "■",
			label: "CRIT",
		});
		expect(badge("nodata")).toEqual({
			severity: "nodata",
			glyph: "○",
			label: "NO DATA",
		});
		expect(badge("stale")).toEqual({
			severity: "stale",
			glyph: "◌",
			label: "STALE",
		});
	});
});

describe("latestValue and seriesPoints", () => {
	it("never concatenates two series, orders by time, and skips NaN/Infinity", () => {
		const m = makeMetric(
			"http_rps",
			[
				makeSeries({ job: "alt-backend", signal: "http_server" }, [
					{ time: "2026-11-06T12:00:30Z", value: 12.5 },
					{ time: "2026-11-06T12:00:00Z", value: 10.0 },
					{ time: "2026-11-06T12:01:00Z", value: Number.NaN },
				]),
				makeSeries({ job: "plecto-proxy", signal: "http_server" }, [
					{ time: "2026-11-06T12:00:00Z", value: 99.0 },
					{ time: "2026-11-06T12:00:30Z", value: 100.0 },
				]),
			],
			{ kind: SeriesKind.RANGE, unit: "req/s" },
		);

		const pts = seriesPoints(m);
		expect(pts).toEqual([
			{ t: Date.parse("2026-11-06T12:00:00Z"), v: 10.0 },
			{ t: Date.parse("2026-11-06T12:00:30Z"), v: 12.5 },
		]);
		expect(latestValue(m)).toBe(12.5);

		const proxyPts = seriesPoints(m, { job: "plecto-proxy" });
		expect(proxyPts).toHaveLength(2);
		expect(latestValue(m, { job: "plecto-proxy" })).toBe(100.0);
		expect(latestValue(m, { job: "non-existent" })).toBeNull();
		expect(seriesPoints(undefined)).toEqual([]);
		expect(latestValue(undefined)).toBeNull();
	});

	it("returns null from latestValue when the newest point is older than maxAgeSeconds", () => {
		const m = makeMetric("http_rps", [
			makeSeries({ job: "alt-backend" }, [
				{ time: "2026-11-06T12:00:00Z", value: 12.5 },
			]),
		]);
		const asOfMs = Date.parse("2026-11-06T12:10:00Z");
		expect(maxPointAgeSeconds("30s")).toBe(300);
		expect(maxPointAgeSeconds("5m")).toBe(600);
		expect(
			latestValue(m, { job: "alt-backend" }, { asOfMs, maxAgeSeconds: 300 }),
		).toBeNull();
		expect(
			latestValue(m, { job: "alt-backend" }, { asOfMs, maxAgeSeconds: 700 }),
		).toBe(12.5);
	});
});

describe("classify", () => {
	it("maps null and non-finite values to nodata, never warn", () => {
		expect(classify(null, THRESHOLDS.edgeErrorRatio)).toBe("nodata");
		expect(classify(Number.NaN, THRESHOLDS.edgeErrorRatio)).toBe("nodata");
	});

	it("classifies direction=above thresholds", () => {
		expect(classify(0.0005, THRESHOLDS.edgeErrorRatio)).toBe("ok");
		expect(classify(0.001, THRESHOLDS.edgeErrorRatio)).toBe("warn");
		expect(classify(0.005, THRESHOLDS.edgeErrorRatio)).toBe("warn");
		expect(classify(0.0144, THRESHOLDS.edgeErrorRatio)).toBe("crit");
		expect(classify(0.05, THRESHOLDS.edgeErrorRatio)).toBe("crit");
	});

	it("classifies direction=below thresholds", () => {
		expect(classify(86400, THRESHOLDS.certRemaining)).toBe("ok");
		expect(classify(14400, THRESHOLDS.certRemaining)).toBe("warn");
		expect(classify(7200, THRESHOLDS.certRemaining)).toBe("warn");
		expect(classify(3600, THRESHOLDS.certRemaining)).toBe("crit");
		expect(classify(600, THRESHOLDS.certRemaining)).toBe("crit");
	});
});

describe("buildServiceRows", () => {
	it("merges <svc>-ops into <svc> with 2/2 targets and flags 1/2 down as crit with reason", () => {
		const upAll = makeMetric("availability_services", [
			makeSeries({ job: "news-creator" }, [
				{ time: "2026-11-06T12:00:00Z", value: 1 },
			]),
			makeSeries({ job: "news-creator-ops" }, [
				{ time: "2026-11-06T12:00:00Z", value: 1 },
			]),
		]);
		const rowsHealthy = buildServiceRows(indexMetrics([upAll]));
		expect(rowsHealthy).toHaveLength(1);
		expect(rowsHealthy[0]?.service).toBe("news-creator");
		expect(rowsHealthy[0]?.group).toBe("Workers");
		expect(rowsHealthy[0]?.jobs).toEqual(["news-creator", "news-creator-ops"]);
		expect(rowsHealthy[0]?.targetsUp).toBe(2);
		expect(rowsHealthy[0]?.targetsTotal).toBe(2);
		expect(rowsHealthy[0]?.severity).toBe("ok");

		const upOneDown = makeMetric("availability_services", [
			makeSeries({ job: "news-creator" }, [
				{ time: "2026-11-06T12:00:00Z", value: 1 },
			]),
			makeSeries({ job: "news-creator-ops" }, [
				{ time: "2026-11-06T12:00:00Z", value: 0 },
			]),
		]);
		const rowsDown = buildServiceRows(indexMetrics([upOneDown]));
		expect(rowsDown).toHaveLength(1);
		expect(rowsDown[0]?.targetsUp).toBe(1);
		expect(rowsDown[0]?.targetsTotal).toBe(2);
		expect(rowsDown[0]?.severity).toBe("crit");
		expect(rowsDown[0]?.reasons).toContain("1/2 targets down");
	});

	it("puts unknown jobs in Other and marks uninstrumented services instrumented=false with severity from targets only", () => {
		const up = makeMetric("availability_services", [
			makeSeries({ job: "mystery-exporter" }, [
				{ time: "2026-11-06T12:00:00Z", value: 1 },
			]),
			makeSeries({ job: "auth-hub" }, [
				{ time: "2026-11-06T12:00:00Z", value: 1 },
			]),
		]);
		const rows = buildServiceRows(indexMetrics([up]));
		const authHub = rows.find((r) => r.service === "auth-hub");
		const mystery = rows.find((r) => r.service === "mystery-exporter");
		expect(authHub?.group).toBe("API & BFF");
		expect(authHub?.instrumented).toBe(false);
		expect(authHub?.severity).toBe("ok");
		expect(mystery?.group).toBe("Other");
		expect(mystery?.instrumented).toBe(false);
		expect(mystery?.severity).toBe("ok");
	});

	it("flags 5xx and p95 over thresholds with readable reasons and orders by group then severity", () => {
		const up = makeMetric("availability_services", [
			makeSeries({ job: "plecto-proxy" }, [
				{ time: "2026-11-06T12:00:00Z", value: 1 },
			]),
			makeSeries({ job: "alt-backend" }, [
				{ time: "2026-11-06T12:00:00Z", value: 1 },
			]),
			makeSeries({ job: "alt-butterfly-facade" }, [
				{ time: "2026-11-06T12:00:00Z", value: 1 },
			]),
			makeSeries({ job: "auth-hub" }, [
				{ time: "2026-11-06T12:00:00Z", value: 1 },
			]),
		]);
		const rps = makeMetric("http_rps", [
			makeSeries({ job: "plecto-proxy", signal: "http_server" }, [
				{ time: "2026-11-06T12:00:00Z", value: 25 },
			]),
			makeSeries({ job: "alt-backend", signal: "http_server" }, [
				{ time: "2026-11-06T12:00:00Z", value: 10 },
			]),
			makeSeries({ job: "alt-butterfly-facade", signal: "user_journey" }, [
				{ time: "2026-11-06T12:00:00Z", value: 10 },
			]),
		]);
		const err = makeMetric("http_error_ratio", [
			makeSeries({ job: "alt-backend", signal: "http_server" }, [
				{ time: "2026-11-06T12:00:00Z", value: 0.021 },
			]),
			makeSeries({ job: "alt-butterfly-facade", signal: "user_journey" }, [
				{ time: "2026-11-06T12:00:00Z", value: 0.08 },
			]),
		]);
		const p95 = makeMetric("http_latency_p95", [
			makeSeries({ job: "alt-backend", signal: "http_server" }, [
				{ time: "2026-11-06T12:00:00Z", value: 1.4 },
			]),
		]);

		const rows = buildServiceRows(indexMetrics([up, rps, err, p95]));
		expect(rows.map((r) => r.service)).toEqual([
			"plecto-proxy",
			"alt-butterfly-facade",
			"alt-backend",
			"auth-hub",
		]);

		const backend = rows.find((r) => r.service === "alt-backend");
		expect(backend?.instrumented).toBe(true);
		expect(backend?.signal).toBe("http_server");
		expect(backend?.severity).toBe("warn");
		expect(backend?.reasons.some((r) => r.includes("5xx"))).toBe(true);
		expect(backend?.reasons.some((r) => r.includes("p95"))).toBe(true);

		const bff = rows.find((r) => r.service === "alt-butterfly-facade");
		expect(bff?.severity).toBe("crit");
	});

	it("extracts scrape_duration and prometheus_scrape_lag per job", () => {
		const up = makeMetric("availability_services", [
			makeSeries({ job: "news-creator" }, [
				{ time: "2026-11-06T12:00:00Z", value: 1 },
			]),
			makeSeries({ job: "news-creator-ops" }, [
				{ time: "2026-11-06T12:00:00Z", value: 1 },
			]),
		]);
		const lag = makeMetric("prometheus_scrape_lag", [
			makeSeries({ job: "news-creator" }, [
				{ time: "2026-11-06T12:00:00Z", value: 6 },
			]),
			makeSeries({ job: "news-creator-ops" }, [
				{ time: "2026-11-06T12:00:00Z", value: 45 },
			]),
		]);
		const dur = makeMetric("scrape_duration", [
			makeSeries({ job: "news-creator" }, [
				{ time: "2026-11-06T12:00:00Z", value: 0.018 },
			]),
			makeSeries({ job: "news-creator-ops" }, [
				{ time: "2026-11-06T12:00:00Z", value: 0.042 },
			]),
		]);

		const rows = buildServiceRows(indexMetrics([up, lag, dur]));
		expect(rows).toHaveLength(1);
		expect(rows[0]?.scrapeAgeSeconds).toBe(45);
		expect(rows[0]?.scrapeDurationSeconds).toBe(0.042);
		expect(rows[0]?.jobScrapes).toEqual([
			{
				job: "news-creator",
				scrapeAgeSeconds: 6,
				scrapeDurationSeconds: 0.018,
			},
			{
				job: "news-creator-ops",
				scrapeAgeSeconds: 45,
				scrapeDurationSeconds: 0.042,
			},
		]);
	});

	it("marks an instrumented service with no availability series (targetsTotal === 0) as nodata, not ok", () => {
		const rpsOnly = makeMetric("http_rps", [
			makeSeries({ job: "alt-backend", signal: "http_server" }, [
				{ time: "2026-11-06T12:00:00Z", value: 10 },
			]),
		]);
		const rows = buildServiceRows(indexMetrics([rpsOnly]));
		const backend = rows.find((r) => r.service === "alt-backend");
		expect(backend?.instrumented).toBe(true);
		expect(backend?.targetsTotal).toBe(0);
		expect(backend?.severity).toBe("nodata");
	});

	it("sets redMetricsAvailable=false when http_rps is missing or degraded and true when healthy", () => {
		const up = makeMetric("availability_services", [
			makeSeries({ job: "recap-evaluator" }, [
				{ time: "2026-11-06T12:00:00Z", value: 1 },
			]),
		]);
		const degradedRps = makeMetric(
			"http_rps",
			[
				makeSeries({ job: "alt-backend", signal: "http_server" }, [
					{ time: "2026-11-06T12:00:00Z", value: 10 },
				]),
			],
			{ degraded: true, reason: "prometheus timeout" },
		);
		const healthyRps = makeMetric("http_rps", [
			makeSeries({ job: "alt-backend", signal: "http_server" }, [
				{ time: "2026-11-06T12:00:00Z", value: 10 },
			]),
		]);

		const rowsMissing = buildServiceRows(indexMetrics([up]));
		expect(rowsMissing[0]?.redMetricsAvailable).toBe(false);

		const rowsDegraded = buildServiceRows(indexMetrics([up, degradedRps]));
		expect(
			rowsDegraded.find((r) => r.service === "recap-evaluator")
				?.redMetricsAvailable,
		).toBe(false);

		const rowsHealthy = buildServiceRows(indexMetrics([up, healthyRps]));
		expect(
			rowsHealthy.find((r) => r.service === "recap-evaluator")
				?.redMetricsAvailable,
		).toBe(true);
	});
});

describe("buildAlertRows", () => {
	it("produces distinct canonical keys when two alerts share alertname and target", () => {
		const firing = makeMetric("alerts_firing", [
			makeSeries(
				{
					alertname: "HighErrorRate",
					job: "alt-backend",
					route: "/v1/articles",
					severity: "critical",
					alertstate: "firing",
				},
				[{ time: "2026-11-06T12:00:00Z", value: 1 }],
			),
			makeSeries(
				{
					severity: "warning",
					route: "/v1/feeds",
					job: "alt-backend",
					alertname: "HighErrorRate",
					alertstate: "firing",
				},
				[{ time: "2026-11-06T12:00:00Z", value: 1 }],
			),
		]);

		const rows = buildAlertRows(indexMetrics([firing]));
		expect(rows).toHaveLength(2);
		expect(rows[0]?.key).toBeTruthy();
		expect(rows[1]?.key).toBeTruthy();
		expect(rows[0]?.key).not.toBe(rows[1]?.key);
	});

	it("treats Watchdog and deadman as heartbeat, joins activeSince by label set, and orders firing/critical/oldest first", () => {
		const firing = makeMetric("alerts_firing", [
			makeSeries(
				{
					alertname: "Watchdog",
					severity: "none",
					alertstate: "firing",
				},
				[{ time: "2026-11-06T12:00:00Z", value: 1 }],
			),
			makeSeries(
				{
					alertname: "DeadMansSwitch",
					severity: "deadman",
					alertstate: "firing",
				},
				[{ time: "2026-11-06T12:00:00Z", value: 1 }],
			),
			makeSeries(
				{
					alertname: "HighLatency",
					severity: "warning",
					job: "alt-backend",
					alertstate: "firing",
				},
				[{ time: "2026-11-06T12:00:00Z", value: 1 }],
			),
			makeSeries(
				{
					alertname: "TargetDown",
					severity: "critical",
					job: "mq-hub",
					alertstate: "firing",
				},
				[{ time: "2026-11-06T12:00:00Z", value: 1 }],
			),
			makeSeries(
				{
					alertname: "OutboxStuck",
					severity: "critical",
					job: "alt-harvester",
					alertstate: "firing",
				},
				[{ time: "2026-11-06T12:00:00Z", value: 1 }],
			),
		]);
		const pending = makeMetric("alerts_pending", [
			makeSeries(
				{
					alertname: "DiskFilling",
					severity: "critical",
					device: "/dev/nvme0n1p1",
					alertstate: "pending",
				},
				[{ time: "2026-11-06T12:00:00Z", value: 1 }],
			),
		]);
		const activeSince = makeMetric("alerts_active_since", [
			makeSeries(
				{
					alertname: "TargetDown",
					severity: "critical",
					job: "mq-hub",
				},
				[{ time: "2026-11-06T12:00:00Z", value: 1_700_000_200 }],
			),
			makeSeries(
				{
					alertname: "OutboxStuck",
					severity: "critical",
					job: "alt-harvester",
				},
				[{ time: "2026-11-06T12:00:00Z", value: 1_700_000_100 }],
			),
			makeSeries(
				{
					alertname: "HighLatency",
					severity: "warning",
					job: "alt-backend",
				},
				[{ time: "2026-11-06T12:00:00Z", value: 1_700_000_050 }],
			),
		]);

		const rows = buildAlertRows(indexMetrics([firing, pending, activeSince]));
		const watchdog = rows.find((r) => r.alertname === "Watchdog");
		const deadman = rows.find((r) => r.alertname === "DeadMansSwitch");
		expect(watchdog?.isHeartbeat).toBe(true);
		expect(deadman?.isHeartbeat).toBe(true);

		const nonHb = rows.filter((r) => !r.isHeartbeat);
		expect(nonHb.map((r) => r.alertname)).toEqual([
			"OutboxStuck",
			"TargetDown",
			"HighLatency",
			"DiskFilling",
		]);
		expect(nonHb[0]?.activeSince).toBe(1_700_000_100);
		expect(nonHb[1]?.activeSince).toBe(1_700_000_200);
		expect(alertRowSeverity({ state: "firing", severity: "critical" })).toBe(
			"crit",
		);
		expect(alertRowSeverity({ state: "firing", severity: "warning" })).toBe(
			"warn",
		);
		expect(alertRowSeverity({ state: "pending", severity: "critical" })).toBe(
			"warn",
		);
	});

	it("reports alertsAvailability as unavailable when alerts_firing or alerts_pending is missing or degraded", () => {
		expect(alertsAvailability(indexMetrics([]))).toEqual({
			available: false,
			reason: "missing alerts_firing, alerts_pending",
		});

		const degradedFiring = indexMetrics([
			makeMetric("alerts_firing", [], {
				degraded: true,
				reason: "prometheus timeout",
			}),
			makeMetric("alerts_pending", []),
		]);
		expect(alertsAvailability(degradedFiring)).toEqual({
			available: false,
			reason: "alerts_firing: prometheus timeout",
		});

		const healthyAlerts = indexMetrics([
			makeMetric("alerts_firing", []),
			makeMetric("alerts_pending", []),
		]);
		expect(alertsAvailability(healthyAlerts)).toEqual({
			available: true,
			reason: null,
		});
	});
});

describe("buildEdgeSignals", () => {
	it("evaluates multi-window page and ticket burn logic", () => {
		const pageMap = indexMetrics([
			makeMetric("edge_rps", [
				makeSeries({}, [{ time: "2026-11-06T12:00:00Z", value: 42 }]),
			]),
			makeMetric("edge_error_ratio", [
				makeSeries({}, [{ time: "2026-11-06T12:00:00Z", value: 0.02 }]),
			]),
			makeMetric("edge_latency_p95", [
				makeSeries({}, [{ time: "2026-11-06T12:00:00Z", value: 0.4 }]),
			]),
			makeMetric("edge_burn_5m", [
				makeSeries({}, [{ time: "2026-11-06T12:00:00Z", value: 16 }]),
			]),
			makeMetric("availability_burn_1h", [
				makeSeries({}, [{ time: "2026-11-06T12:00:00Z", value: 15 }]),
			]),
			makeMetric("edge_burn_30m", [
				makeSeries({}, [{ time: "2026-11-06T12:00:00Z", value: 8 }]),
			]),
			makeMetric("availability_burn_6h", [
				makeSeries({}, [{ time: "2026-11-06T12:00:00Z", value: 7 }]),
			]),
		]);

		const pageSignals = buildEdgeSignals(pageMap);
		expect(pageSignals.pageBurn).toBe(true);
		expect(pageSignals.ticketBurn).toBe(true);
		expect(pageSignals.burnSeverity).toBe("crit");
		expect(pageSignals.errorSeverity).toBe("crit");
		expect(pageSignals.latencySeverity).toBe("ok");

		const ticketOnlyMap = indexMetrics([
			makeMetric("edge_burn_5m", [
				makeSeries({}, [{ time: "2026-11-06T12:00:00Z", value: 20 }]),
			]),
			makeMetric("availability_burn_1h", [
				makeSeries({}, [{ time: "2026-11-06T12:00:00Z", value: 2 }]),
			]),
			makeMetric("edge_burn_30m", [
				makeSeries({}, [{ time: "2026-11-06T12:00:00Z", value: 7 }]),
			]),
			makeMetric("availability_burn_6h", [
				makeSeries({}, [{ time: "2026-11-06T12:00:00Z", value: 8 }]),
			]),
		]);
		const ticketSignals = buildEdgeSignals(ticketOnlyMap);
		expect(ticketSignals.pageBurn).toBe(false);
		expect(ticketSignals.ticketBurn).toBe(true);
		expect(ticketSignals.burnSeverity).toBe("warn");
		expect(BURN.pageThreshold).toBe(14.4);
		expect(BURN.ticketThreshold).toBe(6);

		const partialBurnMap = indexMetrics([
			makeMetric("edge_burn_5m", [
				makeSeries({}, [{ time: "2026-11-06T12:00:00Z", value: 0.5 }]),
			]),
		]);
		expect(buildEdgeSignals(partialBurnMap).burnSeverity).toBe("nodata");
	});

	it("returns 'no traffic' from edgeEmptyText only when edge_rps is healthy with 0 and target is not degraded", () => {
		expect(edgeEmptyText(indexMetrics([]), "edge_error_ratio")).toBe("no data");

		const zeroRps = indexMetrics([
			makeMetric("edge_rps", [
				makeSeries({}, [{ time: "2026-11-06T12:00:00Z", value: 0 }]),
			]),
			makeMetric("edge_error_ratio", []),
		]);
		expect(edgeEmptyText(zeroRps, "edge_error_ratio")).toBe("no traffic");

		const positiveRpsEmptyTarget = indexMetrics([
			makeMetric("edge_rps", [
				makeSeries({}, [{ time: "2026-11-06T12:00:00Z", value: 12 }]),
			]),
			makeMetric("edge_latency_p95", []),
		]);
		expect(edgeEmptyText(positiveRpsEmptyTarget, "edge_latency_p95")).toBe(
			"no data",
		);

		const positiveRpsStaleTarget = indexMetrics([
			makeMetric("edge_rps", [
				makeSeries({}, [{ time: "2026-11-06T12:10:00Z", value: 12 }]),
			]),
			makeMetric("edge_latency_p95", [
				makeSeries({}, [{ time: "2026-11-06T12:00:00Z", value: 0.2 }]),
			]),
		]);
		expect(
			edgeEmptyText(positiveRpsStaleTarget, "edge_latency_p95", {
				asOfMs: Date.parse("2026-11-06T12:10:00Z"),
				maxAgeSeconds: 300,
			}),
		).toBe("no data");

		const degradedTarget = indexMetrics([
			makeMetric("edge_rps", [
				makeSeries({}, [{ time: "2026-11-06T12:00:00Z", value: 0 }]),
			]),
			makeMetric("edge_latency_p95", [], {
				degraded: true,
				reason: "timeout",
			}),
		]);
		expect(edgeEmptyText(degradedTarget, "edge_latency_p95")).toBe("no data");

		const emptyRps = indexMetrics([
			makeMetric("edge_rps", []),
			makeMetric("edge_error_ratio", []),
		]);
		expect(edgeEmptyText(emptyRps, "edge_error_ratio")).toBe("no data");
	});
});

describe("buildHostResources", () => {
	it("includes all provided filesystem series and reports perContainerAvailable=false with hardened cAdvisor reason", () => {
		const m = indexMetrics([
			makeMetric("host_cpu_utilization", [
				makeSeries({}, [{ time: "2026-11-06T12:00:00Z", value: 0.45 }]),
			]),
			makeMetric("host_memory_utilization", [
				makeSeries({}, [{ time: "2026-11-06T12:00:00Z", value: 0.88 }]),
			]),
			makeMetric("host_filesystem_utilization", [
				makeSeries({ device: "/dev/nvme0n1p1" }, [
					{ time: "2026-11-06T12:00:00Z", value: 0.72 },
				]),
				makeSeries({ device: "/dev/sda1" }, [
					{ time: "2026-11-06T12:00:00Z", value: 0.92 },
				]),
			]),
		]);

		const res = buildHostResources(m);
		expect(res.cpu).toBe(0.45);
		expect(res.cpuSeverity).toBe("ok");
		expect(res.memory).toBe(0.88);
		expect(res.memorySeverity).toBe("warn");
		expect(res.filesystems).toHaveLength(2);
		expect(res.filesystemsAvailable).toBe(true);
		expect(res.filesystemsReason).toBeNull();
		expect(res.perContainerAvailable).toBe(false);
		expect(res.perContainerReason.toLowerCase()).toContain("hardened");
		expect(res.perContainerReason.toLowerCase()).toContain("cadvisor");
	});

	it("tracks filesystemsAvailable and filesystemsReason for missing, degraded, stale, and healthy-empty host_filesystem_utilization", () => {
		const missingRes = buildHostResources(indexMetrics([]));
		expect(missingRes.filesystemsAvailable).toBe(false);
		expect(missingRes.filesystemsReason).toBe(
			"missing host_filesystem_utilization",
		);

		const degradedRes = buildHostResources(
			indexMetrics([
				makeMetric("host_filesystem_utilization", [], {
					degraded: true,
					reason: "node_exporter down",
				}),
			]),
		);
		expect(degradedRes.filesystemsAvailable).toBe(false);
		expect(degradedRes.filesystemsReason).toBe("node_exporter down");

		const staleRes = buildHostResources(
			indexMetrics([
				makeMetric("host_filesystem_utilization", [
					makeSeries({ device: "/dev/sda1" }, [
						{ time: "2026-11-06T12:00:00Z", value: 0.5 },
					]),
				]),
			]),
			{
				asOfMs: Date.parse("2026-11-06T12:10:00Z"),
				maxAgeSeconds: 300,
			},
		);
		expect(staleRes.filesystemsAvailable).toBe(false);
		expect(staleRes.filesystemsReason).toBe("stale samples");

		const healthyEmptyRes = buildHostResources(
			indexMetrics([makeMetric("host_filesystem_utilization", [])]),
		);
		expect(healthyEmptyRes.filesystemsAvailable).toBe(true);
		expect(healthyEmptyRes.filesystemsReason).toBeNull();
	});

	it("threads eval options to mark stale points as nodata in buildHostResources and buildServiceRows", () => {
		const staleTime = "2026-11-06T12:00:00Z";
		const asOfMs = Date.parse("2026-11-06T12:10:00Z");
		const evalOpts = { asOfMs, maxAgeSeconds: 300 };

		const m = indexMetrics([
			makeMetric("host_cpu_utilization", [
				makeSeries({}, [{ time: staleTime, value: 0.96 }]),
			]),
			makeMetric("host_memory_utilization", [
				makeSeries({}, [{ time: staleTime, value: 0.97 }]),
			]),
			makeMetric("host_filesystem_utilization", [
				makeSeries({ device: "/dev/sda1" }, [{ time: staleTime, value: 0.95 }]),
			]),
			makeMetric("availability_services", [
				makeSeries({ job: "alt-backend" }, [{ time: staleTime, value: 1 }]),
			]),
			makeMetric("http_rps", [
				makeSeries({ job: "alt-backend", signal: "http_server" }, [
					{ time: staleTime, value: 25 },
				]),
			]),
			makeMetric("http_error_ratio", [
				makeSeries({ job: "alt-backend", signal: "http_server" }, [
					{ time: staleTime, value: 0.25 },
				]),
			]),
		]);

		const res = buildHostResources(m, evalOpts);
		expect(res.cpu).toBeNull();
		expect(res.cpuSeverity).toBe("nodata");
		expect(res.memory).toBeNull();
		expect(res.memorySeverity).toBe("nodata");
		expect(res.filesystems).toEqual([]);
		expect(res.filesystemsAvailable).toBe(false);
		expect(res.filesystemsReason).toBe("stale samples");

		const svcRows = buildServiceRows(m, evalOpts);
		expect(svcRows).toHaveLength(1);
		expect(svcRows[0]?.targetsTotal).toBe(0);
		expect(svcRows[0]?.severity).toBe("nodata");
		expect(svcRows[0]?.rps).toBeNull();
		expect(svcRows[0]?.errorRatio).toBeNull();
		expect(svcRows[0]?.redMetricsAvailable).toBe(false);
	});
});

describe("buildCertRows and certsAvailability", () => {
	it("orders unhealthy/crit first, then ascending by remainingSeconds", () => {
		const m = indexMetrics([
			makeMetric("cert_remaining", [
				makeSeries({ job: "alt-backend" }, [
					{ time: "2026-11-06T12:00:00Z", value: 50_000 },
				]),
				makeSeries({ job: "auth-hub" }, [
					{ time: "2026-11-06T12:00:00Z", value: 7_200 },
				]),
				makeSeries({ job: "mq-hub" }, [
					{ time: "2026-11-06T12:00:00Z", value: 80_000 },
				]),
			]),
			makeMetric("cert_healthy", [
				makeSeries({ job: "alt-backend" }, [
					{ time: "2026-11-06T12:00:00Z", value: 1 },
				]),
				makeSeries({ job: "auth-hub" }, [
					{ time: "2026-11-06T12:00:00Z", value: 1 },
				]),
				makeSeries({ job: "mq-hub" }, [
					{ time: "2026-11-06T12:00:00Z", value: 0 },
				]),
			]),
		]);

		const rows = buildCertRows(m);
		expect(rows.map((r) => r.job)).toEqual([
			"mq-hub",
			"auth-hub",
			"alt-backend",
		]);
		expect(rows[0]?.healthy).toBe(false);
		expect(rows[0]?.severity).toBe("crit");
		expect(rows[1]?.severity).toBe("warn");
		expect(rows[2]?.severity).toBe("ok");
		expect(certsAvailability(m)).toEqual({ available: true, reason: null });
	});

	it("reports certsAvailability for missing, degraded, stale, and healthy-empty certificate metrics", () => {
		expect(certsAvailability(indexMetrics([]))).toEqual({
			available: false,
			reason: "missing cert_remaining, cert_healthy",
		});

		const degradedCerts = indexMetrics([
			makeMetric("cert_remaining", [], {
				degraded: true,
				reason: "prometheus timeout",
			}),
			makeMetric("cert_healthy", []),
		]);
		expect(certsAvailability(degradedCerts)).toEqual({
			available: false,
			reason: "cert_remaining: prometheus timeout",
		});

		const staleCerts = indexMetrics([
			makeMetric("cert_remaining", [
				makeSeries({ job: "auth-hub" }, [
					{ time: "2026-11-06T12:00:00Z", value: 60_000 },
				]),
			]),
			makeMetric("cert_healthy", [
				makeSeries({ job: "auth-hub" }, [
					{ time: "2026-11-06T12:00:00Z", value: 1 },
				]),
			]),
		]);
		const evalOpts = {
			asOfMs: Date.parse("2026-11-06T12:10:00Z"),
			maxAgeSeconds: 300,
		};
		expect(buildCertRows(staleCerts, evalOpts)).toEqual([]);
		expect(certsAvailability(staleCerts, evalOpts)).toEqual({
			available: false,
			reason: "stale samples",
		});

		const healthyEmptyCerts = indexMetrics([
			makeMetric("cert_remaining", []),
			makeMetric("cert_healthy", []),
		]);
		expect(certsAvailability(healthyEmptyCerts)).toEqual({
			available: true,
			reason: null,
		});
	});
});

describe("buildPipelines", () => {
	it("extracts redis status, topic rates, backlogs, push pending, and outbox failures, and tracks publishRateAvailable and backlogAvailable", () => {
		const m = indexMetrics([
			makeMetric("mqhub_redis", [
				makeSeries({}, [{ time: "2026-11-06T12:00:00Z", value: 1 }]),
			]),
			makeMetric("mqhub_publish_rate", [
				makeSeries({ topic: "articles" }, [
					{ time: "2026-11-06T12:00:00Z", value: 3.5 },
				]),
			]),
			makeMetric("backlog_oldest_age", [
				makeSeries({ job: "pre-processor", queue: "notification_outbox" }, [
					{ time: "2026-11-06T12:00:00Z", value: 700 },
				]),
			]),
			makeMetric("push_pending_count", [
				makeSeries({ job: "alt-notifier" }, [
					{ time: "2026-11-06T12:00:00Z", value: 4 },
				]),
			]),
			makeMetric("outbox_failed_24h", [
				makeSeries({ job: "alt-harvester" }, [
					{ time: "2026-11-06T12:00:00Z", value: 0 },
				]),
			]),
		]);

		const p = buildPipelines(m);
		expect(p.redisUp).toBe(true);
		expect(p.publishByTopic).toEqual([{ topic: "articles", rate: 3.5 }]);
		expect(p.publishRateAvailable).toBe(true);
		expect(p.publishRateReason).toBeNull();
		expect(p.backlog[0]?.severity).toBe("warn");
		expect(p.backlogAvailable).toBe(true);
		expect(p.backlogReason).toBeNull();
		expect(p.pushPending).toBe(4);
		expect(p.outboxFailed24h).toBe(0);
		expect(p.outboxSeverity).toBe("ok");

		const emptyHealthy = buildPipelines(
			indexMetrics([
				makeMetric("mqhub_publish_rate", []),
				makeMetric("backlog_oldest_age", []),
			]),
		);
		expect(emptyHealthy.publishRateAvailable).toBe(true);
		expect(emptyHealthy.publishByTopic).toEqual([]);
		expect(emptyHealthy.backlogAvailable).toBe(true);
		expect(emptyHealthy.backlog).toEqual([]);

		const missingPub = buildPipelines(indexMetrics([]));
		expect(missingPub.publishRateAvailable).toBe(false);
		expect(missingPub.publishRateReason).toContain("mqhub_publish_rate");
		expect(missingPub.backlogAvailable).toBe(false);
		expect(missingPub.backlogReason).toContain("backlog_oldest_age");

		const degradedPub = buildPipelines(
			indexMetrics([
				makeMetric("mqhub_publish_rate", [], {
					degraded: true,
					reason: "prometheus timeout",
				}),
				makeMetric("backlog_oldest_age", [], {
					degraded: true,
					reason: "query canceled",
				}),
			]),
		);
		expect(degradedPub.publishRateAvailable).toBe(false);
		expect(degradedPub.publishRateReason).toBe("prometheus timeout");
		expect(degradedPub.backlogAvailable).toBe(false);
		expect(degradedPub.backlogReason).toBe("query canceled");

		const staleEvalOpts = {
			asOfMs: Date.parse("2026-11-06T12:10:00Z"),
			maxAgeSeconds: 300,
		};
		const stalePub = buildPipelines(
			indexMetrics([
				makeMetric("mqhub_publish_rate", [
					makeSeries({ topic: "articles" }, [
						{ time: "2026-11-06T12:00:00Z", value: 3.5 },
					]),
				]),
				makeMetric("backlog_oldest_age", [
					makeSeries({ job: "pre-processor", queue: "notification_outbox" }, [
						{ time: "2026-11-06T12:00:00Z", value: 700 },
					]),
				]),
			]),
			staleEvalOpts,
		);
		expect(stalePub.publishRateAvailable).toBe(false);
		expect(stalePub.publishRateReason).toContain("stale");
		expect(stalePub.backlog).toEqual([]);
		expect(stalePub.backlogAvailable).toBe(false);
		expect(stalePub.backlogReason).toContain("stale");
	});
});

describe("freshness, validSteps, clampStep, enum mappings, MONITOR_KEYS", () => {
	it("marks snapshot stale after 15 seconds", () => {
		const snap = "2026-11-06T12:00:00Z";
		const base = Date.parse(snap);
		expect(STALE_AFTER_SECONDS).toBe(15);
		expect(freshness(snap, base + 5_000)).toEqual({
			ageSeconds: 5,
			stale: false,
		});
		expect(freshness(snap, base + 20_000)).toEqual({
			ageSeconds: 20,
			stale: true,
		});
		expect(freshness(null, base)).toEqual({
			ageSeconds: null,
			stale: false,
		});
	});

	it("computes validSteps, clampStep, maxPointAgeSeconds, and exhaustive enum conversions", () => {
		expect(validSteps("1h")).toEqual(["15s", "30s", "1m", "5m"]);
		expect(validSteps("6h")).toEqual(["30s", "1m", "5m"]);
		expect(validSteps("24h")).toEqual(["5m"]);
		expect(clampStep("24h", "30s")).toBe("5m");
		expect(clampStep("1h", "30s")).toBe("30s");
		expect(clampStep("6h", "15s")).toBe("30s");
		expect(maxPointAgeSeconds("15s")).toBe(300);
		expect(maxPointAgeSeconds("30s")).toBe(300);
		expect(maxPointAgeSeconds("1m")).toBe(300);
		expect(maxPointAgeSeconds("5m")).toBe(600);
		expect(() =>
			maxPointAgeSeconds("invalid" as unknown as "30s"),
		).toThrowError(/Unhandled step option/);

		expect(windowEnumToOpt(RangeWindow.RANGE_WINDOW_5M)).toBe("5m");
		expect(windowEnumToOpt(RangeWindow.RANGE_WINDOW_15M)).toBe("15m");
		expect(windowEnumToOpt(RangeWindow.RANGE_WINDOW_1H)).toBe("1h");
		expect(windowEnumToOpt(RangeWindow.RANGE_WINDOW_6H)).toBe("6h");
		expect(windowEnumToOpt(RangeWindow.RANGE_WINDOW_24H)).toBe("24h");
		expect(() =>
			windowEnumToOpt(RangeWindow.RANGE_WINDOW_UNSPECIFIED),
		).toThrowError(/Unsupported range window/);

		expect(stepEnumToOpt(Step.STEP_15S)).toBe("15s");
		expect(stepEnumToOpt(Step.STEP_30S)).toBe("30s");
		expect(stepEnumToOpt(Step.STEP_1M)).toBe("1m");
		expect(stepEnumToOpt(Step.STEP_5M)).toBe("5m");
		expect(() => stepEnumToOpt(Step.STEP_UNSPECIFIED)).toThrowError(
			/Unsupported step/,
		);

		expect(optToStepEnum("15s")).toBe(Step.STEP_15S);
		expect(optToStepEnum("30s")).toBe(Step.STEP_30S);
		expect(optToStepEnum("1m")).toBe(Step.STEP_1M);
		expect(optToStepEnum("5m")).toBe(Step.STEP_5M);
		expect(() => optToStepEnum("invalid" as unknown as "30s")).toThrowError(
			/Unhandled step option/,
		);
	});

	it("exports all required MONITOR_KEYS", () => {
		expect(MONITOR_KEYS).toContain("availability_services");
		expect(MONITOR_KEYS).toContain("edge_rps");
		expect(MONITOR_KEYS).toContain("alerts_firing");
		expect(MONITOR_KEYS).toContain("cert_remaining");
		expect(MONITOR_KEYS).toContain("outbox_failed_24h");
	});
});

describe("buildAttention", () => {
	const snapTime = "2026-11-06T12:00:00Z";
	const freshNow = Date.parse(snapTime) + 3_000;

	it("emits 'No snapshot received yet' when snapshotTime is null or metrics is empty", () => {
		const emptyItems = buildAttention({
			metrics: indexMetrics([]),
			streamState: "connecting",
			snapshotTime: null,
			now: freshNow,
			lastError: null,
			maxAgeSeconds: 300,
		});
		expect(emptyItems).toContainEqual({
			severity: "warn",
			section: "telemetry",
			anchor: "#attention",
			text: "No snapshot received yet",
		});

		const noMetricsItems = buildAttention({
			metrics: indexMetrics([]),
			streamState: "live",
			snapshotTime: snapTime,
			now: freshNow,
			lastError: null,
			maxAgeSeconds: 300,
		});
		expect(
			noMetricsItems.some((i) => i.text === "No snapshot received yet"),
		).toBe(true);
	});

	it("returns [] for an all-healthy fixture with all MONITOR_KEYS present (excluding heartbeat alerts)", () => {
		const baseMetrics: MetricResult[] = [
			makeMetric("availability_services", [
				makeSeries({ job: "alt-backend" }, [{ time: snapTime, value: 1 }]),
			]),
			makeMetric("http_rps", [
				makeSeries({ job: "alt-backend", signal: "http_server" }, [
					{ time: snapTime, value: 15 },
				]),
			]),
			makeMetric("alerts_firing", [
				makeSeries(
					{ alertname: "Watchdog", severity: "none", alertstate: "firing" },
					[{ time: snapTime, value: 1 }],
				),
				makeSeries(
					{
						alertname: "DeadMansSwitch",
						severity: "deadman",
						alertstate: "firing",
					},
					[{ time: snapTime, value: 1 }],
				),
			]),
			makeMetric("edge_rps", [makeSeries({}, [{ time: snapTime, value: 25 }])]),
			makeMetric("edge_error_ratio", [
				makeSeries({}, [{ time: snapTime, value: 0.0001 }]),
			]),
			makeMetric("edge_latency_p95", [
				makeSeries({}, [{ time: snapTime, value: 0.2 }]),
			]),
			makeMetric("edge_burn_5m", [
				makeSeries({}, [{ time: snapTime, value: 0.5 }]),
			]),
			makeMetric("availability_burn_1h", [
				makeSeries({}, [{ time: snapTime, value: 0.5 }]),
			]),
			makeMetric("host_cpu_utilization", [
				makeSeries({}, [{ time: snapTime, value: 0.3 }]),
			]),
			makeMetric("host_memory_utilization", [
				makeSeries({}, [{ time: snapTime, value: 0.5 }]),
			]),
			makeMetric("cert_remaining", [
				makeSeries({ job: "alt-backend" }, [{ time: snapTime, value: 86_400 }]),
			]),
			makeMetric("cert_healthy", [
				makeSeries({ job: "alt-backend" }, [{ time: snapTime, value: 1 }]),
			]),
			makeMetric("mqhub_redis", [
				makeSeries({}, [{ time: snapTime, value: 1 }]),
			]),
		];
		const presentKeys = new Set(baseMetrics.map((m) => m.key));
		for (const key of MONITOR_KEYS) {
			if (!presentKeys.has(key)) {
				baseMetrics.push(makeMetric(key, []));
			}
		}
		const m = indexMetrics(baseMetrics);

		const items = buildAttention({
			metrics: m,
			streamState: "live",
			snapshotTime: snapTime,
			now: freshNow,
			lastError: null,
			maxAgeSeconds: 300,
		});
		expect(items).toEqual([]);
	});

	it("emits one telemetry item naming missing keys when fewer keys are received than requested", () => {
		const m = indexMetrics(
			MONITOR_KEYS.filter(
				(k) => k !== "cert_healthy" && k !== "mqhub_redis",
			).map((k) => makeMetric(k, [])),
		);

		const items = buildAttention({
			metrics: m,
			streamState: "live",
			snapshotTime: snapTime,
			now: freshNow,
			lastError: null,
			maxAgeSeconds: 300,
		});
		const missingItems = items.filter(
			(i) => i.section === "telemetry" && i.text.includes("Missing"),
		);
		expect(missingItems).toHaveLength(1);
		expect(missingItems[0]?.text).toContain("cert_healthy");
		expect(missingItems[0]?.text).toContain("mqhub_redis");
	});

	it("groups degraded-metric attention items by reason and orders crit before warn", () => {
		const m = indexMetrics([
			makeMetric("edge_burn_5m", [
				makeSeries({}, [{ time: snapTime, value: 18 }]),
			]),
			makeMetric("availability_burn_1h", [
				makeSeries({}, [{ time: snapTime, value: 16 }]),
			]),
			makeMetric("http_rps", [], {
				degraded: true,
				reason: "prometheus timeout",
			}),
			makeMetric("http_error_ratio", [], {
				degraded: true,
				reason: "prometheus timeout",
			}),
			makeMetric("edge_rps", [], {
				degraded: true,
				reason: "query canceled",
			}),
		]);

		const staleNow = Date.parse(snapTime) + 30_000;
		const items = buildAttention({
			metrics: m,
			streamState: "live",
			snapshotTime: snapTime,
			now: staleNow,
			lastError: null,
			maxAgeSeconds: 300,
		});

		const edgeCrit = items.find(
			(i) => i.section === "edge" && i.severity === "crit",
		);
		expect(edgeCrit).toBeDefined();
		expect(edgeCrit?.anchor).toBe("#edge");

		const degradedItems = items.filter(
			(i) => i.section === "telemetry" && i.text.startsWith("Degraded"),
		);
		expect(degradedItems).toHaveLength(2);
		const timeoutItem = degradedItems.find((i) =>
			i.text.includes("prometheus timeout"),
		);
		expect(timeoutItem?.text).toContain("2");
		expect(timeoutItem?.text).toContain("http_error_ratio");
		expect(timeoutItem?.text).toContain("http_rps");

		const canceledItem = degradedItems.find((i) =>
			i.text.includes("query canceled"),
		);
		expect(canceledItem?.text).toContain("edge_rps");

		expect(items.some((i) => i.text.toLowerCase().includes("stale"))).toBe(
			true,
		);

		const firstWarnIdx = items.findIndex((i) => i.severity === "warn");
		const lastCritIdx = items.map((i) => i.severity).lastIndexOf("crit");
		expect(firstWarnIdx).toBeGreaterThan(0);
		expect(lastCritIdx).toBeGreaterThanOrEqual(0);
		expect(lastCritIdx).toBeLessThan(firstWarnIdx);
	});

	it("emits telemetry warn when alert state is unavailable and crit when Watchdog heartbeat is not firing", () => {
		const missingAlertsMap = indexMetrics([
			makeMetric("edge_rps", [makeSeries({}, [{ time: snapTime, value: 10 }])]),
		]);
		const missingItems = buildAttention({
			metrics: missingAlertsMap,
			streamState: "live",
			snapshotTime: snapTime,
			now: freshNow,
			lastError: null,
			maxAgeSeconds: 300,
		});
		expect(
			missingItems.some(
				(i) =>
					i.section === "telemetry" &&
					i.severity === "warn" &&
					i.text.includes("Alert state unavailable"),
			),
		).toBe(true);

		const noWatchdogMap = indexMetrics([
			makeMetric("alerts_firing", []),
			makeMetric("alerts_pending", []),
		]);
		const noWatchdogItems = buildAttention({
			metrics: noWatchdogMap,
			streamState: "live",
			snapshotTime: snapTime,
			now: freshNow,
			lastError: null,
			maxAgeSeconds: 300,
		});
		expect(noWatchdogItems).toContainEqual({
			severity: "crit",
			section: "telemetry",
			anchor: "#alerts",
			text: "Watchdog heartbeat not firing — alert pipeline may be broken",
		});
	});

	it("emits cause-specific coverage warns anchored to their sections when recording-rule or scrape core signals lack current samples", () => {
		const zeroSeriesMap = indexMetrics([
			makeMetric("alerts_firing", [
				makeSeries(
					{ alertname: "Watchdog", severity: "none", alertstate: "firing" },
					[{ time: snapTime, value: 1 }],
				),
			]),
			makeMetric("alerts_pending", []),
			makeMetric("edge_rps", []),
			makeMetric("host_cpu_utilization", []),
			makeMetric("host_memory_utilization", [
				makeSeries({}, [{ time: snapTime, value: 0.4 }]),
			]),
			makeMetric("http_rps", [
				makeSeries({ job: "alt-backend", signal: "http_server" }, [
					{ time: snapTime, value: 10 },
				]),
			]),
			makeMetric("availability_services", [
				makeSeries({ job: "alt-backend" }, [{ time: snapTime, value: 1 }]),
			]),
		]);

		const zeroItems = buildAttention({
			metrics: zeroSeriesMap,
			streamState: "live",
			snapshotTime: snapTime,
			now: freshNow,
			lastError: null,
			maxAgeSeconds: 300,
		});
		expect(zeroItems).toContainEqual({
			severity: "warn",
			section: "telemetry",
			anchor: "#edge",
			text: "No current samples for edge_rps, host_cpu_utilization — recording rules not loaded or not evaluating, or exporters down",
		});

		const httpRpsEmptyMetrics: MetricResult[] = [
			makeMetric("alerts_firing", [
				makeSeries(
					{ alertname: "Watchdog", severity: "none", alertstate: "firing" },
					[{ time: snapTime, value: 1 }],
				),
			]),
			makeMetric("alerts_pending", []),
			makeMetric("edge_rps", [makeSeries({}, [{ time: snapTime, value: 10 }])]),
			makeMetric("host_cpu_utilization", [
				makeSeries({}, [{ time: snapTime, value: 0.3 }]),
			]),
			makeMetric("host_memory_utilization", [
				makeSeries({}, [{ time: snapTime, value: 0.4 }]),
			]),
			makeMetric("http_rps", []),
			makeMetric("availability_services", [
				makeSeries({ job: "alt-backend" }, [{ time: snapTime, value: 1 }]),
			]),
		];
		const presentHttpEmpty = new Set(httpRpsEmptyMetrics.map((m) => m.key));
		for (const key of MONITOR_KEYS) {
			if (!presentHttpEmpty.has(key)) {
				httpRpsEmptyMetrics.push(makeMetric(key, []));
			}
		}
		const httpRpsEmptyItems = buildAttention({
			metrics: indexMetrics(httpRpsEmptyMetrics),
			streamState: "live",
			snapshotTime: snapTime,
			now: freshNow,
			lastError: null,
			maxAgeSeconds: 300,
		});
		expect(httpRpsEmptyItems).toContainEqual({
			severity: "warn",
			section: "telemetry",
			anchor: "#services",
			text: "No current samples for http_rps — recording rules not loaded or not evaluating, or exporters down",
		});

		const availEmptyItems = buildAttention({
			metrics: indexMetrics([
				makeMetric("alerts_firing", [
					makeSeries(
						{ alertname: "Watchdog", severity: "none", alertstate: "firing" },
						[{ time: snapTime, value: 1 }],
					),
				]),
				makeMetric("alerts_pending", []),
				makeMetric("edge_rps", [
					makeSeries({}, [{ time: snapTime, value: 10 }]),
				]),
				makeMetric("host_cpu_utilization", []),
				makeMetric("host_memory_utilization", [
					makeSeries({}, [{ time: snapTime, value: 0.4 }]),
				]),
				makeMetric("http_rps", [
					makeSeries({ job: "alt-backend", signal: "http_server" }, [
						{ time: snapTime, value: 10 },
					]),
				]),
				makeMetric("availability_services", []),
			]),
			streamState: "live",
			snapshotTime: snapTime,
			now: freshNow,
			lastError: null,
			maxAgeSeconds: 300,
		});
		expect(availEmptyItems).toContainEqual({
			severity: "warn",
			section: "telemetry",
			anchor: "#resources",
			text: "No current samples for host_cpu_utilization — recording rules not loaded or not evaluating, or exporters down",
		});
		expect(availEmptyItems).toContainEqual({
			severity: "warn",
			section: "telemetry",
			anchor: "#services",
			text: "No current samples for availability_services — scrape target availability metric unavailable or stale",
		});

		const stalePointTime = new Date(
			Date.parse(snapTime) - 600_000,
		).toISOString();
		const staleCoreMetrics: MetricResult[] = [
			makeMetric("alerts_firing", [
				makeSeries(
					{ alertname: "Watchdog", severity: "none", alertstate: "firing" },
					[{ time: snapTime, value: 1 }],
				),
			]),
			makeMetric("alerts_pending", []),
			makeMetric("edge_rps", [
				makeSeries({}, [{ time: stalePointTime, value: 25 }]),
			]),
			makeMetric("host_cpu_utilization", [
				makeSeries({}, [{ time: stalePointTime, value: 0.3 }]),
			]),
			makeMetric("host_memory_utilization", [
				makeSeries({}, [{ time: stalePointTime, value: 0.5 }]),
			]),
			makeMetric("http_rps", [
				makeSeries({ job: "alt-backend", signal: "http_server" }, [
					{ time: stalePointTime, value: 15 },
				]),
			]),
			makeMetric("availability_services", [
				makeSeries({ job: "alt-backend" }, [{ time: snapTime, value: 1 }]),
			]),
		];
		const presentStale = new Set(staleCoreMetrics.map((m) => m.key));
		for (const key of MONITOR_KEYS) {
			if (!presentStale.has(key)) {
				staleCoreMetrics.push(makeMetric(key, []));
			}
		}

		const staleItems = buildAttention({
			metrics: indexMetrics(staleCoreMetrics),
			streamState: "live",
			snapshotTime: snapTime,
			now: freshNow,
			lastError: null,
			maxAgeSeconds: 300,
		});
		expect(staleItems).toContainEqual({
			severity: "warn",
			section: "telemetry",
			anchor: "#edge",
			text: "No current samples for edge_rps, host_cpu_utilization, host_memory_utilization, http_rps — recording rules not loaded or not evaluating, or exporters down",
		});

		const healthyCoreMetrics: MetricResult[] = [
			makeMetric("alerts_firing", [
				makeSeries(
					{ alertname: "Watchdog", severity: "none", alertstate: "firing" },
					[{ time: snapTime, value: 1 }],
				),
			]),
			makeMetric("alerts_pending", []),
			makeMetric("edge_rps", [makeSeries({}, [{ time: snapTime, value: 0 }])]),
			makeMetric("host_cpu_utilization", [
				makeSeries({}, [{ time: snapTime, value: 0.3 }]),
			]),
			makeMetric("host_memory_utilization", [
				makeSeries({}, [{ time: snapTime, value: 0.5 }]),
			]),
			makeMetric("http_rps", [
				makeSeries({ job: "alt-backend", signal: "http_server" }, [
					{ time: snapTime, value: 12 },
				]),
			]),
			makeMetric("availability_services", [
				makeSeries({ job: "alt-backend" }, [{ time: snapTime, value: 1 }]),
			]),
		];
		const presentHealthy = new Set(healthyCoreMetrics.map((m) => m.key));
		for (const key of MONITOR_KEYS) {
			if (!presentHealthy.has(key)) {
				healthyCoreMetrics.push(makeMetric(key, []));
			}
		}

		const healthyItems = buildAttention({
			metrics: indexMetrics(healthyCoreMetrics),
			streamState: "live",
			snapshotTime: snapTime,
			now: freshNow,
			lastError: null,
			maxAgeSeconds: 300,
		});
		expect(healthyItems).toEqual([]);

		const firstSeriesStaleMetrics: MetricResult[] = [
			makeMetric("alerts_firing", [
				makeSeries(
					{ alertname: "Watchdog", severity: "none", alertstate: "firing" },
					[{ time: snapTime, value: 1 }],
				),
			]),
			makeMetric("alerts_pending", []),
			makeMetric("edge_rps", [makeSeries({}, [{ time: snapTime, value: 5 }])]),
			makeMetric("host_cpu_utilization", [
				makeSeries({}, [{ time: snapTime, value: 0.2 }]),
			]),
			makeMetric("host_memory_utilization", [
				makeSeries({}, [{ time: snapTime, value: 0.4 }]),
			]),
			makeMetric("http_rps", [
				makeSeries({ job: "retired-service", signal: "http_server" }, [
					{ time: stalePointTime, value: 3 },
				]),
				makeSeries({ job: "alt-backend", signal: "http_server" }, [
					{ time: snapTime, value: 12 },
				]),
			]),
			makeMetric("availability_services", [
				makeSeries({ job: "retired-service" }, [
					{ time: stalePointTime, value: 1 },
				]),
				makeSeries({ job: "alt-backend" }, [{ time: snapTime, value: 1 }]),
			]),
		];
		const presentFirstStale = new Set(
			firstSeriesStaleMetrics.map((m) => m.key),
		);
		for (const key of MONITOR_KEYS) {
			if (!presentFirstStale.has(key)) {
				firstSeriesStaleMetrics.push(makeMetric(key, []));
			}
		}
		const firstSeriesStaleItems = buildAttention({
			metrics: indexMetrics(firstSeriesStaleMetrics),
			streamState: "live",
			snapshotTime: snapTime,
			now: freshNow,
			lastError: null,
			maxAgeSeconds: 300,
		});
		expect(firstSeriesStaleItems).toEqual([]);

		const allMultiSeriesStaleItems = buildAttention({
			metrics: indexMetrics([
				makeMetric("alerts_firing", [
					makeSeries(
						{ alertname: "Watchdog", severity: "none", alertstate: "firing" },
						[{ time: snapTime, value: 1 }],
					),
				]),
				makeMetric("alerts_pending", []),
				makeMetric("edge_rps", [
					makeSeries({}, [{ time: snapTime, value: 5 }]),
				]),
				makeMetric("host_cpu_utilization", [
					makeSeries({}, [{ time: snapTime, value: 0.2 }]),
				]),
				makeMetric("host_memory_utilization", [
					makeSeries({}, [{ time: snapTime, value: 0.4 }]),
				]),
				makeMetric("http_rps", [
					makeSeries({ job: "retired-service", signal: "http_server" }, [
						{ time: stalePointTime, value: 3 },
					]),
					makeSeries({ job: "alt-backend", signal: "http_server" }, [
						{ time: stalePointTime, value: 12 },
					]),
				]),
				makeMetric("availability_services", [
					makeSeries({ job: "retired-service" }, [
						{ time: stalePointTime, value: 1 },
					]),
					makeSeries({ job: "alt-backend" }, [
						{ time: stalePointTime, value: 1 },
					]),
				]),
			]),
			streamState: "live",
			snapshotTime: snapTime,
			now: freshNow,
			lastError: null,
			maxAgeSeconds: 300,
		});
		expect(allMultiSeriesStaleItems).toContainEqual({
			severity: "warn",
			section: "telemetry",
			anchor: "#services",
			text: "No current samples for http_rps — recording rules not loaded or not evaluating, or exporters down",
		});
		expect(allMultiSeriesStaleItems).toContainEqual({
			severity: "warn",
			section: "telemetry",
			anchor: "#services",
			text: "No current samples for availability_services — scrape target availability metric unavailable or stale",
		});
	});
});
