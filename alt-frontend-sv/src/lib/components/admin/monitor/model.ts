import {
	type MetricResult,
	RangeWindow,
	type Series,
	Step,
} from "#lib/gen/alt/admin_monitor/v1/admin_monitor_pb.js";

export type Severity = "ok" | "warn" | "crit" | "nodata" | "stale";

export interface Badge {
	severity: Severity;
	glyph: string;
	label: "OK" | "WARN" | "CRIT" | "NO DATA" | "STALE";
}

export function badge(sev: Severity): Badge {
	switch (sev) {
		case "ok":
			return { severity: "ok", glyph: "●", label: "OK" };
		case "warn":
			return { severity: "warn", glyph: "▲", label: "WARN" };
		case "crit":
			return { severity: "crit", glyph: "■", label: "CRIT" };
		case "nodata":
			return { severity: "nodata", glyph: "○", label: "NO DATA" };
		case "stale":
			return { severity: "stale", glyph: "◌", label: "STALE" };
	}
}

export type MetricMap = ReadonlyMap<string, MetricResult>;

export function indexMetrics(metrics: readonly MetricResult[]): MetricMap {
	const map = new Map<string, MetricResult>();
	for (const m of metrics) {
		map.set(m.key, m);
	}
	return map;
}

function matchesLabels(
	labels: Record<string, string>,
	match?: Record<string, string>,
): boolean {
	if (!match) return true;
	for (const [k, v] of Object.entries(match)) {
		if (labels[k] !== v) return false;
	}
	return true;
}

function parseTimestamp(raw: string, fallbackIndex: number): number {
	const parsed = Date.parse(raw);
	if (Number.isFinite(parsed)) return parsed;
	const num = Number(raw);
	if (raw.trim() !== "" && Number.isFinite(num)) return num;
	return fallbackIndex;
}

function extractPointsFromSeries(s: Series): { t: number; v: number }[] {
	const out: { t: number; v: number }[] = [];
	for (let i = 0; i < s.points.length; i++) {
		const p = s.points[i];
		if (p && typeof p.value === "number" && Number.isFinite(p.value)) {
			out.push({ t: parseTimestamp(p.time, i), v: p.value });
		}
	}
	out.sort((a, b) => a.t - b.t);
	return out;
}

export function seriesPoints(
	m: MetricResult | undefined,
	match?: Record<string, string>,
): { t: number; v: number }[] {
	if (!m) return [];
	for (const s of m.series) {
		if (matchesLabels(s.labels, match)) {
			return extractPointsFromSeries(s);
		}
	}
	return [];
}

export interface LatestValueOptions {
	asOfMs: number;
	maxAgeSeconds: number;
}

function latestPointValue(
	pts: readonly { t: number; v: number }[],
	opts?: LatestValueOptions,
): number | null {
	const last = pts.at(-1);
	if (!last) return null;
	if (opts && opts.asOfMs - last.t > opts.maxAgeSeconds * 1000) {
		return null;
	}
	return last.v;
}

export function latestValue(
	m: MetricResult | undefined,
	match?: Record<string, string>,
	opts?: LatestValueOptions,
): number | null {
	return latestPointValue(seriesPoints(m, match), opts);
}

export interface Threshold {
	warn: number;
	crit: number;
	direction: "above" | "below";
	display: string;
}

export const THRESHOLDS: Record<
	| "edgeErrorRatio"
	| "edgeLatencyP95"
	| "serviceErrorRatio"
	| "serviceLatencyP95"
	| "hostCpu"
	| "hostMemory"
	| "hostFilesystem"
	| "certRemaining"
	| "scrapeAge"
	| "backlogAge"
	| "outboxFailed",
	Threshold
> = {
	edgeErrorRatio: {
		warn: 0.001,
		crit: 0.0144,
		direction: "above",
		display: "warn ≥ 0.10% · crit ≥ 1.44%",
	},
	edgeLatencyP95: {
		warn: 1.0,
		crit: 2.5,
		direction: "above",
		display: "warn ≥ 1.00 s · crit ≥ 2.50 s",
	},
	serviceErrorRatio: {
		warn: 0.01,
		crit: 0.05,
		direction: "above",
		display: "warn ≥ 1.0% · crit ≥ 5.0%",
	},
	serviceLatencyP95: {
		warn: 1.0,
		crit: 5.0,
		direction: "above",
		display: "warn ≥ 1.0 s · crit ≥ 5.0 s",
	},
	hostCpu: {
		warn: 0.8,
		crit: 0.95,
		direction: "above",
		display: "warn ≥ 80% · crit ≥ 95%",
	},
	hostMemory: {
		warn: 0.85,
		crit: 0.95,
		direction: "above",
		display: "warn ≥ 85% · crit ≥ 95%",
	},
	hostFilesystem: {
		warn: 0.8,
		crit: 0.9,
		direction: "above",
		display: "warn ≥ 80% · crit ≥ 90%",
	},
	certRemaining: {
		warn: 14400,
		crit: 3600,
		direction: "below",
		display: "warn ≤ 4h · crit ≤ 1h",
	},
	scrapeAge: {
		warn: 60,
		crit: 300,
		direction: "above",
		display: "warn ≥ 60 s · crit ≥ 300 s",
	},
	backlogAge: {
		warn: 600,
		crit: 3600,
		direction: "above",
		display: "warn ≥ 10m · crit ≥ 1h",
	},
	outboxFailed: {
		warn: 1,
		crit: 50,
		direction: "above",
		display: "warn ≥ 1 · crit ≥ 50",
	},
};

export const BURN = { pageThreshold: 14.4, ticketThreshold: 6 } as const;
export const STALE_AFTER_SECONDS = 15;

export function classify(value: number | null, t: Threshold): Severity {
	if (value == null || !Number.isFinite(value)) return "nodata";
	if (t.direction === "above") {
		if (value >= t.crit) return "crit";
		if (value >= t.warn) return "warn";
		return "ok";
	}
	if (value <= t.crit) return "crit";
	if (value <= t.warn) return "warn";
	return "ok";
}

export type ServiceGroup =
	| "Edge"
	| "API & BFF"
	| "Data & Messaging"
	| "Workers"
	| "Monitoring"
	| "Other";

const SERVICE_GROUPS: Readonly<Record<string, ServiceGroup>> = {
	"plecto-proxy": "Edge",
	"alt-butterfly-facade": "API & BFF",
	"alt-backend": "API & BFF",
	"auth-hub": "API & BFF",
	"alt-data-hub": "Data & Messaging",
	"knowledge-sovereign": "Data & Messaging",
	"search-indexer": "Data & Messaging",
	"mq-hub": "Data & Messaging",
	"alt-harvester": "Workers",
	"alt-notifier": "Workers",
	"pre-processor": "Workers",
	"tag-generator": "Workers",
	"news-creator": "Workers",
	"recap-worker": "Workers",
	"recap-subworker": "Workers",
	"recap-evaluator": "Workers",
	"rag-orchestrator": "Workers",
	"acolyte-orchestrator": "Workers",
	prometheus: "Monitoring",
	alertmanager: "Monitoring",
	cadvisor: "Monitoring",
};

const GROUP_ORDER: Readonly<Record<ServiceGroup, number>> = {
	Edge: 0,
	"API & BFF": 1,
	"Data & Messaging": 2,
	Workers: 3,
	Monitoring: 4,
	Other: 5,
};

const SEVERITY_ORDER: Readonly<Record<Severity, number>> = {
	crit: 0,
	warn: 1,
	stale: 2,
	nodata: 3,
	ok: 4,
};

export interface JobScrapeInfo {
	job: string;
	scrapeAgeSeconds: number | null;
	scrapeDurationSeconds: number | null;
}

export interface ServiceRow {
	service: string;
	group: ServiceGroup;
	jobs: string[];
	targetsUp: number;
	targetsTotal: number;
	scrapeAgeSeconds: number | null;
	scrapeDurationSeconds: number | null;
	jobScrapes: JobScrapeInfo[];
	signal: string | null;
	instrumented: boolean;
	redMetricsAvailable: boolean;
	rps: number | null;
	errorRatio: number | null;
	p50: number | null;
	p95: number | null;
	p99: number | null;
	severity: Severity;
	reasons: string[];
}

function canonicalService(job: string): string {
	return job.endsWith("-ops") ? job.slice(0, -4) : job;
}

function formatRatioPct(ratio: number): string {
	const pct = ratio * 100;
	return Number.isInteger(pct) ? `${pct}%` : `${pct.toFixed(1)}%`;
}

export function buildServiceRows(
	m: MetricMap,
	opts?: LatestValueOptions,
): ServiceRow[] {
	const jobsByService = new Map<string, Set<string>>();
	const serviceKeys = [
		"availability_services",
		"prometheus_scrape_lag",
		"scrape_duration",
		"http_rps",
		"http_error_ratio",
		"http_latency_p50",
		"http_latency_p95",
		"http_latency_p99",
	];

	for (const key of serviceKeys) {
		const metric = m.get(key);
		if (!metric) continue;
		for (const s of metric.series) {
			const job = s.labels.job;
			if (!job) continue;
			const svc = canonicalService(job);
			let set = jobsByService.get(svc);
			if (!set) {
				set = new Set<string>();
				jobsByService.set(svc, set);
			}
			set.add(job);
		}
	}

	const availability = m.get("availability_services");
	const scrapeLag = m.get("prometheus_scrape_lag");
	const scrapeDuration = m.get("scrape_duration");
	const httpRps = m.get("http_rps");
	const httpErr = m.get("http_error_ratio");
	const httpP50 = m.get("http_latency_p50");
	const httpP95 = m.get("http_latency_p95");
	const httpP99 = m.get("http_latency_p99");

	const httpRpsHasCurrent =
		httpRps !== undefined &&
		!httpRps.degraded &&
		httpRps.series.some(
			(s) => latestPointValue(extractPointsFromSeries(s), opts) !== null,
		);
	const redMetricsAvailable =
		httpRpsHasCurrent &&
		![httpErr, httpP50, httpP95, httpP99].some((rm) => rm?.degraded);

	const rows: ServiceRow[] = [];

	for (const [service, jobSet] of jobsByService) {
		const jobs = Array.from(jobSet).sort((a, b) => a.localeCompare(b));
		const group: ServiceGroup = SERVICE_GROUPS[service] ?? "Other";

		let targetsUp = 0;
		let targetsTotal = 0;
		for (const job of jobs) {
			const v = latestValue(availability, { job }, opts);
			if (v !== null) {
				targetsTotal += 1;
				if (v >= 1) targetsUp += 1;
			}
		}

		let scrapeAgeSeconds: number | null = null;
		let scrapeDurationSeconds: number | null = null;
		const jobScrapes: JobScrapeInfo[] = [];
		for (const job of jobs) {
			const age = latestValue(scrapeLag, { job }, opts);
			if (age !== null) {
				scrapeAgeSeconds =
					scrapeAgeSeconds === null ? age : Math.max(scrapeAgeSeconds, age);
			}
			const dur = latestValue(scrapeDuration, { job }, opts);
			if (dur !== null) {
				scrapeDurationSeconds =
					scrapeDurationSeconds === null
						? dur
						: Math.max(scrapeDurationSeconds, dur);
			}
			jobScrapes.push({
				job,
				scrapeAgeSeconds: age,
				scrapeDurationSeconds: dur,
			});
		}

		let signal: string | null = null;
		let rps: number | null = null;
		let errorRatio: number | null = null;
		let p50: number | null = null;
		let p95: number | null = null;
		let p99: number | null = null;

		for (const job of jobs) {
			if (rps === null) rps = latestValue(httpRps, { job }, opts);
			if (errorRatio === null) errorRatio = latestValue(httpErr, { job }, opts);
			if (p50 === null) p50 = latestValue(httpP50, { job }, opts);
			if (p95 === null) p95 = latestValue(httpP95, { job }, opts);
			if (p99 === null) p99 = latestValue(httpP99, { job }, opts);
			if (signal === null) {
				for (const redMetric of [httpRps, httpErr, httpP95, httpP50, httpP99]) {
					const matched = redMetric?.series.find((s) => s.labels.job === job);
					if (matched?.labels.signal) {
						signal = matched.labels.signal;
						break;
					}
				}
			}
		}

		const instrumented =
			rps !== null ||
			errorRatio !== null ||
			p50 !== null ||
			p95 !== null ||
			p99 !== null ||
			signal !== null;

		const reasons: string[] = [];
		let worst: Severity = targetsTotal === 0 ? "nodata" : "ok";

		const promote = (sev: Severity) => {
			if (SEVERITY_ORDER[sev] < SEVERITY_ORDER[worst]) {
				worst = sev;
			}
		};

		if (targetsTotal > 0 && targetsUp < targetsTotal) {
			const down = targetsTotal - targetsUp;
			promote("crit");
			reasons.push(`${down}/${targetsTotal} targets down`);
		}

		if (scrapeAgeSeconds !== null) {
			const scrapeSev = classify(scrapeAgeSeconds, THRESHOLDS.scrapeAge);
			if (scrapeSev === "crit") {
				promote("crit");
				reasons.push(
					`scrape age ${Math.round(scrapeAgeSeconds)}s ≥ ${THRESHOLDS.scrapeAge.crit}s`,
				);
			} else if (scrapeSev === "warn") {
				promote("warn");
				reasons.push(
					`scrape age ${Math.round(scrapeAgeSeconds)}s ≥ ${THRESHOLDS.scrapeAge.warn}s`,
				);
			}
		}

		if (errorRatio !== null) {
			const errSev = classify(errorRatio, THRESHOLDS.serviceErrorRatio);
			if (errSev === "crit") {
				promote("crit");
				reasons.push(
					`5xx ${formatRatioPct(errorRatio)} ≥ ${formatRatioPct(THRESHOLDS.serviceErrorRatio.crit)}`,
				);
			} else if (errSev === "warn") {
				promote("warn");
				reasons.push(
					`5xx ${formatRatioPct(errorRatio)} ≥ ${formatRatioPct(THRESHOLDS.serviceErrorRatio.warn)}`,
				);
			}
		}

		if (p95 !== null) {
			const latSev = classify(p95, THRESHOLDS.serviceLatencyP95);
			if (latSev === "crit") {
				promote("crit");
				reasons.push(
					`p95 ${p95.toFixed(2)}s ≥ ${THRESHOLDS.serviceLatencyP95.crit}s`,
				);
			} else if (latSev === "warn") {
				promote("warn");
				reasons.push(
					`p95 ${p95.toFixed(2)}s ≥ ${THRESHOLDS.serviceLatencyP95.warn}s`,
				);
			}
		}

		rows.push({
			service,
			group,
			jobs,
			targetsUp,
			targetsTotal,
			scrapeAgeSeconds,
			scrapeDurationSeconds,
			jobScrapes,
			signal,
			instrumented,
			redMetricsAvailable,
			rps,
			errorRatio,
			p50,
			p95,
			p99,
			severity: worst,
			reasons,
		});
	}

	rows.sort((a, b) => {
		const gDiff = GROUP_ORDER[a.group] - GROUP_ORDER[b.group];
		if (gDiff !== 0) return gDiff;
		const sDiff = SEVERITY_ORDER[a.severity] - SEVERITY_ORDER[b.severity];
		if (sDiff !== 0) return sDiff;
		return a.service.localeCompare(b.service);
	});

	return rows;
}

export interface AlertRow {
	key: string;
	alertname: string;
	severity: string;
	state: "firing" | "pending";
	labels: Record<string, string>;
	activeSince: number | null;
	isHeartbeat: boolean;
}

function canonicalAlertLabelKey(labels: Record<string, string>): string {
	return Object.keys(labels)
		.filter((k) => k !== "alertstate" && k !== "__name__")
		.sort()
		.map((k) => `${k}=${labels[k]}`)
		.join("\u0000");
}

function serializeAllLabels(labels: Record<string, string>): string {
	return Object.keys(labels)
		.sort()
		.map((k) => `${k}=${labels[k]}`)
		.join("\u0000");
}

function alertSeverityRank(sev: string): number {
	const s = sev.toLowerCase();
	if (s === "critical" || s === "crit" || s === "page") return 0;
	if (s === "warning" || s === "warn" || s === "ticket") return 1;
	if (s === "info") return 2;
	if (s === "deadman" || s === "none") return 4;
	return 3;
}

export function alertRowSeverity(
	row: Pick<AlertRow, "state" | "severity">,
): "warn" | "crit" {
	return row.state === "firing" && alertSeverityRank(row.severity) === 0
		? "crit"
		: "warn";
}

export function alertsAvailability(m: MetricMap): {
	available: boolean;
	reason: string | null;
} {
	const firing = m.get("alerts_firing");
	const pending = m.get("alerts_pending");
	const missing: string[] = [];
	if (!firing) missing.push("alerts_firing");
	if (!pending) missing.push("alerts_pending");
	if (missing.length > 0) {
		return {
			available: false,
			reason: `missing ${missing.join(", ")}`,
		};
	}
	const degradedReasons: string[] = [];
	for (const metric of [firing, pending]) {
		if (metric?.degraded) {
			degradedReasons.push(
				`${metric.key}: ${metric.reason?.trim() || "source degraded"}`,
			);
		}
	}
	if (degradedReasons.length > 0) {
		return {
			available: false,
			reason: degradedReasons.join("; "),
		};
	}
	return { available: true, reason: null };
}

export function buildAlertRows(m: MetricMap): AlertRow[] {
	const activeSinceByLabels = new Map<string, number>();
	const activeMetric = m.get("alerts_active_since");
	for (const s of activeMetric?.series ?? []) {
		const pts = extractPointsFromSeries(s);
		const last = pts.at(-1);
		if (last) {
			activeSinceByLabels.set(canonicalAlertLabelKey(s.labels), last.v);
		}
	}

	const rows: AlertRow[] = [];
	const collect = (
		metric: MetricResult | undefined,
		defaultState: "firing" | "pending",
	) => {
		for (const s of metric?.series ?? []) {
			const pts = extractPointsFromSeries(s);
			const latest = pts.at(-1)?.v ?? null;
			if (latest === null || latest <= 0) continue;
			const alertname = s.labels.alertname ?? "(unknown)";
			const severity = s.labels.severity ?? "unknown";
			const state: "firing" | "pending" =
				s.labels.alertstate === "pending"
					? "pending"
					: s.labels.alertstate === "firing"
						? "firing"
						: defaultState;
			const joinKey = canonicalAlertLabelKey(s.labels);
			const activeSince = activeSinceByLabels.get(joinKey) ?? null;
			const isHeartbeat =
				alertname === "Watchdog" || severity.toLowerCase() === "deadman";
			rows.push({
				key: `${state}\u0000${serializeAllLabels(s.labels)}`,
				alertname,
				severity,
				state,
				labels: { ...s.labels },
				activeSince,
				isHeartbeat,
			});
		}
	};

	collect(m.get("alerts_firing"), "firing");
	collect(m.get("alerts_pending"), "pending");

	rows.sort((a, b) => {
		if (a.state !== b.state) return a.state === "firing" ? -1 : 1;
		const rDiff = alertSeverityRank(a.severity) - alertSeverityRank(b.severity);
		if (rDiff !== 0) return rDiff;
		const aTime = a.activeSince ?? Number.POSITIVE_INFINITY;
		const bTime = b.activeSince ?? Number.POSITIVE_INFINITY;
		if (aTime !== bTime) return aTime - bTime;
		return a.alertname.localeCompare(b.alertname);
	});

	return rows;
}

function singleMetricAvailability(
	metric: MetricResult | undefined,
	key: string,
	hasCurrentSample: boolean,
): { available: boolean; reason: string | null } {
	if (!metric) {
		return { available: false, reason: `missing ${key}` };
	}
	if (metric.degraded) {
		return {
			available: false,
			reason: metric.reason?.trim() || "source degraded",
		};
	}
	if (metric.series.length > 0 && !hasCurrentSample) {
		return { available: false, reason: "stale samples" };
	}
	return { available: true, reason: null };
}

export interface CertRow {
	job: string;
	remainingSeconds: number | null;
	healthy: boolean | null;
	severity: Severity;
}

export function buildCertRows(
	m: MetricMap,
	opts?: LatestValueOptions,
): CertRow[] {
	const remMetric = m.get("cert_remaining");
	const healthyMetric = m.get("cert_healthy");
	const jobs = new Set<string>();

	for (const s of remMetric?.series ?? []) {
		if (s.labels.job) jobs.add(s.labels.job);
	}
	for (const s of healthyMetric?.series ?? []) {
		if (s.labels.job) jobs.add(s.labels.job);
	}

	const rows: CertRow[] = [];
	for (const job of jobs) {
		const remainingSeconds = latestValue(remMetric, { job }, opts);
		const healthyVal = latestValue(healthyMetric, { job }, opts);
		if (remainingSeconds === null && healthyVal === null) continue;
		const healthy = healthyVal === null ? null : healthyVal >= 1;
		const remSev = classify(remainingSeconds, THRESHOLDS.certRemaining);
		let severity: Severity = remSev;
		if (healthy === false) {
			severity = "crit";
		} else if (remSev === "nodata" && healthy === true) {
			severity = "ok";
		}
		rows.push({ job, remainingSeconds, healthy, severity });
	}

	rows.sort((a, b) => {
		const aUnhealthy = a.healthy === false ? 0 : 1;
		const bUnhealthy = b.healthy === false ? 0 : 1;
		if (aUnhealthy !== bUnhealthy) return aUnhealthy - bUnhealthy;
		const sDiff = SEVERITY_ORDER[a.severity] - SEVERITY_ORDER[b.severity];
		if (sDiff !== 0) return sDiff;
		const aRem = a.remainingSeconds ?? Number.POSITIVE_INFINITY;
		const bRem = b.remainingSeconds ?? Number.POSITIVE_INFINITY;
		if (aRem !== bRem) return aRem - bRem;
		return a.job.localeCompare(b.job);
	});

	return rows;
}

export function certsAvailability(
	m: MetricMap,
	opts?: LatestValueOptions,
): { available: boolean; reason: string | null } {
	const remMetric = m.get("cert_remaining");
	const healthyMetric = m.get("cert_healthy");
	const missing: string[] = [];
	if (!remMetric) missing.push("cert_remaining");
	if (!healthyMetric) missing.push("cert_healthy");
	if (missing.length > 0) {
		return {
			available: false,
			reason: `missing ${missing.join(", ")}`,
		};
	}
	const degradedReasons: string[] = [];
	for (const metric of [remMetric, healthyMetric]) {
		if (metric?.degraded) {
			degradedReasons.push(
				`${metric.key}: ${metric.reason?.trim() || "source degraded"}`,
			);
		}
	}
	if (degradedReasons.length > 0) {
		return {
			available: false,
			reason: degradedReasons.join("; "),
		};
	}
	const totalSeries =
		(remMetric?.series.length ?? 0) + (healthyMetric?.series.length ?? 0);
	if (totalSeries > 0 && buildCertRows(m, opts).length === 0) {
		return { available: false, reason: "stale samples" };
	}
	return { available: true, reason: null };
}

export interface HostResources {
	cpu: number | null;
	cpuSeverity: Severity;
	memory: number | null;
	memorySeverity: Severity;
	filesystems: { device: string; ratio: number; severity: Severity }[];
	filesystemsAvailable: boolean;
	filesystemsReason: string | null;
	perContainerAvailable: false;
	perContainerReason: string;
}

export function buildHostResources(
	m: MetricMap,
	opts?: LatestValueOptions,
): HostResources {
	const cpu = latestValue(m.get("host_cpu_utilization"), undefined, opts);
	const cpuSeverity = classify(cpu, THRESHOLDS.hostCpu);
	const memory = latestValue(m.get("host_memory_utilization"), undefined, opts);
	const memorySeverity = classify(memory, THRESHOLDS.hostMemory);

	const filesystems: { device: string; ratio: number; severity: Severity }[] =
		[];
	const fsMetric = m.get("host_filesystem_utilization");
	for (const s of fsMetric?.series ?? []) {
		const pts = extractPointsFromSeries(s);
		const ratio = latestPointValue(pts, opts);
		if (ratio === null) continue;
		const device = s.labels.device ?? "(unknown)";
		filesystems.push({
			device,
			ratio,
			severity: classify(ratio, THRESHOLDS.hostFilesystem),
		});
	}
	filesystems.sort(
		(a, b) =>
			SEVERITY_ORDER[a.severity] - SEVERITY_ORDER[b.severity] ||
			b.ratio - a.ratio ||
			a.device.localeCompare(b.device),
	);
	const fsAvail = singleMetricAvailability(
		fsMetric,
		"host_filesystem_utilization",
		filesystems.length > 0,
	);

	return {
		cpu,
		cpuSeverity,
		memory,
		memorySeverity,
		filesystems,
		filesystemsAvailable: fsAvail.available,
		filesystemsReason: fsAvail.reason,
		perContainerAvailable: false,
		perContainerReason:
			"Per-container breakdown is unavailable under the hardened cAdvisor configuration (docker_only / raw cgroup paths without container names).",
	};
}

export interface BacklogRow {
	job: string;
	queue: string;
	oldestAgeSeconds: number | null;
	severity: Severity;
}

export interface Pipelines {
	redisUp: boolean | null;
	publishByTopic: { topic: string; rate: number }[];
	publishRateAvailable: boolean;
	publishRateReason: string | null;
	backlog: BacklogRow[];
	backlogAvailable: boolean;
	backlogReason: string | null;
	pushPending: number | null;
	outboxFailed24h: number | null;
	outboxSeverity: Severity;
}

function sumSeriesLatest(
	m: MetricResult | undefined,
	opts?: LatestValueOptions,
): number | null {
	if (!m) return null;
	let total = 0;
	let any = false;
	for (const s of m.series) {
		const pts = extractPointsFromSeries(s);
		const val = latestPointValue(pts, opts);
		if (val !== null) {
			total += val;
			any = true;
		}
	}
	return any ? total : null;
}

export function buildPipelines(
	m: MetricMap,
	opts?: LatestValueOptions,
): Pipelines {
	const redisVal = latestValue(m.get("mqhub_redis"), undefined, opts);
	const redisUp = redisVal === null ? null : redisVal >= 1;

	const publishByTopic: { topic: string; rate: number }[] = [];
	const pubMetric = m.get("mqhub_publish_rate");
	for (const s of pubMetric?.series ?? []) {
		const pts = extractPointsFromSeries(s);
		const rate = latestPointValue(pts, opts);
		if (rate === null) continue;
		const topic = s.labels.topic || s.labels.stream || "(default)";
		publishByTopic.push({ topic, rate });
	}
	publishByTopic.sort(
		(a, b) => b.rate - a.rate || a.topic.localeCompare(b.topic),
	);
	const pubAvail = singleMetricAvailability(
		pubMetric,
		"mqhub_publish_rate",
		publishByTopic.length > 0,
	);

	const backlog: BacklogRow[] = [];
	const backlogMetric = m.get("backlog_oldest_age");
	for (const s of backlogMetric?.series ?? []) {
		const pts = extractPointsFromSeries(s);
		const oldestAgeSeconds = latestPointValue(pts, opts);
		if (oldestAgeSeconds === null) continue;
		backlog.push({
			job: s.labels.job ?? "(unknown)",
			queue: s.labels.queue ?? "(unknown)",
			oldestAgeSeconds,
			severity: classify(oldestAgeSeconds, THRESHOLDS.backlogAge),
		});
	}
	backlog.sort(
		(a, b) =>
			SEVERITY_ORDER[a.severity] - SEVERITY_ORDER[b.severity] ||
			(b.oldestAgeSeconds ?? -1) - (a.oldestAgeSeconds ?? -1) ||
			a.queue.localeCompare(b.queue),
	);
	const backlogAvail = singleMetricAvailability(
		backlogMetric,
		"backlog_oldest_age",
		backlog.length > 0,
	);

	const pushPending = sumSeriesLatest(m.get("push_pending_count"), opts);
	const outboxFailed24h = sumSeriesLatest(m.get("outbox_failed_24h"), opts);
	const outboxSeverity = classify(outboxFailed24h, THRESHOLDS.outboxFailed);

	return {
		redisUp,
		publishByTopic,
		publishRateAvailable: pubAvail.available,
		publishRateReason: pubAvail.reason,
		backlog,
		backlogAvailable: backlogAvail.available,
		backlogReason: backlogAvail.reason,
		pushPending,
		outboxFailed24h,
		outboxSeverity,
	};
}

export interface EdgeSignals {
	rps: number | null;
	errorRatio: number | null;
	errorSeverity: Severity;
	latencyP95: number | null;
	latencySeverity: Severity;
	burn: {
		m5: number | null;
		m30: number | null;
		h1: number | null;
		h6: number | null;
	};
	pageBurn: boolean;
	ticketBurn: boolean;
	burnSeverity: Severity;
}

export function edgeEmptyText(
	m: MetricMap,
	targetKey: string,
	opts?: LatestValueOptions,
): "no traffic" | "no data" {
	const edgeRps = m.get("edge_rps");
	if (!edgeRps || edgeRps.degraded) return "no data";
	const target = m.get(targetKey);
	if (target?.degraded) return "no data";
	const rpsVal = latestValue(edgeRps, undefined, opts);
	if (rpsVal === 0) return "no traffic";
	return "no data";
}

export function buildEdgeSignals(
	m: MetricMap,
	opts?: LatestValueOptions,
): EdgeSignals {
	const rps = latestValue(m.get("edge_rps"), undefined, opts);
	const errorRatio = latestValue(m.get("edge_error_ratio"), undefined, opts);
	const errorSeverity = classify(errorRatio, THRESHOLDS.edgeErrorRatio);
	const latencyP95 = latestValue(m.get("edge_latency_p95"), undefined, opts);
	const latencySeverity = classify(latencyP95, THRESHOLDS.edgeLatencyP95);

	const m5 = latestValue(m.get("edge_burn_5m"), undefined, opts);
	const m30 = latestValue(m.get("edge_burn_30m"), undefined, opts);
	const h1 = latestValue(m.get("availability_burn_1h"), undefined, opts);
	const h6 = latestValue(m.get("availability_burn_6h"), undefined, opts);

	const pageBurn =
		h1 !== null &&
		m5 !== null &&
		h1 > BURN.pageThreshold &&
		m5 > BURN.pageThreshold;
	const ticketBurn =
		h6 !== null &&
		m30 !== null &&
		h6 > BURN.ticketThreshold &&
		m30 > BURN.ticketThreshold;

	let burnSeverity: Severity = "nodata";
	if (pageBurn) {
		burnSeverity = "crit";
	} else if (ticketBurn) {
		burnSeverity = "warn";
	} else if ((h1 !== null && m5 !== null) || (h6 !== null && m30 !== null)) {
		burnSeverity = "ok";
	}

	return {
		rps,
		errorRatio,
		errorSeverity,
		latencyP95,
		latencySeverity,
		burn: { m5, m30, h1, h6 },
		pageBurn,
		ticketBurn,
		burnSeverity,
	};
}

export type Section =
	| "alerts"
	| "edge"
	| "services"
	| "resources"
	| "certificates"
	| "pipelines"
	| "telemetry";

export interface AttentionItem {
	severity: "warn" | "crit";
	section: Section;
	anchor: string;
	text: string;
}

export function freshness(
	snapshotTime: string | null,
	now: number,
): { ageSeconds: number | null; stale: boolean } {
	if (!snapshotTime) return { ageSeconds: null, stale: false };
	const snapMs = Date.parse(snapshotTime);
	if (!Number.isFinite(snapMs)) return { ageSeconds: null, stale: false };
	const ageSeconds = Math.max(0, Math.round((now - snapMs) / 1000));
	return {
		ageSeconds,
		stale: ageSeconds > STALE_AFTER_SECONDS,
	};
}

function hasCurrentSeriesSample(
	metric: MetricResult | undefined,
	opts?: LatestValueOptions,
): boolean {
	if (!metric || metric.degraded) return false;
	for (const s of metric.series) {
		const pts = extractPointsFromSeries(s);
		if (latestPointValue(pts, opts) !== null) {
			return true;
		}
	}
	return false;
}

export function buildAttention(input: {
	metrics: MetricMap;
	streamState: string;
	snapshotTime: string | null;
	now: number;
	lastError: string | null;
	requestedKeys?: readonly string[];
	maxAgeSeconds: number;
}): AttentionItem[] {
	const items: AttentionItem[] = [];
	const hasSnapshot = Boolean(input.snapshotTime) && input.metrics.size > 0;
	const snapMs = input.snapshotTime ? Date.parse(input.snapshotTime) : null;
	const evalOpts: LatestValueOptions | undefined =
		snapMs !== null && Number.isFinite(snapMs)
			? { asOfMs: snapMs, maxAgeSeconds: input.maxAgeSeconds }
			: undefined;

	if (!hasSnapshot) {
		items.push({
			severity: "warn",
			section: "telemetry",
			anchor: "#attention",
			text: "No snapshot received yet",
		});
	}

	const alertRows = buildAlertRows(input.metrics);
	for (const alert of alertRows) {
		if (alert.isHeartbeat) continue;
		const sev = alertRowSeverity(alert);
		const target = alert.labels.job || alert.labels.service || alert.severity;
		items.push({
			severity: sev,
			section: "alerts",
			anchor: "#alerts",
			text: `Alert ${alert.alertname} (${alert.state}): ${target}`,
		});
	}

	if (hasSnapshot) {
		const alertAvail = alertsAvailability(input.metrics);
		if (!alertAvail.available) {
			items.push({
				severity: "warn",
				section: "telemetry",
				anchor: "#alerts",
				text: `Alert state unavailable${alertAvail.reason ? ` (${alertAvail.reason})` : ""}`,
			});
		} else if (!alertRows.some((a) => a.isHeartbeat && a.state === "firing")) {
			items.push({
				severity: "crit",
				section: "telemetry",
				anchor: "#alerts",
				text: "Watchdog heartbeat not firing — alert pipeline may be broken",
			});
		}
	}

	const edge = buildEdgeSignals(input.metrics, evalOpts);
	if (edge.pageBurn) {
		items.push({
			severity: "crit",
			section: "edge",
			anchor: "#edge",
			text: `Edge SLO page burn active (1h ${edge.burn.h1?.toFixed(1)}×, 5m ${edge.burn.m5?.toFixed(1)}× > ${BURN.pageThreshold}×)`,
		});
	} else if (edge.ticketBurn) {
		items.push({
			severity: "warn",
			section: "edge",
			anchor: "#edge",
			text: `Edge SLO ticket burn active (6h ${edge.burn.h6?.toFixed(1)}×, 30m ${edge.burn.m30?.toFixed(1)}× > ${BURN.ticketThreshold}×)`,
		});
	}
	if (
		edge.errorRatio !== null &&
		(edge.errorSeverity === "crit" || edge.errorSeverity === "warn")
	) {
		items.push({
			severity: edge.errorSeverity,
			section: "edge",
			anchor: "#edge",
			text: `Edge 5xx ratio ${(edge.errorRatio * 100).toFixed(2)}%`,
		});
	}
	if (
		edge.latencyP95 !== null &&
		(edge.latencySeverity === "crit" || edge.latencySeverity === "warn")
	) {
		items.push({
			severity: edge.latencySeverity,
			section: "edge",
			anchor: "#edge",
			text: `Edge p95 latency ${edge.latencyP95.toFixed(2)}s`,
		});
	}

	for (const svc of buildServiceRows(input.metrics, evalOpts)) {
		if (svc.severity === "crit" || svc.severity === "warn") {
			items.push({
				severity: svc.severity,
				section: "services",
				anchor: "#services",
				text: `${svc.service}: ${svc.reasons.join(", ") || svc.severity}`,
			});
		}
	}

	const res = buildHostResources(input.metrics, evalOpts);
	if (
		res.cpu !== null &&
		(res.cpuSeverity === "crit" || res.cpuSeverity === "warn")
	) {
		items.push({
			severity: res.cpuSeverity,
			section: "resources",
			anchor: "#resources",
			text: `Host CPU utilization ${(res.cpu * 100).toFixed(1)}%`,
		});
	}
	if (
		res.memory !== null &&
		(res.memorySeverity === "crit" || res.memorySeverity === "warn")
	) {
		items.push({
			severity: res.memorySeverity,
			section: "resources",
			anchor: "#resources",
			text: `Host memory utilization ${(res.memory * 100).toFixed(1)}%`,
		});
	}
	for (const fs of res.filesystems) {
		if (fs.severity === "crit" || fs.severity === "warn") {
			items.push({
				severity: fs.severity,
				section: "resources",
				anchor: "#resources",
				text: `Filesystem ${fs.device} utilization ${(fs.ratio * 100).toFixed(1)}%`,
			});
		}
	}

	for (const cert of buildCertRows(input.metrics, evalOpts)) {
		if (cert.severity === "crit" || cert.severity === "warn") {
			items.push({
				severity: cert.severity,
				section: "certificates",
				anchor: "#certificates",
				text:
					cert.healthy === false
						? `Certificate unhealthy for ${cert.job}`
						: `Certificate expiring soon for ${cert.job} (${Math.round(cert.remainingSeconds ?? 0)}s remaining)`,
			});
		}
	}

	const pipe = buildPipelines(input.metrics, evalOpts);
	if (pipe.redisUp === false) {
		items.push({
			severity: "crit",
			section: "pipelines",
			anchor: "#pipelines",
			text: "mq-hub Redis connection is down",
		});
	}
	for (const b of pipe.backlog) {
		if (b.severity === "crit" || b.severity === "warn") {
			items.push({
				severity: b.severity,
				section: "pipelines",
				anchor: "#pipelines",
				text: `Backlog ${b.queue} (${b.job}) oldest age ${Math.round(b.oldestAgeSeconds ?? 0)}s`,
			});
		}
	}
	if (pipe.outboxSeverity === "crit" || pipe.outboxSeverity === "warn") {
		items.push({
			severity: pipe.outboxSeverity,
			section: "pipelines",
			anchor: "#pipelines",
			text: `Outbox failed events (24h): ${pipe.outboxFailed24h}`,
		});
	}

	if (input.lastError) {
		items.push({
			severity: "crit",
			section: "telemetry",
			anchor: "#attention",
			text: `Stream error: ${input.lastError}`,
		});
	} else if (input.streamState === "degraded") {
		items.push({
			severity: "warn",
			section: "telemetry",
			anchor: "#attention",
			text: "Telemetry stream is degraded",
		});
	}

	const snapFreshness = freshness(input.snapshotTime, input.now);
	if (snapFreshness.stale) {
		items.push({
			severity: "warn",
			section: "telemetry",
			anchor: "#attention",
			text: `Snapshot is stale (${snapFreshness.ageSeconds}s old)`,
		});
	}

	if (hasSnapshot) {
		const recordingCoreSignals: ReadonlyArray<{
			key: string;
			anchor: "#edge" | "#resources" | "#services";
		}> = [
			{ key: "edge_rps", anchor: "#edge" },
			{ key: "host_cpu_utilization", anchor: "#resources" },
			{ key: "host_memory_utilization", anchor: "#resources" },
			{ key: "http_rps", anchor: "#services" },
		];
		const missingRecording = recordingCoreSignals.filter(
			({ key }) => !hasCurrentSeriesSample(input.metrics.get(key), evalOpts),
		);
		const firstMissingRecording = missingRecording[0];
		if (firstMissingRecording) {
			items.push({
				severity: "warn",
				section: "telemetry",
				anchor: firstMissingRecording.anchor,
				text: `No current samples for ${missingRecording.map((s) => s.key).join(", ")} — recording rules not loaded or not evaluating, or exporters down`,
			});
		}

		if (
			!hasCurrentSeriesSample(
				input.metrics.get("availability_services"),
				evalOpts,
			)
		) {
			items.push({
				severity: "warn",
				section: "telemetry",
				anchor: "#services",
				text: "No current samples for availability_services — scrape target availability metric unavailable or stale",
			});
		}

		const expectedKeys = input.requestedKeys ?? MONITOR_KEYS;
		const missingKeys = expectedKeys.filter((k) => !input.metrics.has(k));
		if (missingKeys.length > 0) {
			items.push({
				severity: "warn",
				section: "telemetry",
				anchor: "#attention",
				text: `Missing ${missingKeys.length} requested metric ${missingKeys.length === 1 ? "key" : "keys"}: ${missingKeys.join(", ")}`,
			});
		}
	}

	const degradedByReason = new Map<string, string[]>();
	for (const metric of input.metrics.values()) {
		if (!metric.degraded) continue;
		const reason = metric.reason?.trim() || "source degraded";
		let keys = degradedByReason.get(reason);
		if (!keys) {
			keys = [];
			degradedByReason.set(reason, keys);
		}
		keys.push(metric.key);
	}
	for (const [reason, keys] of degradedByReason) {
		keys.sort((a, b) => a.localeCompare(b));
		const prefix =
			keys.length === 1
				? `Degraded metric (${keys[0]})`
				: `Degraded metrics (${keys.length}: ${keys.join(", ")})`;
		items.push({
			severity: "warn",
			section: "telemetry",
			anchor: "#attention",
			text: `${prefix}: ${reason}`,
		});
	}

	items.sort((a, b) => {
		if (a.severity === b.severity) return 0;
		return a.severity === "crit" ? -1 : 1;
	});

	return items;
}

export type WindowOpt = "5m" | "15m" | "1h" | "6h" | "24h";
export type StepOpt = "15s" | "30s" | "1m" | "5m";

export function windowEnumToOpt(w: RangeWindow): WindowOpt {
	switch (w) {
		case RangeWindow.RANGE_WINDOW_5M:
			return "5m";
		case RangeWindow.RANGE_WINDOW_15M:
			return "15m";
		case RangeWindow.RANGE_WINDOW_1H:
			return "1h";
		case RangeWindow.RANGE_WINDOW_6H:
			return "6h";
		case RangeWindow.RANGE_WINDOW_24H:
			return "24h";
		case RangeWindow.RANGE_WINDOW_UNSPECIFIED:
			throw new Error(`Unsupported range window: ${w}`);
		default: {
			const exhaustive: never = w;
			throw new Error(`Unhandled range window: ${String(exhaustive)}`);
		}
	}
}

export function stepEnumToOpt(s: Step): StepOpt {
	switch (s) {
		case Step.STEP_15S:
			return "15s";
		case Step.STEP_30S:
			return "30s";
		case Step.STEP_1M:
			return "1m";
		case Step.STEP_5M:
			return "5m";
		case Step.STEP_UNSPECIFIED:
			throw new Error(`Unsupported step: ${s}`);
		default: {
			const exhaustive: never = s;
			throw new Error(`Unhandled step: ${String(exhaustive)}`);
		}
	}
}

export function optToStepEnum(opt: StepOpt): Step {
	switch (opt) {
		case "15s":
			return Step.STEP_15S;
		case "30s":
			return Step.STEP_30S;
		case "1m":
			return Step.STEP_1M;
		case "5m":
			return Step.STEP_5M;
		default: {
			const exhaustive: never = opt;
			throw new Error(`Unhandled step option: ${String(exhaustive)}`);
		}
	}
}

const WINDOW_SECONDS: Readonly<Record<WindowOpt, number>> = {
	"5m": 300,
	"15m": 900,
	"1h": 3_600,
	"6h": 21_600,
	"24h": 86_400,
};

const STEP_SECONDS: Readonly<Record<StepOpt, number>> = {
	"15s": 15,
	"30s": 30,
	"1m": 60,
	"5m": 300,
};

const STEP_OPTIONS: readonly StepOpt[] = ["15s", "30s", "1m", "5m"];

export function validSteps(window: WindowOpt): StepOpt[] {
	const ws = WINDOW_SECONDS[window];
	return STEP_OPTIONS.filter((s) => ws / STEP_SECONDS[s] <= 720);
}

export function clampStep(window: WindowOpt, step: StepOpt): StepOpt {
	const valid = validSteps(window);
	const first = valid[0];
	if (!first) {
		throw new Error(`No valid steps for window: ${window}`);
	}
	return valid.includes(step) ? step : first;
}

function stepSeconds(step: StepOpt): number {
	const seconds = STEP_SECONDS[step];
	if (seconds === undefined) {
		throw new Error(`Unhandled step option: ${String(step)}`);
	}
	return seconds;
}

export function maxPointAgeSeconds(step: StepOpt): number {
	return Math.max(2 * stepSeconds(step), 300);
}

export const MONITOR_KEYS: readonly string[] = [
	"availability_services",
	"prometheus_scrape_lag",
	"scrape_duration",
	"http_rps",
	"http_error_ratio",
	"http_latency_p50",
	"http_latency_p95",
	"http_latency_p99",
	"edge_rps",
	"edge_error_ratio",
	"edge_latency_p95",
	"edge_burn_5m",
	"edge_burn_30m",
	"availability_burn_1h",
	"availability_burn_6h",
	"host_cpu_utilization",
	"host_memory_utilization",
	"host_filesystem_utilization",
	"alerts_firing",
	"alerts_pending",
	"alerts_active_since",
	"cert_remaining",
	"cert_healthy",
	"mqhub_redis",
	"mqhub_publish_rate",
	"backlog_oldest_age",
	"push_pending_count",
	"outbox_failed_24h",
] as const;
