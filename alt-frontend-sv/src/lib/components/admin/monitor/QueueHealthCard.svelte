<script lang="ts">
import type { MetricResult } from "$lib/gen/alt/admin_monitor/v1/admin_monitor_pb";
import SLISparkline from "$lib/components/knowledge-home-admin/SLISparkline.svelte";
import { formatValue, stateBadge } from "./format";

let {
	metrics,
}: {
	metrics: MetricResult[];
} = $props();

function find(key: string): MetricResult | undefined {
	return metrics.find((m) => m.key === key);
}

function latest(m: MetricResult | undefined): number | null {
	const last = m?.series[0]?.points.at(-1)?.value;
	return last != null && Number.isFinite(last) ? last : null;
}

function points(m: MetricResult | undefined): number[] {
	const out: number[] = [];
	for (const s of m?.series ?? []) {
		for (const p of s.points ?? []) out.push(p.value);
	}
	return out;
}

function sumLatestByLabel(m: MetricResult | undefined): {
	label: string;
	value: number;
}[] {
	const out: { label: string; value: number }[] = [];
	for (const s of m?.series ?? []) {
		const topic = s.labels?.topic ?? "(no topic)";
		const last = s.points.at(-1)?.value;
		if (last != null && Number.isFinite(last))
			out.push({ label: topic, value: last });
	}
	return out.sort((a, b) => b.value - a.value);
}

const redis = $derived(find("mqhub_redis"));
const publish = $derived(find("mqhub_publish_rate"));

const redisBadge = $derived(stateBadge(latest(redis), "bool"));
const totalPublishRate = $derived.by(() => {
	let total = 0;
	let any = false;
	for (const s of publish?.series ?? []) {
		const last = s.points.at(-1)?.value;
		if (last != null && Number.isFinite(last)) {
			total += last;
			any = true;
		}
	}
	return any ? total : null;
});
</script>

<section class="queue" aria-label="Queue health">
	<h2 class="section-head">Queue health · mq-hub</h2>

	<div class="grid">
		<article class="cell">
			<header>
				<span class="label">Redis connection</span>
				<span class="badge" data-state={redisBadge.text}>
					<span class="glyph" aria-hidden="true">{redisBadge.glyph}</span>
					<span class="state-text">{redisBadge.text}</span>
				</span>
			</header>
			<div class="value">
				<span class="num">{formatValue(latest(redis), "bool")}</span>
			</div>
		</article>

		<article class="cell">
			<header>
				<span class="label">Publish rate (total)</span>
			</header>
			<div class="value">
				<span class="num">{formatValue(totalPublishRate, "msg/s")}</span>
				<span class="unit">msg/s</span>
			</div>
			<div class="spark" aria-hidden="true">
				{#if points(publish).length >= 2}
					<SLISparkline values={points(publish)} width={240} height={28} />
				{/if}
			</div>
		</article>

		<article class="cell wide">
			<header>
				<span class="label">Publish rate by topic</span>
			</header>
			<ul class="by-topic">
				{#each sumLatestByLabel(publish) as t (t.label)}
					<li>
						<span class="topic">{t.label}</span>
						<span class="t-value">{formatValue(t.value, "msg/s")} msg/s</span>
					</li>
				{:else}
					<li class="dim">no topics</li>
				{/each}
			</ul>
		</article>
	</div>
</section>

<style>
	.queue {
		display: grid;
		gap: 0.55rem;
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
		grid-template-columns: 1fr 1fr 1.4fr;
		gap: 0.7rem;
	}

	@media (max-width: 1100px) {
		.grid {
			grid-template-columns: 1fr;
		}
	}

	.cell {
		display: grid;
		gap: 0.35rem;
		padding: 0.7rem 0.85rem;
		border: 0.5px solid var(--obs-rule, var(--surface-border));
		background: var(--surface);
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

	.badge {
		display: inline-flex;
		gap: 0.25rem;
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-slate);
	}

	.badge[data-state="up"] .glyph,
	.badge[data-state="up"] .state-text {
		color: var(--obs-good, var(--alt-success));
	}

	.badge[data-state="down"] .glyph,
	.badge[data-state="down"] .state-text {
		color: var(--obs-critical, var(--alt-error));
	}

	.value {
		display: flex;
		align-items: baseline;
		gap: 0.35rem;
	}

	.num {
		font-family: var(--font-display, var(--font-serif));
		font-size: 1.6rem;
		font-weight: 500;
		color: var(--alt-charcoal);
		font-variant-numeric: tabular-nums;
	}

	.unit {
		font-family: var(--font-mono);
		font-size: 0.74rem;
		color: var(--alt-ash);
	}

	.by-topic {
		list-style: none;
		padding: 0;
		margin: 0;
		display: grid;
		gap: 0.2rem;
	}

	.by-topic li {
		display: flex;
		justify-content: space-between;
		font-family: var(--font-mono);
		font-size: 0.76rem;
	}

	.topic {
		color: var(--alt-slate);
	}

	.t-value {
		color: var(--alt-charcoal);
		font-variant-numeric: tabular-nums;
	}

	.dim {
		color: var(--alt-ash);
		font-style: italic;
	}
</style>
