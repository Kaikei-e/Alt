/**
 * useConnectAdminMetrics — Connect-RPC server-streaming hook for the Admin
 * Observability panel.
 *
 * Design notes (derived from connectrpc.com + web research):
 *  - Connect-Web does not auto-reconnect; we implement exponential backoff with
 *    full jitter capped at 30s so thundering-herd reconnects are avoided.
 *  - Each stream is bounded to 15 min deadlines (+ ±60s jitter) so intermediate
 *    proxies' idle kill timers cannot silently stall the stream.
 *  - Runes: $state holds UI data, $effect is NOT used here since we drive the
 *    lifecycle imperatively from start/stop, invoked by $effect in the caller.
 */

import { createClientTransport } from "$lib/connect/transport-client";
import {
	createAdminMonitorClient,
	type AdminMonitorClient,
} from "$lib/connect/admin_monitor";
import {
	RangeWindow,
	Step,
	type WatchResponse,
	type MetricResult,
	type CatalogEntry,
} from "$lib/gen/alt/admin_monitor/v1/admin_monitor_pb";

export type StreamState =
	| "idle"
	| "connecting"
	| "live"
	| "degraded"
	| "closed";

export interface UseConnectAdminMetricsOptions {
	keys?: string[];
	window?: RangeWindow;
	step?: Step;
	/** Upper bound per-stream lifetime in ms; default 15min. */
	maxStreamMs?: number;
	/** Initial backoff on reconnect (ms). */
	initialBackoffMs?: number;
	/** Cap on reconnect backoff (ms). */
	maxBackoffMs?: number;
}

export interface UseConnectAdminMetricsResult {
	snapshotTime: string;
	metrics: MetricResult[];
	catalog: CatalogEntry[];
	state: StreamState;
	lastError: string | null;
	start: () => void;
	stop: () => void;
}

const DEFAULT_KEYS = [
	"availability_services",
	"http_latency_p95",
	"http_rps",
	"http_error_ratio",
	"cpu_saturation",
	"memory_rss",
	"mqhub_publish_rate",
	"mqhub_redis",
	"recap_db_pool_in_use",
	"recap_worker_rss",
	"recap_request_p95",
	"recap_subworker_admin_success",
];

export function useConnectAdminMetrics(
	opts: UseConnectAdminMetricsOptions = {},
): UseConnectAdminMetricsResult {
	const maxStreamMs = opts.maxStreamMs ?? 15 * 60 * 1000;
	const initial = opts.initialBackoffMs ?? 1_000;
	const cap = opts.maxBackoffMs ?? 30_000;

	let snapshotTime = $state("");
	let metrics = $state<MetricResult[]>([]);
	let catalog = $state<CatalogEntry[]>([]);
	let state = $state<StreamState>("idle");
	let lastError = $state<string | null>(null);

	let abort: AbortController | null = null;
	let rotateTimer: ReturnType<typeof setTimeout> | null = null;
	let backoffTimer: ReturnType<typeof setTimeout> | null = null;
	let attempt = 0;
	let stopped = false;
	let client: AdminMonitorClient | null = null;

	function ensureClient(): AdminMonitorClient {
		if (!client) {
			client = createAdminMonitorClient(createClientTransport());
		}
		return client;
	}

	async function fetchCatalogOnce() {
		try {
			const c = ensureClient();
			const res = await c.catalog({});
			catalog = res.entries;
		} catch (err) {
			lastError = String(err);
		}
	}

	function jitter(baseMs: number): number {
		// Full jitter: random in [0, baseMs].
		return Math.floor(Math.random() * baseMs);
	}

	function scheduleRotate() {
		if (rotateTimer) clearTimeout(rotateTimer);
		const rotateAfter =
			maxStreamMs + Math.floor((Math.random() - 0.5) * 120_000);
		rotateTimer = setTimeout(() => {
			if (stopped) return;
			if (abort) abort.abort();
		}, rotateAfter);
	}

	async function runOnce() {
		if (stopped) return;
		const controller = new AbortController();
		abort = controller;
		state = attempt === 0 ? "connecting" : "degraded";

		scheduleRotate();
		try {
			const c = ensureClient();
			const req = {
				keys: opts.keys ?? DEFAULT_KEYS,
				window: opts.window ?? RangeWindow.RANGE_WINDOW_UNSPECIFIED,
				step: opts.step ?? Step.STEP_UNSPECIFIED,
			} as const;

			for await (const msg of c.watch(req, {
				signal: controller.signal,
			}) as AsyncIterable<WatchResponse>) {
				if (stopped) break;
				snapshotTime = msg.time;
				metrics = msg.metrics;
				state = "live";
				lastError = null;
				attempt = 0;
			}
		} catch (err) {
			if (controller.signal.aborted && stopped) return;
			lastError = String(err);
			state = "degraded";
		} finally {
			if (rotateTimer) {
				clearTimeout(rotateTimer);
				rotateTimer = null;
			}
		}

		if (stopped) return;
		attempt += 1;
		const base = Math.min(cap, initial * 2 ** (attempt - 1));
		const delay = jitter(base);
		backoffTimer = setTimeout(runOnce, delay);
	}

	function start() {
		if (!stopped && state !== "idle" && state !== "closed") return;
		stopped = false;
		state = "connecting";
		lastError = null;
		attempt = 0;
		void fetchCatalogOnce();
		void runOnce();
	}

	function stop() {
		stopped = true;
		if (abort) abort.abort();
		if (rotateTimer) {
			clearTimeout(rotateTimer);
			rotateTimer = null;
		}
		if (backoffTimer) {
			clearTimeout(backoffTimer);
			backoffTimer = null;
		}
		state = "closed";
	}

	return {
		get snapshotTime() {
			return snapshotTime;
		},
		get metrics() {
			return metrics;
		},
		get catalog() {
			return catalog;
		},
		get state() {
			return state;
		},
		get lastError() {
			return lastError;
		},
		start,
		stop,
	};
}
