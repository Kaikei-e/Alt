<script lang="ts">
import type { MetricResult } from "$lib/gen/alt/admin_monitor/v1/admin_monitor_pb";
import { type SimpleSeries, serviceHealthRows } from "./util";

// notInstrumented is still a list because it is the one thing the series
// cannot tell us: these services have no scrape job at all, so they never
// produce an `up` sample and must be labelled as uninstrumented rather than
// left off the table where they would read as "nothing to worry about".
let {
	availability,
	notInstrumented = ["pre-processor", "auth-hub", "rask-log-aggregator"],
}: {
	availability: MetricResult | undefined;
	notInstrumented?: string[];
} = $props();

// Rows come from the series, not from a job list kept here: a hardcoded list
// drops any job it has not been taught about, so a fully-down service renders
// as no row at all instead of a red one. See serviceHealthRows in ./util.
const rows = $derived(() =>
	serviceHealthRows(
		(availability?.series ?? []).map<SimpleSeries>((s) => ({
			labels: s.labels ?? {},
			points: s.points.map((p) => ({ time: p.time, value: p.value })),
		})),
	),
);
</script>

<div class="table" role="table" aria-label="Service availability">
	<div class="head" role="row">
		<div role="columnheader">Service</div>
		<div role="columnheader">Status</div>
		<div role="columnheader">Note</div>
	</div>
	{#if rows().length === 0}
		<div class="row dim" role="row">
			<div class="job" role="cell">—</div>
			<div class="status" role="cell">
				<span class="dot dim" aria-hidden="true">○</span>
				<span class="word dim">no data</span>
			</div>
			<div class="note" role="cell">Awaiting a snapshot from Prometheus</div>
		</div>
	{/if}
	{#each rows() as row (row.job)}
		<div class="row" role="row" class:down={row.up === 0}>
			<div class="job" role="cell">{row.job}</div>
			<div class="status" role="cell">
				{#if row.up >= 1}
					<span class="dot" aria-hidden="true">●</span>
					<span class="word">up</span>
				{:else}
					<span class="dot crit" aria-hidden="true">●</span>
					<span class="word crit">down</span>
				{/if}
			</div>
			<div class="note" role="cell">—</div>
		</div>
	{/each}
	{#each notInstrumented as job (job)}
		<div class="row dim" role="row">
			<div class="job" role="cell">{job}</div>
			<div class="status" role="cell">
				<span class="dot dim" aria-hidden="true">○</span>
				<span class="word dim">—</span>
			</div>
			<div class="note" role="cell">Not instrumented</div>
		</div>
	{/each}
</div>

<style>
	.table {
		display: flex;
		flex-direction: column;
		font-family: var(--font-body);
	}

	.head {
		display: grid;
		grid-template-columns: 14rem 10rem 1fr;
		gap: 1rem;
		font-size: 0.68rem;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--alt-slate);
		border-bottom: 1px solid var(--obs-rule, var(--surface-border));
		padding: 0.4rem 0;
	}

	.row {
		display: grid;
		grid-template-columns: 14rem 10rem 1fr;
		gap: 1rem;
		padding: 0.4rem 0;
		border-bottom: 0.5px solid var(--obs-rule, var(--surface-border));
		color: var(--alt-charcoal);
		font-size: 0.88rem;
	}

	.row.dim {
		color: var(--obs-muted, var(--alt-ash));
	}

	.job {
		font-variant-numeric: tabular-nums;
	}

	.status {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
	}

	.dot {
		font-size: 0.9rem;
		line-height: 1;
		color: var(--obs-dot-up, var(--alt-charcoal));
	}

	.dot.crit {
		color: var(--obs-dot-down, var(--obs-critical, var(--alt-error)));
	}

	.dot.dim {
		color: var(--obs-muted, var(--alt-ash));
	}

	.word {
		font-weight: 500;
	}

	.word.crit {
		color: var(--obs-critical, var(--alt-error));
		font-weight: 600;
	}

	.word.dim {
		color: var(--obs-muted, var(--alt-ash));
	}

	.note {
		font-size: 0.78rem;
		color: var(--obs-muted, var(--alt-ash));
	}
</style>
