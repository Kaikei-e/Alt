<script lang="ts">
import type { MetricResult } from "$lib/gen/alt/admin_monitor/v1/admin_monitor_pb";
import ServiceDrilldown from "./ServiceDrilldown.svelte";
import { formatValue, stateBadge } from "./format";

let {
	metrics,
}: {
	metrics: MetricResult[];
} = $props();

let expanded = $state<string | null>(null);

function find(key: string): MetricResult | undefined {
	return metrics.find((m) => m.key === key);
}

interface Row {
	job: string;
	p50: number | null;
	p95: number | null;
	p99: number | null;
	rps: number | null;
	err: number | null;
	up: number | null;
}

const rows = $derived.by(() => {
	const byJob: Record<string, Row> = {};
	const ensure = (job: string): Row => {
		if (!byJob[job]) {
			byJob[job] = {
				job,
				p50: null,
				p95: null,
				p99: null,
				rps: null,
				err: null,
				up: null,
			};
		}
		return byJob[job];
	};

	function latestByJob(
		metric: MetricResult | undefined,
	): Record<string, number> {
		const out: Record<string, number> = {};
		for (const s of metric?.series ?? []) {
			const job = s.labels?.job ?? "(unknown)";
			const last = s.points.at(-1)?.value;
			if (last != null && Number.isFinite(last)) out[job] = last;
		}
		return out;
	}

	const p50 = latestByJob(find("http_latency_p50"));
	const p95 = latestByJob(find("http_latency_p95"));
	const p99 = latestByJob(find("http_latency_p99"));
	const rps = latestByJob(find("http_rps"));
	const err = latestByJob(find("http_error_ratio"));
	const up = latestByJob(find("availability_services"));

	for (const [job, v] of Object.entries(p50)) ensure(job).p50 = v;
	for (const [job, v] of Object.entries(p95)) ensure(job).p95 = v;
	for (const [job, v] of Object.entries(p99)) ensure(job).p99 = v;
	for (const [job, v] of Object.entries(rps)) ensure(job).rps = v;
	for (const [job, v] of Object.entries(err)) ensure(job).err = v;
	for (const [job, v] of Object.entries(up)) ensure(job).up = v;

	return Object.values(byJob).sort((a, b) => a.job.localeCompare(b.job));
});

function rowState(r: Row): "ok" | "warn" | "down" | "missing" {
	if (r.up === 0) return "down";
	if (r.up == null && r.rps == null) return "missing";
	if (r.err != null && r.err > 0.01) return "warn";
	if (r.p95 != null && r.p95 > 1.0) return "warn";
	return "ok";
}

function rowGlyph(state: "ok" | "warn" | "down" | "missing"): string {
	switch (state) {
		case "ok":
			return "▲";
		case "warn":
			return "●";
		case "down":
			return "▼";
		case "missing":
			return "○";
	}
}

function toggle(job: string) {
	expanded = expanded === job ? null : job;
}
</script>

<section class="red" aria-label="Service RED">
	<h2 class="section-head">Service RED — rate / errors / duration</h2>
	{#if rows.length === 0}
		<p class="empty">No scraped services yet. Waiting for first stream tick.</p>
	{:else}
		<table>
			<thead>
				<tr>
					<th scope="col" class="status-col">State</th>
					<th scope="col" class="job-col">Service</th>
					<th scope="col" class="num-col">p50</th>
					<th scope="col" class="num-col">p95</th>
					<th scope="col" class="num-col">p99</th>
					<th scope="col" class="num-col">req/s</th>
					<th scope="col" class="num-col">err %</th>
				</tr>
			</thead>
			<tbody>
				{#each rows as r (r.job)}
					{@const state = rowState(r)}
					<tr
						class="row"
						data-state={state}
						class:expanded={expanded === r.job}
					>
						<td>
							<button
								type="button"
								class="status-btn"
								aria-expanded={expanded === r.job}
								aria-controls="drill-{r.job}"
								onclick={() => toggle(r.job)}
							>
								<span class="glyph" aria-hidden="true">{rowGlyph(state)}</span>
								<span class="state-text">{state}</span>
							</button>
						</td>
						<td class="job">{r.job}</td>
						<td class="num">{formatValue(r.p50, "seconds")}</td>
						<td class="num">{formatValue(r.p95, "seconds")}</td>
						<td class="num">{formatValue(r.p99, "seconds")}</td>
						<td class="num">{formatValue(r.rps, "req/s")}</td>
						<td class="num">{formatValue(r.err, "ratio")}</td>
					</tr>
					{#if expanded === r.job}
						<tr class="drill-row">
							<td colspan="7" id="drill-{r.job}">
								<ServiceDrilldown job={r.job} {metrics} />
							</td>
						</tr>
					{/if}
				{/each}
			</tbody>
		</table>
	{/if}
</section>

<style>
	.red {
		display: grid;
		gap: 0.5rem;
	}

	.section-head {
		font-family: var(--font-display, var(--font-serif));
		font-size: 0.92rem;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		margin: 0;
		color: var(--alt-charcoal);
	}

	table {
		width: 100%;
		border-collapse: collapse;
		font-variant-numeric: tabular-nums;
	}

	thead th {
		font-family: var(--font-body);
		font-size: 0.66rem;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--alt-slate);
		text-align: left;
		padding: 0.4rem 0.6rem 0.4rem 0;
		border-bottom: 1px solid var(--alt-charcoal);
	}

	.num-col {
		text-align: right;
	}

	tbody td {
		padding: 0.55rem 0.6rem 0.55rem 0;
		font-size: 0.84rem;
		color: var(--alt-charcoal);
		border-bottom: 0.5px solid var(--obs-rule, var(--surface-border));
	}

	.num {
		text-align: right;
		font-family: var(--font-mono);
	}

	.job {
		font-family: var(--font-body);
		font-weight: 500;
	}

	.status-btn {
		background: none;
		border: none;
		padding: 0;
		display: inline-flex;
		gap: 0.35rem;
		align-items: baseline;
		cursor: pointer;
		font-family: var(--font-mono);
		font-size: 0.72rem;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: inherit;
	}

	.row[data-state="warn"] .glyph,
	.row[data-state="warn"] .state-text {
		color: var(--obs-warn, var(--alt-warning));
	}

	.row[data-state="down"] .glyph,
	.row[data-state="down"] .state-text {
		color: var(--obs-critical, var(--alt-error));
	}

	.row[data-state="missing"] {
		color: var(--obs-muted, var(--alt-ash));
	}

	.drill-row td {
		background: color-mix(in oklch, var(--surface) 96%, var(--alt-charcoal) 4%);
		padding: 0.6rem 0.6rem 0.9rem;
	}

	.empty {
		color: var(--alt-ash);
		font-style: italic;
	}

	@media (max-width: 900px) {
		table {
			display: block;
			overflow-x: auto;
		}
	}
</style>
