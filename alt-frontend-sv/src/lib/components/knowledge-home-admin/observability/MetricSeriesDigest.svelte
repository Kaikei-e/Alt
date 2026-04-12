<script lang="ts">
import { topSeries, type SimpleSeries } from "./util";

let {
	series = [],
	preferLabel = "job",
	unit = "",
	limit = 3,
}: {
	series?: SimpleSeries[];
	preferLabel?: string;
	unit?: string;
	limit?: number;
} = $props();

const result = $derived(() => topSeries(series, preferLabel, limit));

function fmt(n: number): string {
	if (!Number.isFinite(n)) return "—";
	if (unit === "bytes") return formatBytes(n);
	if (unit === "ratio") return `${(n * 100).toFixed(2)}%`;
	if (unit === "seconds") return `${(n * 1000).toFixed(0)}ms`;
	if (unit === "bool") return n >= 1 ? "up" : "down";
	if (Math.abs(n) >= 1000) return n.toFixed(0);
	if (Math.abs(n) >= 1) return n.toFixed(2);
	return n.toFixed(3);
}

function formatBytes(n: number): string {
	const units = ["B", "KiB", "MiB", "GiB"];
	let v = n;
	let i = 0;
	while (v >= 1024 && i < units.length - 1) {
		v /= 1024;
		i += 1;
	}
	return `${v.toFixed(v >= 100 ? 0 : v >= 10 ? 1 : 2)}${units[i]}`;
}
</script>

{#if series.length === 0}
	<span class="empty" aria-label="No series">—</span>
{:else}
	<span class="digest" role="list">
		{#each result().head as row, i (row.labelValue + i)}
			{#if i > 0}
				<span class="sep" aria-hidden="true">│</span>
			{/if}
			<span class="row" role="listitem">
				<span class="label">{row.labelValue}</span>
				<span class="value">{fmt(row.lead)}</span>
			</span>
		{/each}
		{#if result().overflow > 0}
			<span class="overflow" aria-label={`${result().overflow} more series`}>
				+{result().overflow} more
			</span>
		{/if}
	</span>
{/if}

<style>
	.digest {
		display: inline-flex;
		flex-wrap: wrap;
		gap: 0.4rem;
		align-items: baseline;
		font-size: 0.74rem;
		color: var(--alt-slate);
	}

	.row {
		display: inline-flex;
		gap: 0.35rem;
		align-items: baseline;
	}

	.label {
		color: var(--alt-charcoal);
		font-variant-numeric: tabular-nums;
	}

	.value {
		color: var(--alt-slate);
		font-variant-numeric: tabular-nums;
	}

	.sep {
		color: var(--obs-rule-strong, #b4ab9b);
	}

	.overflow {
		color: var(--obs-muted, var(--alt-ash));
		font-style: italic;
	}

	.empty {
		color: var(--alt-ash);
	}
</style>
