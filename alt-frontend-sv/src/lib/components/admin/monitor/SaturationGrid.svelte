<script lang="ts">
import type { MetricResult } from "$lib/gen/alt/admin_monitor/v1/admin_monitor_pb";
import SLISparkline from "$lib/components/knowledge-home-admin/SLISparkline.svelte";
import { formatValue } from "./format";

let {
	metrics,
}: {
	metrics: MetricResult[];
} = $props();

interface ContainerRow {
	name: string;
	cpu: number | null;
	cpuSeries: number[];
	mem: number | null;
	memSeries: number[];
}

function find(key: string): MetricResult | undefined {
	return metrics.find((m) => m.key === key);
}

const rows = $derived(() => {
	const byName: Record<string, ContainerRow> = {};
	const ensure = (name: string): ContainerRow => {
		if (!byName[name]) {
			byName[name] = {
				name,
				cpu: null,
				cpuSeries: [],
				mem: null,
				memSeries: [],
			};
		}
		return byName[name];
	};

	const cpu = find("cpu_saturation");
	for (const s of cpu?.series ?? []) {
		const name = s.labels?.name;
		if (!name) continue;
		const series = s.points.map((p) => p.value);
		const row = ensure(name);
		row.cpu = series.at(-1) ?? null;
		row.cpuSeries = series;
	}

	const mem = find("memory_rss");
	for (const s of mem?.series ?? []) {
		const name = s.labels?.name;
		if (!name) continue;
		const last = s.points.at(-1)?.value ?? null;
		const row = ensure(name);
		row.mem = last;
		row.memSeries = s.points.map((p) => p.value);
	}

	return Object.values(byName).sort((a, b) => a.name.localeCompare(b.name));
});

const cpuWarn = (v: number) => v > 1.5;
const memWarn = (v: number) => v > 1.5 * 1024 * 1024 * 1024; // > 1.5 GiB

function cpuClass(r: ContainerRow): string {
	return r.cpu != null && cpuWarn(r.cpu) ? "warn" : "ok";
}

function memClass(r: ContainerRow): string {
	return r.mem != null && memWarn(r.mem) ? "warn" : "ok";
}
</script>

<section class="sat" aria-label="Container saturation">
	<h2 class="section-head">Container saturation · USE</h2>
	{#if rows().length === 0}
		<p class="dim">No cAdvisor series yet.</p>
	{:else}
		<div class="grid">
			{#each rows() as r (r.name)}
				<article class="cell">
					<header>
						<span class="name">{r.name}</span>
					</header>
					<div class="metric" data-state={cpuClass(r)}>
						<div class="metric-head">
							<span class="label">CPU</span>
							<span class="value">{formatValue(r.cpu, "cores")} cores</span>
						</div>
						{#if r.cpuSeries.length >= 2}
							<SLISparkline values={r.cpuSeries} width={200} height={22} threshold={1.5} />
						{/if}
					</div>
					<div class="metric" data-state={memClass(r)}>
						<div class="metric-head">
							<span class="label">Mem</span>
							<span class="value">{formatValue(r.mem, "bytes")}</span>
						</div>
						{#if r.memSeries.length >= 2}
							<SLISparkline values={r.memSeries} width={200} height={22} />
						{/if}
					</div>
				</article>
			{/each}
		</div>
	{/if}
</section>

<style>
	.sat {
		display: grid;
		gap: 0.55rem;
	}

	.section-head {
		font-family: var(--font-display, var(--font-serif));
		font-size: 0.92rem;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		margin: 0;
		color: var(--alt-charcoal);
	}

	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
		gap: 0.65rem;
	}

	.cell {
		display: grid;
		gap: 0.4rem;
		padding: 0.6rem 0.75rem;
		border: 0.5px solid var(--obs-rule, var(--surface-border));
		background: var(--surface);
	}

	.name {
		font-family: var(--font-mono);
		font-size: 0.78rem;
		color: var(--alt-charcoal);
	}

	.metric {
		display: grid;
		gap: 0.15rem;
	}

	.metric-head {
		display: flex;
		justify-content: space-between;
		font-size: 0.74rem;
	}

	.label {
		color: var(--alt-ash);
		font-family: var(--font-body);
		letter-spacing: 0.1em;
		text-transform: uppercase;
		font-size: 0.66rem;
	}

	.value {
		font-family: var(--font-mono);
		font-variant-numeric: tabular-nums;
		color: var(--alt-charcoal);
	}

	.metric[data-state="warn"] .value {
		color: var(--obs-warn, var(--alt-warning));
		font-weight: 600;
	}

	.dim {
		color: var(--alt-ash);
		font-style: italic;
	}
</style>
