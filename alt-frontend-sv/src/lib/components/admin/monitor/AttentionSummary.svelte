<script lang="ts">
import {
	type AlertRow,
	type AttentionItem,
	badge,
	type ServiceRow,
} from "./model";

let {
	items,
	serviceRows,
	alertRows,
	alertsAvailable,
}: {
	items: AttentionItem[];
	serviceRows: ServiceRow[];
	alertRows: AlertRow[];
	alertsAvailable: boolean;
} = $props();

const targetsUp = $derived(
	serviceRows.reduce((sum, r) => sum + r.targetsUp, 0),
);
const targetsTotal = $derived(
	serviceRows.reduce((sum, r) => sum + r.targetsTotal, 0),
);
const firingAlertsCount = $derived(
	alertRows.filter((a) => !a.isHeartbeat && a.state === "firing").length,
);

const overallSeverity = $derived.by(() => {
	if (items.some((i) => i.severity === "crit")) return "crit";
	if (items.length > 0) return "warn";
	if (serviceRows.length === 0 || !alertsAvailable) return "nodata";
	return "ok";
});

const overallBadge = $derived(badge(overallSeverity));

const verdictText = $derived.by(() => {
	const alertsText = alertsAvailable
		? `${firingAlertsCount} ${firingAlertsCount === 1 ? "alert" : "alerts"} firing`
		: "alerts unknown";
	const counts = `${targetsUp}/${targetsTotal} targets up · ${alertsText}`;
	if (items.length === 0 && serviceRows.length === 0 && !alertsAvailable) {
		return `NO DATA — target availability and alert state unavailable · ${counts}`;
	}
	if (items.length === 0 && serviceRows.length === 0) {
		return `NO DATA — target availability unavailable · ${counts}`;
	}
	if (items.length === 0 && !alertsAvailable) {
		return `NO DATA — alert state unavailable · ${counts}`;
	}
	if (items.length === 0) {
		return `All checks passing · ${counts}`;
	}
	const issueNoun = items.length === 1 ? "issue needs" : "issues need";
	return `${items.length} ${issueNoun} attention · ${counts}`;
});
</script>

<section id="attention" class="attention" aria-label="Attention">
	<header class="section-bar">
		<h2 class="section-head">Attention</h2>
		<span class="verdict-badge" data-severity={overallBadge.severity}>
			<span class="glyph" aria-hidden="true">{overallBadge.glyph}</span>
			<span>{overallBadge.label}</span>
		</span>
	</header>

	<div class="verdict-box" data-severity={overallBadge.severity}>
		<p class="verdict-line">{verdictText}</p>

		{#if items.length > 0}
			<ul class="items">
				{#each items as item, idx (`${item.section}-${idx}-${item.text}`)}
					{@const b = badge(item.severity)}
					<li class="item" data-severity={item.severity}>
						<span class="item-badge" data-severity={b.severity}>
							<span class="glyph" aria-hidden="true">{b.glyph}</span>
							<span class="state-text">{b.label}</span>
						</span>
						<span class="item-section">{item.section}</span>
						<a class="item-link" href={item.anchor}>{item.text}</a>
					</li>
				{/each}
			</ul>
		{/if}
	</div>
</section>

<style>
	.attention {
		display: grid;
		gap: 0.5rem;
		min-width: 0;
	}

	.section-bar {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
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

	.verdict-badge {
		display: inline-flex;
		align-items: baseline;
		gap: 0.3rem;
		font-family: var(--font-mono);
		font-size: 0.72rem;
		font-weight: 600;
		letter-spacing: 0.08em;
	}

	.verdict-badge[data-severity="ok"] {
		color: var(--obs-good, var(--alt-success));
	}

	.verdict-badge[data-severity="warn"] {
		color: var(--obs-warn, var(--alt-warning));
	}

	.verdict-badge[data-severity="crit"] {
		color: var(--obs-critical, var(--alt-error));
	}

	.verdict-box {
		display: grid;
		gap: 0.55rem;
		padding: 0.8rem 0.95rem;
		border: 0.5px solid var(--obs-rule, var(--surface-border));
		border-left: 3px solid var(--alt-charcoal);
		background: var(--surface-bg);
		min-width: 0;
	}

	.verdict-box[data-severity="ok"] {
		border-left-color: var(--obs-good, var(--alt-success));
	}

	.verdict-box[data-severity="warn"] {
		border-left-color: var(--obs-warn, var(--alt-warning));
	}

	.verdict-box[data-severity="crit"] {
		border-left-color: var(--obs-critical, var(--alt-error));
	}

	.verdict-line {
		margin: 0;
		font-family: var(--font-body);
		font-size: 0.92rem;
		font-weight: 600;
		color: var(--alt-charcoal);
		font-variant-numeric: tabular-nums;
	}

	.items {
		list-style: none;
		margin: 0;
		padding: 0;
		display: grid;
		gap: 0.35rem;
		border-top: 0.5px solid var(--obs-rule, var(--surface-border));
		padding-top: 0.55rem;
	}

	.item {
		display: grid;
		grid-template-columns: auto auto 1fr;
		align-items: baseline;
		gap: 0.55rem;
		font-size: 0.82rem;
		min-width: 0;
	}

	.item-badge {
		display: inline-flex;
		align-items: baseline;
		gap: 0.25rem;
		font-family: var(--font-mono);
		font-size: 0.7rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		min-width: 3.6rem;
	}

	.item-badge[data-severity="crit"] {
		color: var(--obs-critical, var(--alt-error));
	}

	.item-badge[data-severity="warn"] {
		color: var(--obs-warn, var(--alt-warning));
	}

	.item-section {
		font-family: var(--font-mono);
		font-size: 0.68rem;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--alt-slate);
		min-width: 5.5rem;
	}

	.item-link {
		color: var(--alt-charcoal);
		text-decoration: underline;
		text-underline-offset: 2px;
		overflow-wrap: anywhere;
	}

	.item-link:hover {
		color: var(--interactive-text-hover, var(--alt-primary));
	}

	@media (max-width: 540px) {
		.item {
			grid-template-columns: auto 1fr;
		}

		.item-link {
			grid-column: 1 / -1;
		}
	}
</style>
