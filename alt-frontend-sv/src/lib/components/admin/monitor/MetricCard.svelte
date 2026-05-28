<script lang="ts">
import SLISparkline from "$lib/components/knowledge-home-admin/SLISparkline.svelte";
import type { MetricResult } from "$lib/gen/alt/admin_monitor/v1/admin_monitor_pb";
import { formatValue, stateBadge } from "./format";

let {
	label,
	metric,
	warn,
	thresholdValue,
	aggregate = "last",
}: {
	label: string;
	metric: MetricResult | undefined;
	warn?: (v: number) => boolean;
	thresholdValue?: number;
	aggregate?: "last" | "sum" | "max";
} = $props();

const primaryPoints = $derived(() => {
	const pts: number[] = [];
	for (const s of metric?.series ?? []) {
		for (const p of s.points ?? []) pts.push(p.value);
	}
	return pts;
});

const leadValue = $derived(() => {
	const series = metric?.series ?? [];
	if (series.length === 0) return null;
	if (aggregate === "sum") {
		let total = 0;
		let any = false;
		for (const s of series) {
			const last = s.points.at(-1)?.value;
			if (last != null && Number.isFinite(last)) {
				total += last;
				any = true;
			}
		}
		return any ? total : null;
	}
	if (aggregate === "max") {
		let max: number | null = null;
		for (const s of series) {
			const last = s.points.at(-1)?.value;
			if (last != null && Number.isFinite(last)) {
				max = max == null ? last : Math.max(max, last);
			}
		}
		return max;
	}
	return series[0]?.points.at(-1)?.value ?? null;
});

const valueText = $derived(() => formatValue(leadValue(), metric?.unit));
const badge = $derived(() => stateBadge(leadValue(), metric?.unit, warn));
const degraded = $derived(() => metric?.degraded ?? false);
</script>

<article class="card" class:degraded={degraded()} data-state={badge().text} data-testid="metric-card">
	<header>
		<span class="label">{label}</span>
		<span class="badge" aria-label="state {badge().text}">
			<span class="glyph" aria-hidden="true">{badge().glyph}</span>
			<span class="state-text">{badge().text}</span>
		</span>
	</header>

	<div class="value">
		<span class="num">{valueText()}</span>
		{#if metric?.unit && metric.unit !== "bool" && metric.unit !== "ratio"}
			<span class="unit">{metric.unit}</span>
		{/if}
	</div>

	<div class="spark" aria-hidden="true">
		{#if primaryPoints().length >= 2}
			<SLISparkline values={primaryPoints()} threshold={thresholdValue} width={220} height={36} />
		{:else}
			<span class="dim">no series</span>
		{/if}
	</div>

	{#if degraded()}
		<div class="reason">{metric?.reason || "source degraded"}</div>
	{/if}
</article>

<style>
	.card {
		display: grid;
		grid-template-rows: auto auto auto;
		gap: 0.35rem;
		padding: 0.85rem 0.95rem;
		border: 0.5px solid var(--obs-rule, var(--surface-border));
		border-top: 2px solid var(--alt-charcoal);
		background: var(--surface);
		min-width: 0;
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
		font-size: 0.7rem;
		color: var(--alt-slate);
	}

	.card[data-state="warn"] .glyph,
	.card[data-state="warn"] .state-text {
		color: var(--obs-warn, var(--alt-warning));
	}

	.card[data-state="down"] .glyph,
	.card[data-state="down"] .state-text {
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
		font-size: 2.1rem;
		font-weight: 500;
		color: var(--alt-charcoal);
		font-variant-numeric: tabular-nums;
	}

	.unit {
		font-family: var(--font-mono);
		font-size: 0.78rem;
		color: var(--alt-ash);
	}

	.spark {
		min-height: 36px;
		display: flex;
		align-items: center;
	}

	.dim {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-ash);
	}

	.reason {
		font-size: 0.72rem;
		color: var(--obs-muted, var(--alt-ash));
		font-style: italic;
	}
</style>
