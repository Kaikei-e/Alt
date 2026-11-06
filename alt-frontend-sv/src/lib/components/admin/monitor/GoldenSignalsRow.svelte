<script lang="ts">
import MetricCard from "./MetricCard.svelte";
import {
	edgeEmptyText,
	type LatestValueOptions,
	type MetricMap,
	THRESHOLDS,
} from "./model";

let {
	metrics,
	evalOpts,
}: {
	metrics: MetricMap;
	evalOpts?: LatestValueOptions;
} = $props();

const latency = $derived(metrics.get("edge_latency_p95"));
const traffic = $derived(metrics.get("edge_rps"));
const errorRatio = $derived(metrics.get("edge_error_ratio"));
const hostCpu = $derived(metrics.get("host_cpu_utilization"));
</script>

<div class="golden" role="group" aria-label="Golden signals">
	<h3 class="sub-head">Golden signals</h3>
	<div class="grid">
		<MetricCard
			label="Edge p95 latency"
			metric={latency}
			threshold={THRESHOLDS.edgeLatencyP95}
			emptyText={edgeEmptyText(metrics, "edge_latency_p95", evalOpts)}
			{evalOpts}
		/>
		<MetricCard
			label="Edge traffic"
			metric={traffic}
			thresholdText="5m request rate · informational"
			emptyText={edgeEmptyText(metrics, "edge_rps", evalOpts)}
			{evalOpts}
		/>
		<MetricCard
			label="Edge 5xx ratio"
			metric={errorRatio}
			threshold={THRESHOLDS.edgeErrorRatio}
			emptyText={edgeEmptyText(metrics, "edge_error_ratio", evalOpts)}
			{evalOpts}
		/>
		<MetricCard
			label="Host CPU"
			metric={hostCpu}
			threshold={THRESHOLDS.hostCpu}
			{evalOpts}
		/>
	</div>
</div>

<style>
	.golden {
		display: grid;
		gap: 0.5rem;
		min-width: 0;
	}

	.sub-head {
		font-family: var(--font-body);
		font-size: 0.72rem;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		margin: 0;
		color: var(--alt-slate);
	}

	.grid {
		display: grid;
		grid-template-columns: repeat(4, minmax(0, 1fr));
		gap: 0.7rem;
		min-width: 0;
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
