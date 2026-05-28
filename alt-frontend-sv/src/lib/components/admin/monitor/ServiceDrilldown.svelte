<script lang="ts">
import type { MetricResult } from "$lib/gen/alt/admin_monitor/v1/admin_monitor_pb";
import SLISparkline from "$lib/components/knowledge-home-admin/SLISparkline.svelte";
import { formatValue } from "./format";

let {
	job,
	metrics,
}: {
	job: string;
	metrics: MetricResult[];
} = $props();

function find(key: string): MetricResult | undefined {
	return metrics.find((m) => m.key === key);
}

function seriesForJob(key: string): number[] {
	const m = find(key);
	if (!m) return [];
	for (const s of m.series ?? []) {
		if (s.labels?.job === job) {
			return s.points.map((p) => p.value);
		}
	}
	return [];
}

function seriesForName(key: string): { container: string; values: number[] }[] {
	const m = find(key);
	if (!m) return [];
	const out: { container: string; values: number[] }[] = [];
	for (const s of m.series ?? []) {
		const name = s.labels?.name ?? "";
		// Match cAdvisor container name to the prometheus job heuristically.
		if (name && (name === job || name.startsWith(job))) {
			out.push({ container: name, values: s.points.map((p) => p.value) });
		}
	}
	return out;
}

const p50 = $derived(() => seriesForJob("http_latency_p50"));
const p95 = $derived(() => seriesForJob("http_latency_p95"));
const p99 = $derived(() => seriesForJob("http_latency_p99"));
const rps = $derived(() => seriesForJob("http_rps"));
const err = $derived(() => seriesForJob("http_error_ratio"));
const cpu = $derived(() => seriesForName("cpu_saturation"));
const mem = $derived(() => seriesForName("memory_rss"));
</script>

<div class="drill">
	<div class="grid">
		<div class="panel">
			<h3>Latency band</h3>
			<dl>
				<dt>p99 latest</dt>
				<dd>{formatValue(p99().at(-1) ?? null, "seconds")}</dd>
				<dt>p95 latest</dt>
				<dd>{formatValue(p95().at(-1) ?? null, "seconds")}</dd>
				<dt>p50 latest</dt>
				<dd>{formatValue(p50().at(-1) ?? null, "seconds")}</dd>
			</dl>
			<div class="overlay" aria-hidden="true">
				{#if p99().length >= 2}
					<SLISparkline values={p99()} width={260} height={32} threshold={1.0} />
				{/if}
				{#if p95().length >= 2}
					<SLISparkline values={p95()} width={260} height={32} />
				{/if}
				{#if p50().length >= 2}
					<SLISparkline values={p50()} width={260} height={32} />
				{/if}
			</div>
		</div>

		<div class="panel">
			<h3>Traffic + errors</h3>
			<dl>
				<dt>rps latest</dt>
				<dd>{formatValue(rps().at(-1) ?? null, "req/s")}</dd>
				<dt>err latest</dt>
				<dd>{formatValue(err().at(-1) ?? null, "ratio")}</dd>
			</dl>
			<div class="overlay" aria-hidden="true">
				{#if rps().length >= 2}
					<SLISparkline values={rps()} width={260} height={32} />
				{/if}
				{#if err().length >= 2}
					<SLISparkline values={err()} width={260} height={32} threshold={0.01} />
				{/if}
			</div>
		</div>

		<div class="panel">
			<h3>Container resource</h3>
			{#if cpu().length === 0 && mem().length === 0}
				<p class="dim">No cAdvisor series match this service.</p>
			{:else}
				<dl>
					<dt>cpu (cores)</dt>
					<dd>
						{cpu()
							.map(
								(c) =>
									`${c.container} · ${formatValue(c.values.at(-1) ?? null, "cores")}`,
							)
							.join("  ·  ") || "—"}
					</dd>
					<dt>mem</dt>
					<dd>
						{mem()
							.map(
								(c) =>
									`${c.container} · ${formatValue(c.values.at(-1) ?? null, "bytes")}`,
							)
							.join("  ·  ") || "—"}
					</dd>
				</dl>
			{/if}
		</div>
	</div>
</div>

<style>
	.drill {
		font-size: 0.82rem;
	}

	.grid {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 0.7rem;
	}

	@media (max-width: 1100px) {
		.grid {
			grid-template-columns: 1fr;
		}
	}

	.panel {
		display: grid;
		gap: 0.35rem;
		padding: 0.55rem 0.7rem;
		border: 0.5px solid var(--obs-rule, var(--surface-border));
		background: var(--surface);
	}

	.panel h3 {
		font-family: var(--font-body);
		font-size: 0.66rem;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--alt-slate);
		margin: 0;
	}

	dl {
		display: grid;
		grid-template-columns: auto 1fr;
		gap: 0.15rem 0.6rem;
		margin: 0;
		font-family: var(--font-mono);
		font-size: 0.78rem;
	}

	dt {
		color: var(--alt-ash);
	}

	dd {
		margin: 0;
		color: var(--alt-charcoal);
		font-variant-numeric: tabular-nums;
	}

	.overlay {
		display: flex;
		flex-direction: column;
		gap: 0.1rem;
	}

	.dim {
		color: var(--alt-ash);
		font-style: italic;
	}
</style>
