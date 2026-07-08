<script lang="ts">
import type { MetricResult } from "$lib/gen/alt/admin_monitor/v1/admin_monitor_pb";
import MetricCard from "./MetricCard.svelte";

let {
	metrics,
}: {
	metrics: MetricResult[];
} = $props();

function find(key: string): MetricResult | undefined {
	return metrics.find((m) => m.key === key);
}

const latency = $derived(find("http_latency_p95"));
const traffic = $derived(find("http_rps"));
const errorRatio = $derived(find("http_error_ratio"));
const cpu = $derived(find("cpu_saturation"));

// Thresholds — conservative first pass aligned with the existing ObservabilityPanel.
const latencyWarn = (v: number) => v > 1.0; // p95 > 1s
const errorWarn = (v: number) => v > 0.01; // > 1%
const cpuWarn = (v: number) => v > 1.5; // > 1.5 cores summed across containers
</script>

<section class="golden" aria-label="Golden signals">
	<h2 class="section-head">Golden signals</h2>
	<div class="grid">
		<MetricCard
			label="HTTP latency p95"
			metric={latency}
			warn={latencyWarn}
			thresholdValue={1.0}
			aggregate="max"
		/>
		<MetricCard
			label="HTTP traffic"
			metric={traffic}
			aggregate="sum"
		/>
		<MetricCard
			label="HTTP 5xx ratio"
			metric={errorRatio}
			warn={errorWarn}
			thresholdValue={0.01}
			aggregate="max"
		/>
		<MetricCard
			label="CPU saturation"
			metric={cpu}
			warn={cpuWarn}
			thresholdValue={1.5}
			aggregate="sum"
		/>
	</div>
</section>

<style>
	.golden {
		display: grid;
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

	.grid {
		display: grid;
		grid-template-columns: repeat(4, minmax(0, 1fr));
		gap: 0.7rem;
	}

	@media (max-width: 1100px) {
		.grid {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
	}

	@media (max-width: 540px) {
		.grid {
			grid-template-columns: 1fr;
		}
	}
</style>
