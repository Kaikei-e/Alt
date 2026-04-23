<script lang="ts">
	import type { KnowledgeLoopEntryData } from "$lib/connect/knowledge_loop";

	let { entry, stagger = 0 }: { entry: KnowledgeLoopEntryData; stagger?: number } = $props();

	/**
	 * Alt-Paper responsibility-driven expression for Knowledge Loop:
	 * each entry is a "ledger row" — a ruled record of a deliberative step in
	 * the OODA cycle. We borrow Alt-Paper idioms (thin rules, uppercase labels,
	 * serif why-text, monospace metadata, 3px status stripe) but shape them
	 * around *process* rather than *publication* or *consultation*.
	 *
	 * Depth per ADR-000831 §12: Alt-Paper prohibits shadows, so render_depth_hint
	 * collapses to a subtle saturate+brightness filter plus a rule-weight bump,
	 * never elevation. Reduced-motion users get the weight bump alone.
	 */

	const stageLabel = $derived(
		(
			{
				observe: "Observe",
				orient: "Orient",
				decide: "Decide",
				act: "Act",
			} as const
		)[entry.proposedStage],
	);

	const priorityLabel = $derived(
		(
			{
				critical: "Critical",
				continuing: "Continuing",
				confirm: "Confirm",
				reference: "Reference",
			} as const
		)[entry.loopPriority],
	);

	const ariaDescription = $derived(`Priority: ${priorityLabel}`);

	// Relative time formatter for the freshness line, monospace column.
	const relFresh = $derived.by(() => {
		if (!entry.freshnessAt) return "—";
		const diff = Date.now() - new Date(entry.freshnessAt).getTime();
		if (diff < 60_000) return "just now";
		if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}m ago`;
		if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)}h ago`;
		return `${Math.floor(diff / 86_400_000)}d ago`;
	});
</script>

<article
	class="entry depth-{entry.renderDepthHint}"
	data-role="loop-entry"
	data-testid="loop-entry-tile"
	data-priority={entry.loopPriority}
	data-stage={entry.proposedStage}
	aria-label={ariaDescription}
	style="--stagger: {stagger}"
>
	<span class="entry-stripe" aria-hidden="true"></span>
	<div class="entry-body">
		<header class="entry-head">
			<span class="stage-label">{stageLabel}</span>
			<span class="priority-label">{priorityLabel}</span>
		</header>
		<p class="why-text">{entry.whyPrimary.text}</p>
		{#if entry.whyPrimary.evidenceRefs.length > 0}
			<section class="evidence">
				<h3 class="evidence-heading">Evidence</h3>
				<ol class="evidence-list">
					{#each entry.whyPrimary.evidenceRefs as ref (ref.refId)}
						<li class="evidence-item">
							<span class="evidence-id">{ref.refId}</span>
							{#if ref.label}
								<span class="evidence-sep">·</span>
								<span class="evidence-label">{ref.label}</span>
							{/if}
						</li>
					{/each}
				</ol>
			</section>
		{/if}
		<footer class="entry-foot">
			<span class="foot-cell">rev {entry.projectionRevision}</span>
			<span class="foot-cell foot-cell--freshness">{relFresh}</span>
			{#if entry.supersededByEntryKey}
				<span class="foot-cell foot-cell--super">Superseded</span>
			{/if}
		</footer>
	</div>
</article>

<style>
	.entry {
		display: grid;
		grid-template-columns: 3px 1fr;
		gap: 0;
		border: 1px solid var(--surface-border, #c8c8c8);
		background: var(--surface-bg, #faf9f7);
		animation: entry-in 0.3s ease forwards;
		animation-delay: calc(var(--stagger, 0) * 40ms);
		opacity: 0;
		transition:
			filter 180ms ease,
			border-color 180ms ease;
	}
	@keyframes entry-in {
		to {
			opacity: 1;
		}
	}

	/* Stripe — stage stamp. Color encoded from priority, but in Alt-Paper ink
	   tones only (no saturated badges). */
	.entry-stripe {
		background: var(--alt-slate, #666);
		align-self: stretch;
	}
	.entry[data-priority="critical"] .entry-stripe {
		background: var(--alt-terracotta, #b85450);
	}
	.entry[data-priority="continuing"] .entry-stripe {
		background: var(--alt-sand, #d4a574);
	}
	.entry[data-priority="confirm"] .entry-stripe {
		background: var(--alt-primary, #2f4f4f);
	}
	.entry[data-priority="reference"] .entry-stripe {
		background: var(--alt-ash, #999);
	}

	.entry-body {
		padding: 0.9rem 1.1rem 0.85rem;
	}

	.entry-head {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		gap: 0.75rem;
		margin-bottom: 0.55rem;
	}
	.stage-label,
	.priority-label {
		font-family: var(--font-body, "Source Sans 3", system-ui, sans-serif);
		font-size: 0.65rem;
		font-weight: 700;
		letter-spacing: 0.12em;
		text-transform: uppercase;
	}
	.stage-label {
		color: var(--alt-charcoal, #1a1a1a);
	}
	.priority-label {
		color: var(--alt-ash, #999);
	}
	.entry[data-priority="critical"] .priority-label {
		color: var(--alt-terracotta, #b85450);
	}

	.why-text {
		font-family: var(--font-body, "Source Sans 3", system-ui, sans-serif);
		font-size: 0.95rem;
		line-height: 1.65;
		color: var(--alt-charcoal, #1a1a1a);
		margin: 0;
		max-width: 65ch;
	}

	.evidence {
		margin-top: 0.7rem;
		padding-top: 0.55rem;
		border-top: 1px solid var(--surface-border, #c8c8c8);
	}
	.evidence-heading {
		font-family: var(--font-body, "Source Sans 3", system-ui, sans-serif);
		font-size: 0.6rem;
		font-weight: 700;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--alt-ash, #999);
		margin: 0 0 0.35rem;
	}
	.evidence-list {
		list-style: decimal;
		padding-left: 1.1rem;
		margin: 0;
	}
	.evidence-item {
		font-family: var(--font-body, "Source Sans 3", system-ui, sans-serif);
		font-size: 0.8rem;
		line-height: 1.5;
		color: var(--alt-slate, #666);
	}
	.evidence-id {
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.72rem;
		font-weight: 600;
		color: var(--alt-charcoal, #1a1a1a);
	}
	.evidence-sep {
		margin: 0 0.35rem;
		color: var(--alt-ash, #999);
	}
	.evidence-label {
		color: var(--alt-primary, #2f4f4f);
	}

	.entry-foot {
		margin-top: 0.7rem;
		padding-top: 0.5rem;
		border-top: 1px solid var(--surface-border, #c8c8c8);
		display: flex;
		gap: 1.2rem;
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.65rem;
		color: var(--alt-ash, #999);
	}
	.foot-cell--freshness {
		margin-left: auto;
	}
	.foot-cell--super {
		text-transform: uppercase;
		letter-spacing: 0.12em;
		color: var(--alt-terracotta, #b85450);
	}

	/* Depth hints (ADR-000831 §12). Alt-Paper forbids drop shadows, so depth
	   is expressed via saturate/brightness + rule weight. Reduced motion keeps
	   only the rule weight so the visual hierarchy remains. */
	.entry.depth-1 {
		filter: saturate(0.88) brightness(0.995);
	}
	.entry.depth-2 {
		filter: none;
	}
	.entry.depth-3 {
		filter: saturate(1.04);
		border-color: color-mix(in oklab, var(--alt-charcoal, #1a1a1a) 35%, var(--surface-border, #c8c8c8));
	}
	.entry.depth-4 {
		filter: saturate(1.06);
		border: 1.5px solid var(--alt-charcoal, #1a1a1a);
	}

	@media (prefers-reduced-motion: reduce) {
		.entry {
			animation: none;
			opacity: 1;
			transition: opacity 160ms ease;
		}
		.entry.depth-1,
		.entry.depth-2,
		.entry.depth-3,
		.entry.depth-4 {
			filter: none;
		}
	}
</style>
