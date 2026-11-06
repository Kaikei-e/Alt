<script lang="ts">
import { formatValue } from "./format";
import MonitorSparkline from "./MonitorSparkline.svelte";
import {
	badge,
	type HostResources as HostResourcesModel,
	type MetricMap,
	seriesPoints,
	THRESHOLDS,
} from "./model";

let {
	metrics,
	resources: res,
}: {
	metrics: MetricMap;
	resources: HostResourcesModel;
} = $props();

const cpuBadge = $derived(badge(res.cpuSeverity));
const memBadge = $derived(badge(res.memorySeverity));
const cpuPoints = $derived(seriesPoints(metrics.get("host_cpu_utilization")));
const memPoints = $derived(
	seriesPoints(metrics.get("host_memory_utilization")),
);

function clampPct(ratio: number | null): number {
	if (ratio === null || !Number.isFinite(ratio)) return 0;
	return Math.max(0, Math.min(100, ratio * 100));
}
</script>

<section id="resources" class="resources" aria-label="Resources">
	<header class="section-bar">
		<h2 class="section-head">Resources — Host Utilization</h2>
	</header>

	<div class="grid">
		<article class="cell" data-severity={cpuBadge.severity}>
			<header>
				<span class="label">Host CPU utilization</span>
				<span class="badge" data-severity={cpuBadge.severity}>
					<span class="glyph" aria-hidden="true">{cpuBadge.glyph}</span>
					<span>{cpuBadge.label}</span>
				</span>
			</header>
			<div class="value">
				<span class="num" class:muted={res.cpu === null}>
					{res.cpu !== null ? formatValue(res.cpu, "ratio") : "NO DATA"}
				</span>
			</div>
			<div class="bar-track" aria-hidden="true">
				<div
					class="bar-fill"
					data-severity={cpuBadge.severity}
					style="width: {clampPct(res.cpu).toFixed(1)}%"
				></div>
			</div>
			<div class="threshold">{THRESHOLDS.hostCpu.display}</div>
			<MonitorSparkline
				points={cpuPoints}
				label="Host CPU utilization"
				unit="ratio"
				threshold={THRESHOLDS.hostCpu.warn}
				width={220}
				height={32}
			/>
		</article>

		<article class="cell" data-severity={memBadge.severity}>
			<header>
				<span class="label">Host memory utilization</span>
				<span class="badge" data-severity={memBadge.severity}>
					<span class="glyph" aria-hidden="true">{memBadge.glyph}</span>
					<span>{memBadge.label}</span>
				</span>
			</header>
			<div class="value">
				<span class="num" class:muted={res.memory === null}>
					{res.memory !== null ? formatValue(res.memory, "ratio") : "NO DATA"}
				</span>
			</div>
			<div class="bar-track" aria-hidden="true">
				<div
					class="bar-fill"
					data-severity={memBadge.severity}
					style="width: {clampPct(res.memory).toFixed(1)}%"
				></div>
			</div>
			<div class="threshold">{THRESHOLDS.hostMemory.display}</div>
			<MonitorSparkline
				points={memPoints}
				label="Host memory utilization"
				unit="ratio"
				threshold={THRESHOLDS.hostMemory.warn}
				width={220}
				height={32}
			/>
		</article>

		<article class="cell">
			<header>
				<span class="label">Host filesystems</span>
				<span class="threshold">{THRESHOLDS.hostFilesystem.display}</span>
			</header>
			{#if !res.filesystemsAvailable}
				<p class="dim">
					NO DATA — filesystem utilization unavailable{res.filesystemsReason ? ` (${res.filesystemsReason})` : ""}
				</p>
			{:else if res.filesystems.length === 0}
				<p class="dim">No filesystem utilization series reported.</p>
			{:else}
				<ul class="fs-list">
					{#each res.filesystems as fs (fs.device)}
						{@const fsBadge = badge(fs.severity)}
						<li class="fs-row">
							<div class="fs-head">
								<span class="device">{fs.device}</span>
								<span class="fs-meta">
									<span class="badge" data-severity={fsBadge.severity}>
										<span class="glyph" aria-hidden="true">{fsBadge.glyph}</span>
										<span>{fsBadge.label}</span>
									</span>
									<span class="fs-val">{formatValue(fs.ratio, "ratio")}</span>
								</span>
							</div>
							<div class="bar-track" aria-hidden="true">
								<div
									class="bar-fill"
									data-severity={fsBadge.severity}
									style="width: {clampPct(fs.ratio).toFixed(1)}%"
								></div>
							</div>
						</li>
					{/each}
				</ul>
			{/if}
		</article>
	</div>

	<p class="container-note" data-testid="per-container-note">
		{res.perContainerReason}
	</p>
</section>

<style>
	.resources {
		display: grid;
		gap: 0.55rem;
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

	.grid {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 0.7rem;
		min-width: 0;
	}

	@media (max-width: 1100px) {
		.grid {
			grid-template-columns: 1fr;
		}
	}

	.cell {
		display: grid;
		gap: 0.4rem;
		padding: 0.85rem 0.95rem;
		border: 0.5px solid var(--obs-rule, var(--surface-border));
		border-top: 2px solid var(--alt-charcoal);
		background: var(--surface-bg);
		min-width: 0;
	}

	header {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		gap: 0.5rem;
	}

	.label {
		font-family: var(--font-body);
		font-size: 0.66rem;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--alt-slate);
	}

	.badge {
		display: inline-flex;
		align-items: baseline;
		gap: 0.25rem;
		font-family: var(--font-mono);
		font-size: 0.7rem;
		font-weight: 600;
		color: var(--alt-slate);
	}

	.badge[data-severity="ok"] {
		color: var(--obs-good, var(--alt-success));
	}

	.badge[data-severity="warn"] {
		color: var(--obs-warn, var(--alt-warning));
	}

	.badge[data-severity="crit"] {
		color: var(--obs-critical, var(--alt-error));
	}

	.num {
		font-family: var(--font-display, var(--font-serif));
		font-size: 1.85rem;
		font-weight: 500;
		color: var(--alt-charcoal);
		font-variant-numeric: tabular-nums;
	}

	.num.muted {
		font-family: var(--font-mono);
		font-size: 0.95rem;
		color: var(--alt-ash);
	}

	.bar-track {
		width: 100%;
		height: 6px;
		background: color-mix(in oklch, var(--surface-bg) 85%, var(--alt-charcoal) 15%);
		overflow: hidden;
	}

	.bar-fill {
		height: 100%;
		background: var(--alt-charcoal);
	}

	.bar-fill[data-severity="ok"] {
		background: var(--obs-good, var(--alt-success));
	}

	.bar-fill[data-severity="warn"] {
		background: var(--obs-warn, var(--alt-warning));
	}

	.bar-fill[data-severity="crit"] {
		background: var(--obs-critical, var(--alt-error));
	}

	.threshold {
		font-family: var(--font-mono);
		font-size: 0.68rem;
		color: var(--alt-slate);
	}

	.fs-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: grid;
		gap: 0.45rem;
	}

	.fs-row {
		display: grid;
		gap: 0.2rem;
	}

	.fs-head {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		gap: 0.5rem;
		font-family: var(--font-mono);
		font-size: 0.76rem;
	}

	.device {
		color: var(--alt-charcoal);
		overflow-wrap: anywhere;
	}

	.fs-meta {
		display: inline-flex;
		align-items: baseline;
		gap: 0.5rem;
		font-variant-numeric: tabular-nums;
	}

	.fs-val {
		color: var(--alt-charcoal);
	}

	.container-note {
		margin: 0;
		font-family: var(--font-mono);
		font-size: 0.72rem;
		color: var(--obs-muted, var(--alt-ash));
	}

	.dim {
		margin: 0;
		font-family: var(--font-mono);
		font-size: 0.74rem;
		color: var(--alt-ash);
	}
</style>
