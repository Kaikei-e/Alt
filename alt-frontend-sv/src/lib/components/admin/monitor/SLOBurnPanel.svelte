<script lang="ts">
import type { MetricResult } from "$lib/gen/alt/admin_monitor/v1/admin_monitor_pb";
import SLISparkline from "$lib/components/knowledge-home-admin/SLISparkline.svelte";
import { burnSeverity } from "./format";

let {
	metrics,
}: {
	metrics: MetricResult[];
} = $props();

function find(key: string): MetricResult | undefined {
	return metrics.find((m) => m.key === key);
}

const burn1h = $derived(find("availability_burn_1h"));
const burn6h = $derived(find("availability_burn_6h"));
const errorRatio = $derived(find("http_error_ratio"));

function latest(m: MetricResult | undefined): number | null {
	const last = m?.series[0]?.points.at(-1)?.value;
	return last != null && Number.isFinite(last) ? last : null;
}

function pointsOf(m: MetricResult | undefined): number[] {
	const pts: number[] = [];
	for (const s of m?.series ?? []) {
		for (const p of s.points ?? []) pts.push(p.value);
	}
	return pts;
}

// 30-day SLO compliance — approximate as 1 - (max recent error ratio over the
// visible window). Operators can read the full SLO from Grafana when the
// number looks ambiguous; this is the "is this getting worse" view.
const sloEstimate = $derived.by(() => {
	const v = latest(errorRatio);
	if (v == null) return null;
	return Math.max(0, 1 - v);
});

const tiers = [
	{ value: 14.4, label: "page-1" },
	{ value: 6, label: "page-2" },
	{ value: 1, label: "ticket" },
];

function tierLabel(value: number | null): string {
	const s = burnSeverity(value);
	if (s === "page1") return "page-1 burn";
	if (s === "page2") return "page-2 burn";
	if (s === "ticket") return "ticket burn";
	return "within budget";
}
</script>

<section class="slo" aria-label="SLO burn rate">
	<h2 class="section-head">SLO burn rate — 99.9% availability</h2>

	<div class="grid">
		<article class="cell" data-tier={burnSeverity(latest(burn1h))}>
			<header>
				<span class="label">Burn rate · 1h</span>
				<span class="tier">{tierLabel(latest(burn1h))}</span>
			</header>
			<div class="value">
				<span class="num">
					{latest(burn1h) != null ? latest(burn1h)!.toFixed(2) + "×" : "—"}
				</span>
			</div>
			<div class="spark" aria-hidden="true">
				{#if pointsOf(burn1h).length >= 2}
					<SLISparkline
						values={pointsOf(burn1h)}
						threshold={14.4}
						width={240}
						height={36}
					/>
				{:else}
					<span class="dim">no series</span>
				{/if}
			</div>
			<ul class="tiers">
				{#each tiers as t (t.value)}
					<li>{t.label} ≥ {t.value}×</li>
				{/each}
			</ul>
		</article>

		<article class="cell" data-tier={burnSeverity(latest(burn6h))}>
			<header>
				<span class="label">Burn rate · 6h</span>
				<span class="tier">{tierLabel(latest(burn6h))}</span>
			</header>
			<div class="value">
				<span class="num">
					{latest(burn6h) != null ? latest(burn6h)!.toFixed(2) + "×" : "—"}
				</span>
			</div>
			<div class="spark" aria-hidden="true">
				{#if pointsOf(burn6h).length >= 2}
					<SLISparkline
						values={pointsOf(burn6h)}
						threshold={6}
						width={240}
						height={36}
					/>
				{:else}
					<span class="dim">no series</span>
				{/if}
			</div>
			<ul class="tiers">
				<li>page-2 ≥ 6×</li>
				<li>ticket ≥ 1×</li>
			</ul>
		</article>

		<article class="cell" data-tier="ok">
			<header>
				<span class="label">SLO estimate (visible window)</span>
				<span class="tier"></span>
			</header>
			<div class="value">
				<span class="num">
					{sloEstimate != null
						? `${(sloEstimate! * 100).toFixed(3)}%`
						: "—"}
				</span>
			</div>
			<p class="note">
				Approximate from the displayed window's error ratio. For the full 30-day SLO, check the
				Prometheus recording rule directly.
			</p>
		</article>
	</div>
</section>

<style>
	.slo {
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
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 0.7rem;
	}

	@media (max-width: 1100px) {
		.grid {
			grid-template-columns: 1fr;
		}
	}

	.cell {
		display: grid;
		grid-template-rows: auto auto auto auto;
		gap: 0.35rem;
		padding: 0.85rem 0.95rem;
		border: 0.5px solid var(--obs-rule, var(--surface-border));
		border-top: 2px solid var(--alt-charcoal);
		background: var(--surface);
	}

	.cell[data-tier="page1"] {
		border-top-color: var(--obs-critical, var(--alt-error));
	}

	.cell[data-tier="page2"] {
		border-top-color: var(--obs-warn, var(--alt-warning));
	}

	header {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
	}

	.label {
		font-family: var(--font-body);
		font-size: 0.66rem;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--alt-slate);
	}

	.tier {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-slate);
		text-transform: uppercase;
		letter-spacing: 0.1em;
	}

	.cell[data-tier="page1"] .tier {
		color: var(--obs-critical, var(--alt-error));
	}

	.cell[data-tier="page2"] .tier {
		color: var(--obs-warn, var(--alt-warning));
	}

	.value .num {
		font-family: var(--font-display, var(--font-serif));
		font-size: 1.9rem;
		font-weight: 500;
		font-variant-numeric: tabular-nums;
		color: var(--alt-charcoal);
	}

	.note {
		font-size: 0.72rem;
		color: var(--alt-slate);
		margin: 0;
	}

	.tiers {
		list-style: none;
		padding: 0;
		margin: 0;
		font-family: var(--font-mono);
		font-size: 0.68rem;
		color: var(--alt-ash);
		display: flex;
		gap: 0.85rem;
		flex-wrap: wrap;
	}

	.dim {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-ash);
	}
</style>
