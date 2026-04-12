<script lang="ts">
import { onDestroy } from "svelte";
import { useConnectAdminMetrics } from "$lib/hooks/useConnectAdminMetrics.svelte";
import type { MetricResult } from "$lib/gen/alt/admin_monitor/v1/admin_monitor_pb";
import MetricRow from "./MetricRow.svelte";
import ServiceHealthTable from "./ServiceHealthTable.svelte";
import DegradedBanner from "./DegradedBanner.svelte";

const stream = useConnectAdminMetrics();
stream.start();
onDestroy(() => stream.stop());

function byKey(key: string): MetricResult | undefined {
	return stream.metrics.find((m) => m.key === key);
}

// Thresholds are a conservative first pass; tune per service SLO later.
const latencyThreshold = { warn: (v: number) => v > 1.0, value: 1.0 }; // p95 > 1s
const errorRateThreshold = { warn: (v: number) => v > 0.01, value: 0.01 }; // > 1%
const dbPoolThreshold = { warn: (v: number) => v > 20 }; // > 20 conns
const recapRequestThreshold = { warn: (v: number) => v > 3.0, value: 3.0 }; // p95 > 3s
const redisThreshold = { warn: (v: number) => v < 1 }; // disconnected

const allDegraded = $derived(
	() => stream.metrics.length > 0 && stream.metrics.every((m) => m.degraded),
);

const showBanner = $derived(
	() =>
		allDegraded() ||
		stream.state === "degraded" ||
		stream.state === "connecting",
);
</script>

<section class="observability" aria-label="Admin observability">
	<header class="head">
		<h2>Observability</h2>
		<p class="sub">
			Near-real-time snapshot of scraped services. Everything you need to
			notice something is wrong lives on this page; per-row Grafana links
			appear on hover for deeper investigation.
		</p>
	</header>

	{#if showBanner()}
		<DegradedBanner
			message={stream.state === "connecting"
				? "Connecting to observability stream…"
				: "Observability degraded."}
			hint={stream.lastError ?? "The stream is retrying in the background."}
		/>
	{/if}

	<h3 class="section-head">Golden signals</h3>
	<MetricRow label="HTTP p95 latency" metric={byKey("http_latency_p95")} threshold={latencyThreshold} preferLabel="job" />
	<MetricRow label="HTTP traffic"     metric={byKey("http_rps")} preferLabel="job" />
	<MetricRow label="HTTP 5xx ratio"   metric={byKey("http_error_ratio")} threshold={errorRateThreshold} preferLabel="job" />

	<h3 class="section-head">Container resources</h3>
	<MetricRow label="CPU saturation"   metric={byKey("cpu_saturation")} preferLabel="name" />
	<MetricRow label="Memory RSS"       metric={byKey("memory_rss")} preferLabel="name" />

	<h3 class="section-head">mq-hub</h3>
	<MetricRow label="Publish rate"        metric={byKey("mqhub_publish_rate")} preferLabel="topic" />
	<MetricRow label="Redis connection"    metric={byKey("mqhub_redis")} threshold={redisThreshold} />

	<h3 class="section-head">Recap pipeline</h3>
	<MetricRow label="DB pool in-use"            metric={byKey("recap_db_pool_in_use")} threshold={dbPoolThreshold} preferLabel="pool" />
	<MetricRow label="recap-worker RSS"          metric={byKey("recap_worker_rss")} />
	<MetricRow label="Request p95"               metric={byKey("recap_request_p95")} threshold={recapRequestThreshold} />
	<MetricRow label="Subworker admin job rate"  metric={byKey("recap_subworker_admin_success")} preferLabel="status" />

	<h3 class="section-head">Services</h3>
	<ServiceHealthTable availability={byKey("availability_services")} />

	<footer class="foot">
		<span class="muted">
			{#if stream.snapshotTime}
				Last snapshot {new Date(stream.snapshotTime).toLocaleTimeString()}
			{:else}
				Awaiting first snapshot…
			{/if}
		</span>
		<span class="muted right" aria-live="polite">{stream.state}</span>
	</footer>
</section>

<style>
	/* Observability-scoped semantic tokens. Cover every state signal used in
	 * child components — none of these need to change when the global theme
	 * shifts. Alt-Paper theme already supplies editorial ink values for
	 * --alt-success / --alt-warning / --alt-error via app.css.
	 */
	.observability {
		--obs-good: var(--alt-success);
		--obs-warn: var(--alt-warning);
		--obs-critical: var(--alt-error);
		--obs-critical-accent: #b4231f;
		--obs-muted: #6b655c;
		--obs-rule: #cfc8bb;
		--obs-rule-strong: #b4ab9b;
		--obs-spark-stroke: #3b3630;
		--obs-spark-threshold: var(--obs-warn);
		--obs-series-1: #0072b2;
		--obs-series-2: #d55e00;
		--obs-series-3: #007a5a;
		--obs-series-4: #5b2f84;
		--obs-dot-up: var(--alt-charcoal);
		--obs-dot-down: var(--obs-critical);

		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		padding: 1rem 0;
		color: var(--alt-charcoal);
	}

	.head h2 {
		font-family: var(--font-display, var(--font-serif));
		font-weight: 700;
		font-size: 1.35rem;
		letter-spacing: 0.02em;
		margin: 0 0 0.15rem 0;
	}

	.head .sub {
		margin: 0;
		color: var(--alt-slate);
		font-size: 0.82rem;
		max-width: 70ch;
	}

	.section-head {
		margin: 1.25rem 0 0.1rem 0;
		font-size: 0.7rem;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--alt-slate);
		border-top: 1px solid var(--obs-rule-strong);
		padding-top: 0.55rem;
	}

	.foot {
		display: flex;
		justify-content: space-between;
		padding-top: 0.65rem;
		border-top: 1px solid var(--obs-rule-strong);
		font-size: 0.72rem;
		color: var(--alt-slate);
	}

	.muted {
		color: var(--alt-ash);
		font-variant-numeric: tabular-nums;
	}

	.right {
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}
</style>
