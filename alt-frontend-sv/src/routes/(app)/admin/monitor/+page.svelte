<script lang="ts">
import {
	RangeWindow,
	Step,
} from "$lib/gen/alt/admin_monitor/v1/admin_monitor_pb";
import {
	useConnectAdminMetrics,
	type UseConnectAdminMetricsResult,
} from "$lib/hooks/useConnectAdminMetrics.svelte";

import MonitorHeader from "$lib/components/admin/monitor/MonitorHeader.svelte";
import MonitorErrorBanner from "$lib/components/admin/monitor/MonitorErrorBanner.svelte";
import GoldenSignalsRow from "$lib/components/admin/monitor/GoldenSignalsRow.svelte";
import SLOBurnPanel from "$lib/components/admin/monitor/SLOBurnPanel.svelte";
import ServiceREDTable from "$lib/components/admin/monitor/ServiceREDTable.svelte";
import SaturationGrid from "$lib/components/admin/monitor/SaturationGrid.svelte";
import QueueHealthCard from "$lib/components/admin/monitor/QueueHealthCard.svelte";

const MONITOR_KEYS = [
	"availability_services",
	"http_latency_p50",
	"http_latency_p95",
	"http_latency_p99",
	"http_rps",
	"http_error_ratio",
	"cpu_saturation",
	"memory_rss",
	"mqhub_publish_rate",
	"mqhub_redis",
	"prometheus_scrape_lag",
	"availability_burn_1h",
	"availability_burn_6h",
];

let window = $state(RangeWindow.RANGE_WINDOW_1H);
let step = $state(Step.STEP_30S);
let paused = $state(false);

let stream = $state<UseConnectAdminMetricsResult | null>(null);

// The hook reads window/step inside its runOnce closure, so a picker change
// requires a tear-down + restart. $effect re-runs on every window/step change
// and returns its cleanup, which Svelte invokes before the next run AND on
// component destroy — handles route navigation too.
$effect(() => {
	const _window = window;
	const _step = step;
	const next = useConnectAdminMetrics({
		keys: MONITOR_KEYS,
		window: _window,
		step: _step,
	});
	stream = next;
	if (!paused) next.start();
	return () => next.stop();
});

function togglePause() {
	if (!stream) return;
	paused = !paused;
	if (paused) stream.stop();
	else stream.start();
}
</script>

<svelte:head>
	<title>System Monitor — Admin</title>
</svelte:head>

<main class="monitor" data-style="alt-paper">
	{#if stream}
		<MonitorHeader
			bind:window
			bind:step
			streamState={stream.state}
			snapshotTime={stream.snapshotTime}
			onTogglePause={togglePause}
			{paused}
		/>

		<MonitorErrorBanner metrics={stream.metrics} streamState={stream.state} />

		<GoldenSignalsRow metrics={stream.metrics} />

		<SLOBurnPanel metrics={stream.metrics} />

		<ServiceREDTable metrics={stream.metrics} />

		<div class="bottom">
			<SaturationGrid metrics={stream.metrics} />
			<QueueHealthCard metrics={stream.metrics} />
		</div>

		<footer class="foot">
			<span>Showing {stream.metrics.length} allowlisted metric keys.</span>
			<span class="dim">
				Stream rotates every 15 min · 5 s push interval · reconnect backoff up
				to 30 s.
			</span>
		</footer>
	{/if}
</main>

<style>
	.monitor {
		display: grid;
		gap: 1.4rem;
		padding: 1.4rem 1.6rem 2.5rem;
		max-width: 1480px;
		margin: 0 auto;
		font-family: var(--font-body);
		color: var(--alt-charcoal);
		background: var(--surface);
		min-height: 100dvh;
	}

	.bottom {
		display: grid;
		grid-template-columns: 1.4fr 1fr;
		gap: 1rem;
	}

	@media (max-width: 1100px) {
		.bottom {
			grid-template-columns: 1fr;
		}
	}

	.foot {
		display: flex;
		justify-content: space-between;
		gap: 1rem;
		padding-top: 0.6rem;
		border-top: 0.5px solid var(--obs-rule, var(--surface-border));
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-slate);
	}

	.dim {
		color: var(--alt-ash);
	}

	@media (max-width: 540px) {
		.monitor {
			padding: 1rem 0.9rem 2rem;
		}
	}
</style>
