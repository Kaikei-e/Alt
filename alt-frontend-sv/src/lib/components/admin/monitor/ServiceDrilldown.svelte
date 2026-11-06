<script lang="ts">
import { formatValue } from "./format";
import MonitorSparkline from "./MonitorSparkline.svelte";
import {
	type MetricMap,
	type ServiceRow,
	seriesPoints,
	THRESHOLDS,
} from "./model";

let {
	row,
	metrics,
}: {
	row: ServiceRow;
	metrics: MetricMap;
} = $props();

function pointsForService(key: string): { t: number; v: number }[] {
	const m = metrics.get(key);
	if (!m) return [];
	for (const job of row.jobs) {
		const pts = seriesPoints(m, { job });
		if (pts.length > 0) return pts;
	}
	return [];
}

const p50Points = $derived(pointsForService("http_latency_p50"));
const p95Points = $derived(pointsForService("http_latency_p95"));
const p99Points = $derived(pointsForService("http_latency_p99"));
const rpsPoints = $derived(pointsForService("http_rps"));
const errPoints = $derived(pointsForService("http_error_ratio"));
</script>

<div class="drill" data-testid="service-drilldown">
	<div class="meta-bar">
		<div class="meta-group">
			<span class="meta-key">Jobs:</span>
			{#each row.jobScrapes as js (js.job)}
				<span class="meta-val">
					{js.job} (age {js.scrapeAgeSeconds !== null ? `${Math.round(js.scrapeAgeSeconds)}s` : "no data"} · scrape {js.scrapeDurationSeconds !== null ? formatValue(js.scrapeDurationSeconds, "seconds") : "no data"})
				</span>
			{/each}
			{#if row.signal}
				<span class="meta-key">Signal:</span>
				<span class="meta-val">{row.signal}</span>
			{/if}
		</div>
		{#if row.reasons.length > 0}
			<ul
				class="reasons"
				data-severity={row.severity}
				data-testid="drilldown-reasons"
			>
				{#each row.reasons as reason (reason)}
					<li>{reason}</li>
				{/each}
			</ul>
		{:else if row.severity === "ok" && row.redMetricsAvailable}
			<span class="meta-ok">All thresholds passing</span>
		{:else if row.severity === "ok"}
			<span class="dim">Targets up · RED unknown</span>
		{:else}
			<span class="dim">NO DATA — target availability unknown</span>
		{/if}
	</div>

	{#if !row.instrumented}
		<p class="dim">
			{row.redMetricsAvailable
				? "This service has no request instrumentation (no request metrics)."
				: "NO DATA — request metrics unavailable."}
		</p>
	{:else}
		<div class="grid">
			<div class="panel">
				<h3>Latency p50 / p95 / p99</h3>
				<div class="spark-list">
					<div class="spark-item">
						<div class="spark-head">
							<span>p50</span>
							<span class:dim={row.p50 === null}>
								{row.p50 !== null ? formatValue(row.p50, "seconds") : "no data"}
							</span>
						</div>
						<MonitorSparkline
							points={p50Points}
							label="{row.service} p50 latency"
							unit="seconds"
							width={240}
							height={30}
							showAxisLabels={true}
						/>
					</div>

					<div class="spark-item">
						<div class="spark-head">
							<span>p95</span>
							<span class:dim={row.p95 === null}>
								{row.p95 !== null ? formatValue(row.p95, "seconds") : "no data"}
							</span>
						</div>
						<MonitorSparkline
							points={p95Points}
							label="{row.service} p95 latency"
							unit="seconds"
							threshold={THRESHOLDS.serviceLatencyP95.warn}
							width={240}
							height={30}
							showAxisLabels={true}
						/>
					</div>

					<div class="spark-item">
						<div class="spark-head">
							<span>p99</span>
							<span class:dim={row.p99 === null}>
								{row.p99 !== null ? formatValue(row.p99, "seconds") : "no data"}
							</span>
						</div>
						<MonitorSparkline
							points={p99Points}
							label="{row.service} p99 latency"
							unit="seconds"
							width={240}
							height={30}
							showAxisLabels={true}
						/>
					</div>
				</div>
			</div>

			<div class="panel">
				<h3>Request rate (req/s)</h3>
				<div class="spark-item">
					<div class="spark-head">
						<span>req/s</span>
						<span class:dim={row.rps === null}>
							{row.rps !== null ? `${formatValue(row.rps, "req/s")} req/s` : "no data"}
						</span>
					</div>
					<MonitorSparkline
						points={rpsPoints}
						label="{row.service} request rate"
						unit="req/s"
						width={240}
						height={36}
						showAxisLabels={true}
					/>
				</div>
			</div>

			<div class="panel">
				<h3>Error ratio (5xx)</h3>
				<div class="spark-item">
					<div class="spark-head">
						<span>5xx ratio</span>
						<span class:dim={row.errorRatio === null}>
							{row.errorRatio !== null
								? formatValue(row.errorRatio, "ratio")
								: "no data"}
						</span>
					</div>
					<MonitorSparkline
						points={errPoints}
						label="{row.service} 5xx ratio"
						unit="ratio"
						threshold={THRESHOLDS.serviceErrorRatio.warn}
						width={240}
						height={36}
						showAxisLabels={true}
					/>
				</div>
			</div>
		</div>
	{/if}
</div>

<style>
	.drill {
		display: grid;
		gap: 0.6rem;
		font-size: 0.82rem;
		min-width: 0;
	}

	.meta-bar {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		flex-wrap: wrap;
		gap: 0.6rem;
		padding-bottom: 0.4rem;
		border-bottom: 0.5px solid var(--obs-rule, var(--surface-border));
	}

	.meta-group {
		display: flex;
		align-items: baseline;
		flex-wrap: wrap;
		gap: 0.45rem;
		font-family: var(--font-mono);
		font-size: 0.72rem;
	}

	.meta-key {
		color: var(--alt-slate);
		text-transform: uppercase;
		letter-spacing: 0.08em;
		font-size: 0.66rem;
	}

	.meta-val {
		color: var(--alt-charcoal);
	}

	.meta-ok {
		font-family: var(--font-mono);
		font-size: 0.72rem;
		color: var(--obs-good, var(--alt-success));
	}

	.reasons {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-wrap: wrap;
		gap: 0.6rem;
		font-family: var(--font-mono);
		font-size: 0.74rem;
		font-weight: 600;
		color: var(--obs-warn, var(--alt-warning));
	}

	.reasons[data-severity="crit"] {
		color: var(--obs-critical, var(--alt-error));
	}

	.reasons[data-severity="warn"] {
		color: var(--obs-warn, var(--alt-warning));
	}

	.grid {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 0.7rem;
		min-width: 0;
	}

	@media (max-width: 1100px) {
		.grid {
			grid-template-columns: 1fr;
		}
	}

	.panel {
		display: grid;
		gap: 0.45rem;
		padding: 0.6rem 0.75rem;
		border: 0.5px solid var(--obs-rule, var(--surface-border));
		background: var(--surface-bg);
		min-width: 0;
	}

	.panel h3 {
		font-family: var(--font-body);
		font-size: 0.66rem;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--alt-slate);
		margin: 0;
	}

	.spark-list {
		display: grid;
		gap: 0.45rem;
	}

	.spark-item {
		display: grid;
		gap: 0.2rem;
		min-width: 0;
	}

	.spark-head {
		display: flex;
		justify-content: space-between;
		font-family: var(--font-mono);
		font-size: 0.74rem;
		color: var(--alt-charcoal);
		font-variant-numeric: tabular-nums;
	}

	.dim {
		margin: 0;
		font-family: var(--font-mono);
		font-size: 0.74rem;
		color: var(--obs-muted, var(--alt-ash));
	}
</style>
