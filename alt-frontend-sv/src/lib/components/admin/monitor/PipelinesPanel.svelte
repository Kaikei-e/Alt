<script lang="ts">
import { formatValue } from "./format";
import { badge, type Pipelines, type Severity, THRESHOLDS } from "./model";

let {
	pipelines,
}: {
	pipelines: Pipelines;
} = $props();

const redisSeverity = $derived.by((): Severity => {
	if (pipelines.redisUp === null) return "nodata";
	return pipelines.redisUp ? "ok" : "crit";
});

const redisBadge = $derived(badge(redisSeverity));
const outboxBadge = $derived(badge(pipelines.outboxSeverity));

function formatAge(sec: number | null): string {
	if (sec === null || !Number.isFinite(sec)) return "NO DATA";
	const s = Math.max(0, Math.round(sec));
	if (s < 60) return `${s}s`;
	if (s < 3600) {
		const m = Math.floor(s / 60);
		const rem = s % 60;
		return rem > 0 ? `${m}m ${rem}s (${s}s)` : `${m}m (${s}s)`;
	}
	const h = Math.floor(s / 3600);
	const m = Math.floor((s % 3600) / 60);
	return `${h}h ${m}m (${s}s)`;
}
</script>

<section id="pipelines" class="pipelines" aria-label="Pipelines and queues">
	<header class="section-bar">
		<h2 class="section-head">Pipelines &amp; Queues</h2>
		<span class="threshold-meta">
			backlog age {THRESHOLDS.backlogAge.display} · outbox failed (24h) {THRESHOLDS.outboxFailed.display}
		</span>
	</header>

	<div class="cards">
		<article class="cell">
			<header>
				<span class="label">mq-hub Redis connection</span>
				<span class="badge" data-severity={redisBadge.severity}>
					<span class="glyph" aria-hidden="true">{redisBadge.glyph}</span>
					<span>{redisBadge.label}</span>
				</span>
			</header>
			<div class="value">
				<span class="num" class:muted={pipelines.redisUp === null}>
					{pipelines.redisUp === null
						? "NO DATA"
						: pipelines.redisUp
							? "up"
							: "down"}
				</span>
			</div>
		</article>

		<article class="cell">
			<header>
				<span class="label">Push delivery pending</span>
			</header>
			<div class="value">
				<span class="num" class:muted={pipelines.pushPending === null}>
					{pipelines.pushPending !== null
						? Math.round(pipelines.pushPending)
						: "NO DATA"}
				</span>
			</div>
		</article>

		<article class="cell">
			<header>
				<span class="label">Harvester outbox failed (24h)</span>
				<span class="badge" data-severity={outboxBadge.severity}>
					<span class="glyph" aria-hidden="true">{outboxBadge.glyph}</span>
					<span>{outboxBadge.label}</span>
				</span>
			</header>
			<div class="value">
				<span class="num" class:muted={pipelines.outboxFailed24h === null}>
					{pipelines.outboxFailed24h !== null
						? Math.round(pipelines.outboxFailed24h)
						: "NO DATA"}
				</span>
			</div>
			<div class="sub-threshold">{THRESHOLDS.outboxFailed.display}</div>
		</article>

		<article class="cell">
			<header>
				<span class="label">Publish rate by stream</span>
			</header>
			{#if !pipelines.publishRateAvailable}
				<p class="dim">
					NO DATA — publish rate unavailable{pipelines.publishRateReason ? ` (${pipelines.publishRateReason})` : ""}
				</p>
			{:else if pipelines.publishByTopic.length === 0}
				<p class="dim">No active publish streams.</p>
			{:else}
				<ul class="topic-list">
					{#each pipelines.publishByTopic as t (t.topic)}
						<li>
							<span class="topic">{t.topic}</span>
							<span class="rate">{formatValue(t.rate, "msg/s")} msg/s</span>
						</li>
					{/each}
				</ul>
			{/if}
		</article>
	</div>

	<div class="table-wrap">
		<h3 class="sub-head">Queue backlog oldest age</h3>
		{#if !pipelines.backlogAvailable}
			<p class="dim">
				NO DATA — queue backlog unavailable{pipelines.backlogReason ? ` (${pipelines.backlogReason})` : ""}
			</p>
		{:else if pipelines.backlog.length === 0}
			<p class="dim">No queue backlogs reported.</p>
		{:else}
			<table>
				<thead>
					<tr>
						<th scope="col">State</th>
						<th scope="col">Queue</th>
						<th scope="col">Job</th>
						<th scope="col" class="num-col">Oldest pending age</th>
					</tr>
				</thead>
				<tbody>
					{#each pipelines.backlog as b (`${b.job}-${b.queue}`)}
						{@const bBadge = badge(b.severity)}
						<tr data-severity={bBadge.severity}>
							<td>
								<span class="badge" data-severity={bBadge.severity}>
									<span class="glyph" aria-hidden="true">{bBadge.glyph}</span>
									<span>{bBadge.label}</span>
								</span>
							</td>
							<td class="mono">{b.queue}</td>
							<td class="mono">{b.job}</td>
							<td class="num">{formatAge(b.oldestAgeSeconds)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</div>
</section>

<style>
	.pipelines {
		display: grid;
		gap: 0.65rem;
		min-width: 0;
	}

	.section-bar {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		flex-wrap: wrap;
		gap: 0.5rem;
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

	.sub-head {
		font-family: var(--font-body);
		font-size: 0.68rem;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--alt-slate);
		margin: 0 0 0.35rem;
	}

	.threshold-meta,
	.sub-threshold {
		font-family: var(--font-mono);
		font-size: 0.68rem;
		color: var(--alt-slate);
	}

	.cards {
		display: grid;
		grid-template-columns: repeat(4, minmax(0, 1fr));
		gap: 0.7rem;
		min-width: 0;
	}

	@media (max-width: 1100px) {
		.cards {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
	}

	@media (max-width: 540px) {
		.cards {
			grid-template-columns: 1fr;
		}
	}

	.cell {
		display: grid;
		gap: 0.35rem;
		padding: 0.75rem 0.85rem;
		border: 0.5px solid var(--obs-rule, var(--surface-border));
		border-top: 2px solid var(--alt-charcoal);
		background: var(--surface-bg);
		min-width: 0;
	}

	header {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		gap: 0.45rem;
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
		font-size: 1.65rem;
		font-weight: 500;
		color: var(--alt-charcoal);
		font-variant-numeric: tabular-nums;
	}

	.num.muted {
		font-family: var(--font-mono);
		font-size: 0.95rem;
		color: var(--alt-ash);
	}

	.topic-list {
		list-style: none;
		padding: 0;
		margin: 0;
		display: grid;
		gap: 0.2rem;
	}

	.topic-list li {
		display: flex;
		justify-content: space-between;
		gap: 0.5rem;
		font-family: var(--font-mono);
		font-size: 0.74rem;
	}

	.topic {
		color: var(--alt-slate);
		overflow-wrap: anywhere;
	}

	.rate {
		color: var(--alt-charcoal);
		white-space: nowrap;
		font-variant-numeric: tabular-nums;
	}

	.table-wrap {
		width: 100%;
		max-width: 100%;
		overflow-x: auto;
		min-width: 0;
	}

	table {
		width: 100%;
		border-collapse: collapse;
		font-variant-numeric: tabular-nums;
	}

	thead th {
		font-family: var(--font-body);
		font-size: 0.66rem;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--alt-slate);
		text-align: left;
		padding: 0.4rem 0.6rem 0.4rem 0;
		border-bottom: 1px solid var(--alt-charcoal);
		white-space: nowrap;
	}

	.num-col {
		text-align: right;
	}

	tbody td {
		padding: 0.5rem 0.6rem 0.5rem 0;
		font-size: 0.82rem;
		color: var(--alt-charcoal);
		border-bottom: 0.5px solid var(--obs-rule, var(--surface-border));
		vertical-align: baseline;
	}

	.mono {
		font-family: var(--font-mono);
		font-size: 0.78rem;
	}

	.num {
		text-align: right;
	}

	tbody td.num {
		font-family: var(--font-mono);
		font-size: 0.78rem;
		white-space: nowrap;
	}

	.dim {
		margin: 0;
		font-family: var(--font-mono);
		font-size: 0.74rem;
		color: var(--alt-ash);
	}
</style>
