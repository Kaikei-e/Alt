<script lang="ts">
import SLISparkline from "../SLISparkline.svelte";
import MetricSeriesDigest from "./MetricSeriesDigest.svelte";
import type { MetricResult } from "$lib/gen/alt/admin_monitor/v1/admin_monitor_pb";
import { computeDelta, type SimpleSeries } from "./util";

type Threshold = {
	warn?: (v: number) => boolean;
	value?: number; // optional sparkline reference line
};

let {
	metric,
	label,
	threshold,
	preferLabel = "job",
}: {
	metric: MetricResult | undefined;
	label: string;
	threshold?: Threshold;
	preferLabel?: string;
} = $props();

// All series as framework-agnostic shape for util + digest.
const simpleSeries = $derived(() =>
	(metric?.series ?? []).map<SimpleSeries>((s) => ({
		labels: s.labels ?? {},
		points: s.points.map((p) => ({ time: p.time, value: p.value })),
	})),
);

// Primary (first) series: used for the sparkline and the lead value.
const primary = $derived(() => simpleSeries()[0]);
const primaryPoints = $derived(() => (primary()?.points ?? []).map((p) => p.value));
const leadValue = $derived(() => {
	const pts = primary()?.points ?? [];
	return pts.length ? pts[pts.length - 1].value : null;
});
const delta = $derived(() => {
	const pts = primary()?.points ?? [];
	return pts.length ? computeDelta(pts) : null;
});
const warn = $derived(() => {
	const v = leadValue();
	return v != null && threshold?.warn ? threshold.warn(v) : false;
});

const valueText = $derived(() => formatValue(leadValue(), metric?.unit));
const deltaText = $derived(() => {
	const d = delta();
	if (!d) return null;
	const direction = d.direction;
	const glyph = direction === "up" ? "▲" : direction === "down" ? "▼" : "·";
	const abs = formatValue(Math.abs(d.absolute), metric?.unit);
	const sign = d.percent === 0 ? "" : d.percent > 0 ? "+" : "−";
	const pct = Number.isFinite(d.percent) ? `${sign}${Math.abs(d.percent).toFixed(1)}%` : "";
	return { glyph, abs, pct, direction };
});

function formatValue(v: number | null, unit: string | undefined): string {
	if (v == null || !Number.isFinite(v)) return "—";
	if (unit === "bytes") return formatBytes(v);
	if (unit === "ratio") return `${(v * 100).toFixed(2)}%`;
	if (unit === "seconds") return `${(v * 1000).toFixed(0)} ms`;
	if (unit === "bool") return v >= 1 ? "up" : "down";
	if (Math.abs(v) >= 1000) return v.toFixed(0);
	if (Math.abs(v) >= 1) return v.toFixed(2);
	return v.toFixed(3);
}

function formatBytes(n: number): string {
	const units = ["B", "KiB", "MiB", "GiB"];
	let v = n;
	let i = 0;
	while (v >= 1024 && i < units.length - 1) {
		v /= 1024;
		i += 1;
	}
	return `${v.toFixed(v >= 100 ? 0 : v >= 10 ? 1 : 2)} ${units[i]}`;
}
</script>

<article
	class="row"
	class:warn={warn()}
	class:degraded={metric?.degraded}
	aria-label="{label} {valueText()}{metric?.unit ? ` ${metric.unit}` : ''}"
>
	<header class="row-head">
		<span class="label">{label}</span>
		{#if deltaText()}
			<span class="delta" data-direction={deltaText()!.direction}>
				<span class="glyph" aria-hidden="true">{deltaText()!.glyph}</span>
				<span class="delta-abs">{deltaText()!.abs}{metric?.unit && metric.unit !== 'ratio' && metric.unit !== 'bool' ? ` ${metric.unit}` : ''}</span>
				{#if deltaText()!.pct}
					<span class="delta-pct">{deltaText()!.pct}</span>
				{/if}
				<span class="delta-window">5m</span>
			</span>
		{/if}
	</header>

	<div class="row-body">
		<span class="value">
			<span class="value-num">{valueText()}</span>
			{#if metric?.unit && metric.unit !== 'bool'}
				<span class="value-unit">{metric.unit}</span>
			{/if}
		</span>
		<span class="spark" aria-hidden="true">
			{#if primaryPoints().length >= 2}
				<SLISparkline
					values={primaryPoints()}
					threshold={threshold?.value}
					width={140}
					height={28}
				/>
			{:else}
				<span class="dim">—</span>
			{/if}
		</span>
		<span class="digest">
			<MetricSeriesDigest
				series={simpleSeries()}
				preferLabel={preferLabel}
				unit={metric?.unit ?? ""}
				limit={3}
			/>
		</span>
	</div>

	{#if metric?.grafanaUrl}
		<a
			class="grafana"
			href={metric.grafanaUrl}
			target="_blank"
			rel="noreferrer noopener"
			aria-label="Open {label} in Grafana"
		>
			⋯ open in Grafana
		</a>
	{/if}

	{#if metric?.degraded}
		<div class="reason" title={metric.reason}>{metric.reason || "degraded"}</div>
	{/if}
</article>

<style>
	.row {
		display: grid;
		grid-template-columns: 1fr;
		gap: 0.1rem;
		padding: 0.55rem 0 0.6rem;
		border-bottom: 0.5px solid var(--obs-rule, var(--surface-border));
	}

	.row.degraded {
		color: var(--obs-muted, var(--alt-ash));
	}

	.row.warn .value-num {
		color: var(--obs-warn, var(--alt-warning));
		font-weight: 600;
	}

	.row-head {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		gap: 1rem;
	}

	.label {
		font-family: var(--font-body);
		font-size: 0.68rem;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--alt-slate);
	}

	.delta {
		display: inline-flex;
		align-items: baseline;
		gap: 0.35rem;
		font-size: 0.74rem;
		color: var(--alt-slate);
		font-variant-numeric: tabular-nums;
	}

	.delta[data-direction="up"] .glyph,
	.delta[data-direction="up"] .delta-pct {
		color: var(--obs-warn, var(--alt-warning));
	}

	.delta[data-direction="down"] .glyph,
	.delta[data-direction="down"] .delta-pct {
		color: var(--obs-good, var(--alt-success));
	}

	.delta[data-direction="flat"] .glyph {
		color: var(--alt-ash);
	}

	.delta-window {
		color: var(--alt-ash);
		font-size: 0.66rem;
		letter-spacing: 0.1em;
		text-transform: uppercase;
	}

	.row-body {
		display: grid;
		grid-template-columns: minmax(7rem, auto) 150px 1fr;
		align-items: center;
		gap: 0.9rem;
		padding-top: 0.1rem;
	}

	.value {
		font-family: var(--font-body);
		font-variant-numeric: tabular-nums;
		color: var(--alt-charcoal);
	}

	.value-num {
		font-size: 1.05rem;
		font-weight: 500;
	}

	.value-unit {
		font-size: 0.72rem;
		color: var(--alt-ash);
		margin-left: 0.35ch;
	}

	.spark {
		display: inline-flex;
		align-items: center;
	}

	.digest {
		min-width: 0;
		overflow: hidden;
	}

	.dim {
		color: var(--alt-ash);
	}

	.grafana {
		justify-self: end;
		font-size: 0.7rem;
		color: var(--alt-slate);
		text-decoration: none;
		border-bottom: 1px dotted var(--alt-slate);
		opacity: 0;
		transition: opacity 0.15s ease;
	}

	.row:hover .grafana,
	.row:focus-within .grafana,
	.grafana:focus-visible {
		opacity: 1;
		color: var(--alt-charcoal);
		border-color: var(--alt-charcoal);
	}

	.reason {
		font-size: 0.72rem;
		color: var(--obs-muted, var(--alt-ash));
		font-style: italic;
	}

	@media (prefers-reduced-motion: reduce) {
		.grafana {
			transition: none;
		}
	}
</style>
