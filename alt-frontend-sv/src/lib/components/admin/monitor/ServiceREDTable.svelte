<script lang="ts">
import { formatValue } from "./format";
import {
	badge,
	type MetricMap,
	type ServiceGroup,
	type ServiceRow,
	THRESHOLDS,
} from "./model";
import ServiceDrilldown from "./ServiceDrilldown.svelte";

let {
	metrics,
	rows,
}: {
	metrics: MetricMap;
	rows: ServiceRow[];
} = $props();

let onlyIssues = $state(false);
let expanded = $state<string | null>(null);

const visibleRows = $derived(
	onlyIssues ? rows.filter((r) => r.severity !== "ok") : rows,
);

const groupedRows = $derived.by(() => {
	const groups: Array<{ group: ServiceGroup; items: ServiceRow[] }> = [];
	const byGroup = new Map<ServiceGroup, ServiceRow[]>();
	for (const r of visibleRows) {
		let list = byGroup.get(r.group);
		if (!list) {
			list = [];
			byGroup.set(r.group, list);
			groups.push({ group: r.group, items: list });
		}
		list.push(r);
	}
	return groups;
});

function toggle(service: string) {
	expanded = expanded === service ? null : service;
}

function stateAriaLabel(r: ServiceRow): string {
	const b = badge(r.severity);
	if (r.reasons.length === 0) return b.label;
	return `${b.label}: ${r.reasons.join("; ")}`;
}

function formatScrapeAge(sec: number | null): string {
	if (sec === null || !Number.isFinite(sec)) return "no data";
	return `${Math.round(sec)}s`;
}
</script>

<section id="services" class="red" aria-label="Services">
	<header class="section-bar">
		<div class="title-wrap">
			<h2 class="section-head">Services — Targets &amp; RED</h2>
			<span class="thresholds-meta">
				5xx {THRESHOLDS.serviceErrorRatio.display} · p95 {THRESHOLDS.serviceLatencyP95.display} · scrape age {THRESHOLDS.scrapeAge.display}
			</span>
		</div>

		<label class="issue-toggle">
			<input type="checkbox" bind:checked={onlyIssues} />
			<span>Show only issues</span>
		</label>
	</header>

	<div class="table-wrap">
		{#if visibleRows.length === 0}
			<p class="empty">
				{onlyIssues
					? "No services with active issues."
					: metrics.size === 0
						? "No scraped services yet. Waiting for first stream tick."
						: "NO DATA — target availability unavailable"}
			</p>
		{:else}
			<table>
				<thead>
					<tr>
						<th scope="col" class="status-col">State</th>
						<th scope="col" class="job-col">Service</th>
						<th scope="col" class="num-col">Targets</th>
						<th scope="col" class="num-col">Scrape age</th>
						<th scope="col" class="num-col">req/s</th>
						<th scope="col" class="num-col">5xx</th>
						<th scope="col" class="num-col">p95</th>
					</tr>
				</thead>
				<tbody>
					{#each groupedRows as g (g.group)}
						<tr class="group-row">
							<th colspan="7" scope="colgroup">{g.group}</th>
						</tr>
						{#each g.items as r (r.service)}
							{@const b = badge(r.severity)}
							{@const isExpanded = expanded === r.service}
							<tr
								class="row"
								data-service={r.service}
								data-state={b.severity}
								class:expanded={isExpanded}
								onclick={() => toggle(r.service)}
							>
								<td class="state-cell" aria-label={stateAriaLabel(r)}>
									<span class="state-badge" data-severity={b.severity}>
										<span class="glyph" aria-hidden="true">{b.glyph}</span>
										<span class="state-text">{b.label}</span>
									</span>
								</td>
								<td class="job">
									<button
										type="button"
										class="svc-toggle"
										aria-expanded={isExpanded}
										aria-controls="drill-{r.service}"
										onclick={(e) => {
											e.stopPropagation();
											toggle(r.service);
										}}
									>
										<span class="svc-name">{r.service}</span>
									</button>
									{#if r.reasons.length > 0}
										<span class="inline-reason">{r.reasons.join(" · ")}</span>
									{/if}
								</td>
								<td class="num">{r.targetsUp}/{r.targetsTotal}</td>
								<td class="num">{formatScrapeAge(r.scrapeAgeSeconds)}</td>
								{#if r.instrumented}
									<td class="num" class:no-metrics={r.rps === null}>
										{r.rps !== null ? formatValue(r.rps, "req/s") : "no data"}
									</td>
									<td class="num" class:no-metrics={r.errorRatio === null}>
										{r.errorRatio !== null
											? formatValue(r.errorRatio, "ratio")
											: "no data"}
									</td>
									<td class="num" class:no-metrics={r.p95 === null}>
										{r.p95 !== null
											? formatValue(r.p95, "seconds")
											: "no data"}
									</td>
								{:else}
									{@const uninstrumentedText = r.redMetricsAvailable
										? "no request metrics"
										: "no data"}
									<td class="num no-metrics">{uninstrumentedText}</td>
									<td class="num no-metrics">{uninstrumentedText}</td>
									<td class="num no-metrics">{uninstrumentedText}</td>
								{/if}
							</tr>
							<tr class="drill-row" hidden={!isExpanded}>
								<td colspan="7" id="drill-{r.service}">
									{#if isExpanded}
										<ServiceDrilldown row={r} {metrics} />
									{/if}
								</td>
							</tr>
						{/each}
					{/each}
				</tbody>
			</table>
		{/if}
	</div>
</section>

<style>
	.red {
		display: grid;
		gap: 0.55rem;
		min-width: 0;
	}

	.section-bar {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		flex-wrap: wrap;
		gap: 0.6rem;
		border-top: 1px solid var(--obs-rule-strong, var(--surface-border));
		padding-top: 0.55rem;
	}

	.title-wrap {
		display: flex;
		align-items: baseline;
		flex-wrap: wrap;
		gap: 0.6rem;
	}

	.section-head {
		font-family: var(--font-display, var(--font-serif));
		font-size: 0.92rem;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		margin: 0;
		color: var(--alt-charcoal);
	}

	.thresholds-meta {
		font-family: var(--font-mono);
		font-size: 0.68rem;
		color: var(--alt-slate);
	}

	.issue-toggle {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		font-family: var(--font-mono);
		font-size: 0.72rem;
		color: var(--alt-charcoal);
		cursor: pointer;
	}

	.table-wrap {
		width: 100%;
		max-width: 100%;
		overflow-x: auto;
		min-width: 0;
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
		white-space: nowrap;
	}

	.num-col {
		text-align: right;
	}

	.group-row th {
		font-family: var(--font-mono);
		font-size: 0.68rem;
		font-weight: 600;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--alt-slate);
		text-align: left;
		padding: 0.55rem 0.6rem 0.25rem 0;
		border-bottom: 0.5px solid var(--obs-rule-strong, var(--surface-border));
		background: color-mix(in oklch, var(--surface-bg) 96%, var(--alt-charcoal) 4%);
	}

	.row {
		cursor: pointer;
	}

	.row:hover {
		background: var(--surface-hover, rgba(0, 0, 0, 0.03));
	}

	tbody td {
		padding: 0.5rem 0.6rem 0.5rem 0;
		font-size: 0.82rem;
		color: var(--alt-charcoal);
		border-bottom: 0.5px solid var(--obs-rule, var(--surface-border));
		vertical-align: baseline;
	}

	.num {
		text-align: right;
		font-family: var(--font-mono);
		white-space: nowrap;
	}

	.no-metrics {
		color: var(--obs-muted, var(--alt-ash));
		font-size: 0.72rem;
	}

	.job {
		font-family: var(--font-body);
		font-weight: 500;
	}

	.svc-toggle {
		all: unset;
		cursor: pointer;
		display: inline-flex;
		align-items: baseline;
	}

	.svc-toggle:focus-visible {
		outline: 1.5px solid var(--alt-charcoal);
		outline-offset: 2px;
	}

	.svc-name {
		font-family: var(--font-mono);
		font-size: 0.8rem;
	}

	.inline-reason {
		display: block;
		font-family: var(--font-mono);
		font-size: 0.68rem;
		font-weight: 400;
		color: var(--alt-slate);
	}

	.state-badge {
		display: inline-flex;
		gap: 0.3rem;
		align-items: baseline;
		font-family: var(--font-mono);
		font-size: 0.72rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		white-space: nowrap;
	}

	.state-badge[data-severity="ok"] {
		color: var(--obs-good, var(--alt-success));
	}

	.state-badge[data-severity="warn"] {
		color: var(--obs-warn, var(--alt-warning));
	}

	.state-badge[data-severity="crit"] {
		color: var(--obs-critical, var(--alt-error));
	}

	.state-badge[data-severity="nodata"],
	.state-badge[data-severity="stale"] {
		color: var(--obs-muted, var(--alt-ash));
	}

	.drill-row td {
		background: color-mix(in oklch, var(--surface-bg) 96%, var(--alt-charcoal) 4%);
		padding: 0.65rem 0.75rem 0.9rem;
	}

	.empty {
		color: var(--alt-ash);
		font-style: italic;
		margin: 0.4rem 0;
	}
</style>
