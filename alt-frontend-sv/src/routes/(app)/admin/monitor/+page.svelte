<script lang="ts">
import { untrack } from "svelte";
import AlertsPanel from "#lib/components/admin/monitor/AlertsPanel.svelte";
import AttentionSummary from "#lib/components/admin/monitor/AttentionSummary.svelte";
import CertificatesPanel from "#lib/components/admin/monitor/CertificatesPanel.svelte";
import GoldenSignalsRow from "#lib/components/admin/monitor/GoldenSignalsRow.svelte";
import HostResources from "#lib/components/admin/monitor/HostResources.svelte";
import MonitorErrorBanner from "#lib/components/admin/monitor/MonitorErrorBanner.svelte";
import MonitorHeader from "#lib/components/admin/monitor/MonitorHeader.svelte";
import {
	alertsAvailability,
	buildAlertRows,
	buildAttention,
	buildCertRows,
	buildHostResources,
	buildPipelines,
	buildServiceRows,
	certsAvailability,
	indexMetrics,
	type LatestValueOptions,
	MONITOR_KEYS,
	maxPointAgeSeconds,
	stepEnumToOpt,
} from "#lib/components/admin/monitor/model.js";
import PipelinesPanel from "#lib/components/admin/monitor/PipelinesPanel.svelte";
import ServiceREDTable from "#lib/components/admin/monitor/ServiceREDTable.svelte";
import SLOBurnPanel from "#lib/components/admin/monitor/SLOBurnPanel.svelte";
import {
	RangeWindow,
	Step,
} from "#lib/gen/alt/admin_monitor/v1/admin_monitor_pb.js";
import {
	type UseConnectAdminMetricsResult,
	useConnectAdminMetrics,
} from "#lib/hooks/useConnectAdminMetrics.svelte.js";

let window = $state(RangeWindow.RANGE_WINDOW_1H);
let step = $state(Step.STEP_30S);
let paused = $state(false);
let now = $state(Date.now());

let stream = $state<UseConnectAdminMetricsResult | null>(null);

$effect(() => {
	now = Date.now();
	const id = setInterval(() => {
		now = Date.now();
	}, 1000);
	return () => clearInterval(id);
});

$effect(() => {
	if (stream?.snapshotTime) {
		now = Date.now();
	}
});

$effect(() => {
	const _window = window;
	const _step = step;
	const next = useConnectAdminMetrics({
		keys: [...MONITOR_KEYS],
		window: _window,
		step: _step,
	});
	stream = next;
	if (!untrack(() => paused)) {
		next.start();
	}
	return () => next.stop();
});

function togglePause() {
	if (!stream) return;
	paused = !paused;
	if (paused) stream.stop();
	else stream.start();
}

const maxAgeSeconds = $derived(maxPointAgeSeconds(stepEnumToOpt(step)));
const evalOpts = $derived.by((): LatestValueOptions | undefined => {
	const snapTime = stream?.snapshotTime;
	if (!snapTime) return undefined;
	const snapMs = Date.parse(snapTime);
	if (!Number.isFinite(snapMs)) return undefined;
	return { asOfMs: snapMs, maxAgeSeconds };
});

const metricMap = $derived(indexMetrics(stream?.metrics ?? []));
const alertAvailability = $derived(alertsAvailability(metricMap));
const alertRows = $derived(buildAlertRows(metricMap));
const serviceRows = $derived(buildServiceRows(metricMap, evalOpts));
const hostResources = $derived(buildHostResources(metricMap, evalOpts));
const certAvailability = $derived(certsAvailability(metricMap, evalOpts));
const certRows = $derived(buildCertRows(metricMap, evalOpts));
const pipelines = $derived(buildPipelines(metricMap, evalOpts));
const attentionItems = $derived(
	buildAttention({
		metrics: metricMap,
		streamState: stream?.state ?? "idle",
		snapshotTime: stream?.snapshotTime || null,
		now,
		lastError: stream?.lastError ?? null,
		maxAgeSeconds,
	}),
);
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
			{now}
			lastError={stream.lastError}
			onTogglePause={togglePause}
			{paused}
		/>

		<MonitorErrorBanner
			metrics={stream.metrics}
			streamState={stream.state}
			lastError={stream.lastError}
		/>

		<AttentionSummary
			items={attentionItems}
			{serviceRows}
			{alertRows}
			alertsAvailable={alertAvailability.available}
		/>

		<AlertsPanel
			rows={alertRows}
			{now}
			availability={alertAvailability}
		/>

		<section id="edge" class="edge-section" aria-label="Edge">
			<header class="section-bar">
				<h2 class="section-head">Edge — Golden Signals &amp; SLO Burn</h2>
			</header>
			<GoldenSignalsRow metrics={metricMap} {evalOpts} />
			<SLOBurnPanel metrics={metricMap} {evalOpts} />
		</section>

		<ServiceREDTable metrics={metricMap} rows={serviceRows} />

		<HostResources metrics={metricMap} resources={hostResources} />

		<CertificatesPanel rows={certRows} availability={certAvailability} />

		<PipelinesPanel {pipelines} />
	{/if}
</main>

<style>
	.monitor {
		--obs-good: var(--alt-success);
		--obs-warn: var(--alt-warning);
		--obs-critical: var(--alt-error);
		--obs-muted: #6b655c;
		--obs-rule: #cfc8bb;
		--obs-rule-strong: #b4ab9b;
		--obs-spark-stroke: #3b3630;
		--obs-spark-threshold: var(--obs-warn);

		display: grid;
		gap: 1.4rem;
		padding: 1.4rem 1.6rem 2.5rem;
		max-width: 1480px;
		width: 100%;
		box-sizing: border-box;
		margin: 0 auto;
		font-family: var(--font-body);
		color: var(--alt-charcoal);
		background: var(--surface-bg);
		min-height: 100dvh;
	}

	.edge-section {
		display: grid;
		gap: 0.85rem;
		min-width: 0;
	}

	.section-bar {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		border-top: 1px solid var(--obs-rule-strong, var(--surface-border));
		padding-top: 0.55rem;
	}

	.section-head {
		font-family: var(--font-display, var(--font-serif));
		font-size: 0.92rem;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		margin: 0;
		color: var(--alt-charcoal);
	}

	@media (max-width: 540px) {
		.monitor {
			padding: 1rem 0.9rem 2rem;
		}
	}
</style>
