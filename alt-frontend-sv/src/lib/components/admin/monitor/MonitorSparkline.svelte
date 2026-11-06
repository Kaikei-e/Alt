<script lang="ts">
import { formatLocalTime, formatValue } from "./format";

let {
	points = [],
	label = "series",
	unit = "",
	threshold,
	width = 220,
	height = 36,
	showAxisLabels = false,
	stroke = "var(--obs-spark-stroke, var(--alt-primary))",
	thresholdStroke = "var(--obs-spark-threshold, var(--alt-warning))",
}: {
	points: { t: number; v: number }[];
	label?: string;
	unit?: string;
	threshold?: number;
	width?: number;
	height?: number;
	showAxisLabels?: boolean;
	stroke?: string;
	thresholdStroke?: string;
} = $props();

const padding = 2;

const stats = $derived.by(() => {
	const first = points[0];
	const last = points[points.length - 1];
	if (!first || !last) return null;
	let min = Number.POSITIVE_INFINITY;
	let max = Number.NEGATIVE_INFINITY;
	for (const p of points) {
		if (p.v < min) min = p.v;
		if (p.v > max) max = p.v;
	}
	return { min, max, latest: last.v, firstT: first.t, lastT: last.t };
});

const domain = $derived.by(() => {
	if (points.length < 2 || !stats) return null;
	const min =
		threshold !== undefined ? Math.min(stats.min, threshold) : stats.min;
	const max =
		threshold !== undefined ? Math.max(stats.max, threshold) : stats.max;
	return { min, max, range: max - min || 1 };
});

function pointX(
	t: number,
	i: number,
	total: number,
	firstT: number,
	lastT: number,
): number {
	const spanX = width - padding * 2;
	const dt = lastT - firstT;
	if (dt > 0 && Number.isFinite(t)) {
		return padding + ((t - firstT) / dt) * spanX;
	}
	return padding + (i / Math.max(1, total - 1)) * spanX;
}

const polylinePoints = $derived.by(() => {
	const d = domain;
	const s = stats;
	if (!d || !s || points.length < 2) return "";
	return points
		.map((p, i) => {
			const x = pointX(p.t, i, points.length, s.firstT, s.lastT);
			const y =
				height - padding - ((p.v - d.min) / d.range) * (height - padding * 2);
			return `${x.toFixed(1)},${y.toFixed(1)}`;
		})
		.join(" ");
});

const lastCoord = $derived.by(() => {
	const d = domain;
	const s = stats;
	const last = points[points.length - 1];
	if (!d || !s || points.length < 2 || !last) return null;
	const lastIdx = points.length - 1;
	const x = pointX(last.t, lastIdx, points.length, s.firstT, s.lastT);
	const y =
		height - padding - ((last.v - d.min) / d.range) * (height - padding * 2);
	return { x, y };
});

const thresholdY = $derived.by(() => {
	const d = domain;
	if (!d || threshold === undefined) return null;
	return (
		height - padding - ((threshold - d.min) / d.range) * (height - padding * 2)
	);
});

const ariaSummary = $derived.by(() => {
	if (!stats) return `${label}: no series`;
	const latestStr = formatValue(stats.latest, unit);
	const minStr = formatValue(stats.min, unit);
	const maxStr = formatValue(stats.max, unit);
	return `${label}: latest ${latestStr}, min ${minStr}, max ${maxStr}`;
});
</script>

{#if points.length >= 2 && stats}
	<div class="sparkline-wrap">
		<svg
			{width}
			{height}
			viewBox="0 0 {width} {height}"
			role="img"
			aria-label={ariaSummary}
			class="spark-svg"
		>
			{#if thresholdY !== null}
				<line
					x1="0"
					x2={width}
					y1={thresholdY}
					y2={thresholdY}
					stroke={thresholdStroke}
					stroke-width="1"
					stroke-dasharray="3 3"
					opacity="0.65"
					data-testid="sparkline-threshold"
				/>
			{/if}
			<polyline
				points={polylinePoints}
				fill="none"
				{stroke}
				stroke-width="1.25"
				stroke-linecap="round"
				stroke-linejoin="round"
			/>
			{#if lastCoord}
				<circle
					cx={lastCoord.x}
					cy={lastCoord.y}
					r="2"
					fill={stroke}
				/>
			{/if}
		</svg>
		{#if showAxisLabels}
			<div class="axis-labels">
				<span class="axis-time">
					{formatLocalTime(stats.firstT)} – {formatLocalTime(stats.lastT)}
				</span>
				<span class="axis-minmax">
					min {formatValue(stats.min, unit)} · max {formatValue(stats.max, unit)}
				</span>
			</div>
		{/if}
	</div>
{:else if points.length === 1 && stats}
	<div class="sparkline-wrap">
		<span class="dim" role="img" aria-label={ariaSummary}>
			single sample ({formatValue(stats.latest, unit)})
		</span>
		{#if showAxisLabels}
			<div class="axis-labels">
				<span class="axis-time">{formatLocalTime(stats.firstT)}</span>
				<span class="axis-minmax">
					min {formatValue(stats.min, unit)} · max {formatValue(stats.max, unit)}
				</span>
			</div>
		{/if}
	</div>
{:else}
	<span class="dim" role="img" aria-label={ariaSummary}>no series</span>
{/if}

<style>
	.sparkline-wrap {
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		width: 100%;
		min-width: 0;
	}

	.spark-svg {
		display: block;
		max-width: 100%;
		height: auto;
		overflow: visible;
	}

	.axis-labels {
		display: flex;
		justify-content: space-between;
		gap: 0.5rem;
		flex-wrap: wrap;
		font-family: var(--font-mono);
		font-size: 0.66rem;
		color: var(--alt-slate);
		font-variant-numeric: tabular-nums;
	}

	.dim {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-ash);
	}
</style>
