<script lang="ts">
import type { MetricResult } from "#lib/gen/alt/admin_monitor/v1/admin_monitor_pb.js";
import { formatValue } from "./format";
import MonitorSparkline from "./MonitorSparkline.svelte";
import {
	badge,
	classify,
	type LatestValueOptions,
	latestValue,
	type Severity,
	seriesPoints,
	type Threshold,
} from "./model";

let {
	label,
	metric,
	threshold,
	thresholdText,
	emptyText,
	evalOpts,
}: {
	label: string;
	metric: MetricResult | undefined;
	threshold?: Threshold;
	thresholdText?: string;
	emptyText?: string;
	evalOpts?: LatestValueOptions;
} = $props();

const points = $derived(seriesPoints(metric));
const leadValue = $derived(latestValue(metric, undefined, evalOpts));

const resolvedSeverity = $derived.by((): Severity => {
	if (threshold) return classify(leadValue, threshold);
	if (leadValue === null || !Number.isFinite(leadValue)) return "nodata";
	return "ok";
});

const stateBadge = $derived(badge(resolvedSeverity));

const valueText = $derived.by(() => {
	if (leadValue === null || !Number.isFinite(leadValue)) {
		return emptyText ?? "no data";
	}
	return formatValue(leadValue, metric?.unit);
});

const sparkThreshold = $derived(threshold?.warn);
const displayThresholdText = $derived(threshold?.display ?? thresholdText);
const degraded = $derived(metric?.degraded ?? false);
</script>

<article
	class="card"
	class:degraded={degraded}
	data-state={stateBadge.severity}
	data-testid="metric-card"
>
	<header>
		<span class="label">{label}</span>
		<span
			class="badge"
			data-severity={stateBadge.severity}
			aria-label="state {stateBadge.label}"
		>
			<span class="glyph" aria-hidden="true">{stateBadge.glyph}</span>
			<span class="state-text">{stateBadge.label}</span>
		</span>
	</header>

	<div class="value">
		<span class="num" class:muted={leadValue === null}>{valueText}</span>
		{#if leadValue !== null && metric?.unit && metric.unit !== "bool" && metric.unit !== "ratio" && metric.unit !== "seconds"}
			<span class="unit">{metric.unit}</span>
		{/if}
	</div>

	{#if displayThresholdText}
		<div class="threshold-text">{displayThresholdText}</div>
	{/if}

	<div class="spark">
		<MonitorSparkline
			{points}
			{label}
			unit={metric?.unit}
			threshold={sparkThreshold}
			width={220}
			height={36}
		/>
	</div>

	{#if degraded}
		<div class="reason">{metric?.reason || "source degraded"}</div>
	{/if}
</article>

<style>
	.card {
		display: grid;
		grid-template-rows: auto auto auto 1fr;
		gap: 0.35rem;
		padding: 0.85rem 0.95rem;
		border: 0.5px solid var(--obs-rule, var(--surface-border));
		border-top: 2px solid var(--alt-charcoal);
		background: var(--surface-bg);
		min-width: 0;
	}

	.card[data-state="crit"] {
		border-top-color: var(--obs-critical, var(--alt-error));
	}

	.card[data-state="warn"] {
		border-top-color: var(--obs-warn, var(--alt-warning));
	}

	.card.degraded {
		color: var(--obs-muted, var(--alt-ash));
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
		gap: 0.25rem;
		align-items: baseline;
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

	.value {
		display: flex;
		align-items: baseline;
		gap: 0.4rem;
		font-family: var(--font-display, var(--font-serif));
		line-height: 1.05;
	}

	.num {
		font-size: 1.95rem;
		font-weight: 500;
		color: var(--alt-charcoal);
		font-variant-numeric: tabular-nums;
	}

	.num.muted {
		font-family: var(--font-mono);
		font-size: 1.05rem;
		color: var(--alt-ash);
	}

	.unit {
		font-family: var(--font-mono);
		font-size: 0.78rem;
		color: var(--alt-ash);
	}

	.threshold-text {
		font-family: var(--font-mono);
		font-size: 0.68rem;
		color: var(--alt-slate);
	}

	.spark {
		min-height: 36px;
		display: flex;
		align-items: center;
		min-width: 0;
	}

	.reason {
		font-size: 0.72rem;
		color: var(--obs-muted, var(--alt-ash));
		font-style: italic;
	}
</style>
