<script lang="ts">
import MonitorSparkline from "./MonitorSparkline.svelte";
import {
	BURN,
	badge,
	buildEdgeSignals,
	edgeEmptyText,
	type LatestValueOptions,
	type MetricMap,
	type Severity,
	seriesPoints,
} from "./model";

let {
	metrics,
	evalOpts,
}: {
	metrics: MetricMap;
	evalOpts?: LatestValueOptions;
} = $props();

const edge = $derived(buildEdgeSignals(metrics, evalOpts));

const pageSeverity = $derived.by((): Severity => {
	if (edge.burn.h1 === null || edge.burn.m5 === null) return "nodata";
	return edge.pageBurn ? "crit" : "ok";
});

const ticketSeverity = $derived.by((): Severity => {
	if (edge.burn.h6 === null || edge.burn.m30 === null) return "nodata";
	return edge.ticketBurn ? "warn" : "ok";
});

const pageBadge = $derived(badge(pageSeverity));
const ticketBadge = $derived(badge(ticketSeverity));

const pts1h = $derived(seriesPoints(metrics.get("availability_burn_1h")));
const pts5m = $derived(seriesPoints(metrics.get("edge_burn_5m")));
const pts6h = $derived(seriesPoints(metrics.get("availability_burn_6h")));
const pts30m = $derived(seriesPoints(metrics.get("edge_burn_30m")));

function formatBurn(val: number | null, key: string): string {
	if (val === null || !Number.isFinite(val)) {
		return edgeEmptyText(metrics, key, evalOpts);
	}
	return `${val.toFixed(1)}×`;
}
</script>

<div class="slo" role="group" aria-label="SLO burn rate">
	<header class="slo-head">
		<h3 class="sub-head">SLO 99.9% availability at the edge</h3>
		<span class="rule-note">
			Multi-window burn-rate alerts (page: 1h &amp; 5m &gt; {BURN.pageThreshold}× ·
			ticket: 6h &amp; 30m &gt; {BURN.ticketThreshold}×)
		</span>
	</header>

	<div class="grid">
		<article class="cell" data-severity={pageBadge.severity}>
			<header>
				<span class="label">Page burn pair (1h &amp; 5m vs {BURN.pageThreshold}×)</span>
				<span class="badge" data-severity={pageBadge.severity}>
					<span class="glyph" aria-hidden="true">{pageBadge.glyph}</span>
					<span>{pageBadge.label}</span>
				</span>
			</header>

			<div class="pair-values">
				<div class="win">
					<span class="win-label">1h window</span>
					<span class="num" class:no-traffic={edge.burn.h1 === null}>
						{formatBurn(edge.burn.h1, "availability_burn_1h")}
					</span>
					<MonitorSparkline
						points={pts1h}
						label="1h burn rate"
						threshold={BURN.pageThreshold}
						width={200}
						height={32}
					/>
				</div>
				<div class="win">
					<span class="win-label">5m window</span>
					<span class="num" class:no-traffic={edge.burn.m5 === null}>
						{formatBurn(edge.burn.m5, "edge_burn_5m")}
					</span>
					<MonitorSparkline
						points={pts5m}
						label="5m burn rate"
						threshold={BURN.pageThreshold}
						width={200}
						height={32}
					/>
				</div>
			</div>

			<p class="threshold-line">
				crit when both 1h &gt; {BURN.pageThreshold}× and 5m &gt; {BURN.pageThreshold}×
			</p>
		</article>

		<article class="cell" data-severity={ticketBadge.severity}>
			<header>
				<span class="label">Ticket burn pair (6h &amp; 30m vs {BURN.ticketThreshold}×)</span>
				<span class="badge" data-severity={ticketBadge.severity}>
					<span class="glyph" aria-hidden="true">{ticketBadge.glyph}</span>
					<span>{ticketBadge.label}</span>
				</span>
			</header>

			<div class="pair-values">
				<div class="win">
					<span class="win-label">6h window</span>
					<span class="num" class:no-traffic={edge.burn.h6 === null}>
						{formatBurn(edge.burn.h6, "availability_burn_6h")}
					</span>
					<MonitorSparkline
						points={pts6h}
						label="6h burn rate"
						threshold={BURN.ticketThreshold}
						width={200}
						height={32}
					/>
				</div>
				<div class="win">
					<span class="win-label">30m window</span>
					<span class="num" class:no-traffic={edge.burn.m30 === null}>
						{formatBurn(edge.burn.m30, "edge_burn_30m")}
					</span>
					<MonitorSparkline
						points={pts30m}
						label="30m burn rate"
						threshold={BURN.ticketThreshold}
						width={200}
						height={32}
					/>
				</div>
			</div>

			<p class="threshold-line">
				warn when both 6h &gt; {BURN.ticketThreshold}× and 30m &gt; {BURN.ticketThreshold}×
			</p>
		</article>
	</div>
</div>

<style>
	.slo {
		display: grid;
		gap: 0.55rem;
		min-width: 0;
	}

	.slo-head {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		flex-wrap: wrap;
		gap: 0.5rem;
	}

	.sub-head {
		font-family: var(--font-body);
		font-size: 0.72rem;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		margin: 0;
		color: var(--alt-slate);
	}

	.rule-note {
		font-family: var(--font-mono);
		font-size: 0.68rem;
		color: var(--alt-slate);
	}

	.grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 0.7rem;
		min-width: 0;
	}

	@media (max-width: 900px) {
		.grid {
			grid-template-columns: 1fr;
		}
	}

	.cell {
		display: grid;
		gap: 0.5rem;
		padding: 0.85rem 0.95rem;
		border: 0.5px solid var(--obs-rule, var(--surface-border));
		border-top: 2px solid var(--alt-charcoal);
		background: var(--surface-bg);
		min-width: 0;
	}

	.cell[data-severity="crit"] {
		border-top-color: var(--obs-critical, var(--alt-error));
	}

	.cell[data-severity="warn"] {
		border-top-color: var(--obs-warn, var(--alt-warning));
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

	.pair-values {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 0.75rem;
		min-width: 0;
	}

	@media (max-width: 480px) {
		.pair-values {
			grid-template-columns: 1fr;
		}
	}

	.win {
		display: grid;
		gap: 0.2rem;
		min-width: 0;
	}

	.win-label {
		font-family: var(--font-mono);
		font-size: 0.68rem;
		color: var(--alt-slate);
	}

	.num {
		font-family: var(--font-display, var(--font-serif));
		font-size: 1.65rem;
		font-weight: 500;
		font-variant-numeric: tabular-nums;
		color: var(--alt-charcoal);
	}

	.num.no-traffic {
		font-family: var(--font-mono);
		font-size: 0.95rem;
		color: var(--alt-ash);
	}

	.threshold-line {
		margin: 0;
		font-family: var(--font-mono);
		font-size: 0.68rem;
		color: var(--alt-slate);
	}
</style>
