<script lang="ts">
	import type { KnowledgeLoopEntryData } from "$lib/connect/knowledge_loop";

	let { entry }: { entry: KnowledgeLoopEntryData } = $props();

	const depthClass = $derived(`depth-${entry.renderDepthHint}`);

	/** Accessibility label for assistive tech. English tokens per user preference. */
	const priorityAriaLabel = $derived(
		({
			critical: "Priority: critical",
			continuing: "Priority: continuing",
			confirm: "Priority: confirm",
			reference: "Priority: reference",
		})[entry.loopPriority],
	);

	const stageLabel = $derived(
		({
			observe: "Observe",
			orient: "Orient",
			decide: "Decide",
			act: "Act",
		})[entry.proposedStage],
	);
</script>

<article
	class="loop-tile {depthClass}"
	data-testid="loop-entry-tile"
	data-priority={entry.loopPriority}
	data-stage={entry.proposedStage}
	aria-label={priorityAriaLabel}
>
	<header class="tile-head">
		<span class="stage-kicker">{stageLabel}</span>
		<span class="priority-label">{entry.loopPriority}</span>
	</header>
	<p class="why-text">{entry.whyPrimary.text}</p>
	{#if entry.whyPrimary.evidenceRefs.length > 0}
		<ul class="evidence">
			{#each entry.whyPrimary.evidenceRefs as ref (ref.refId)}
				<li>{ref.label || ref.refId}</li>
			{/each}
		</ul>
	{/if}
	<footer class="tile-foot">
		<span class="seq">rev {entry.projectionRevision}</span>
		{#if entry.supersededByEntryKey}
			<span class="superseded" aria-label="This entry has been superseded">superseded</span>
		{/if}
	</footer>
</article>

<style>
	.loop-tile {
		--tile-shadow: 0 2px 4px rgba(0, 0, 0, 0.04);
		--tile-shadow-strong: 0 4px 12px rgba(0, 0, 0, 0.1);
		--tile-saturation: 1;
		--tile-brightness: 1;
		--tile-z: 0;

		padding: var(--space-md, 1rem);
		border: 1px solid var(--border-muted, #ddd);
		background: var(--surface, #fff);
		box-shadow: var(--tile-shadow);
		transform: translateZ(var(--tile-z));
		filter: saturate(var(--tile-saturation)) brightness(var(--tile-brightness));
		transition:
			box-shadow 180ms ease,
			transform 180ms ease,
			filter 180ms ease;
	}

	/* Depth on tiles; text stays flat (Apple spatial design). */
	.loop-tile.depth-1 {
		--tile-z: 0px;
		--tile-saturation: 0.92;
		--tile-brightness: 0.98;
		--tile-shadow: none;
	}
	.loop-tile.depth-2 {
		--tile-z: 2px;
		--tile-saturation: 1;
		--tile-brightness: 1;
		--tile-shadow: 0 2px 6px rgba(0, 0, 0, 0.06);
	}
	.loop-tile.depth-3 {
		--tile-z: 6px;
		--tile-saturation: 1.05;
		--tile-brightness: 1.02;
		--tile-shadow: 0 6px 16px rgba(0, 0, 0, 0.08);
	}
	.loop-tile.depth-4 {
		--tile-z: 10px;
		--tile-saturation: 1.1;
		--tile-brightness: 1.05;
		--tile-shadow: 0 10px 26px rgba(0, 0, 0, 0.12);
	}

	.tile-head {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		font-family: var(--font-meta, "IBM Plex Mono", monospace);
		font-size: 0.75rem;
		text-transform: uppercase;
		letter-spacing: 0.1em;
		color: var(--fg-muted, #555);
		margin-bottom: var(--space-xs, 0.5rem);
	}

	.stage-kicker {
		font-weight: 600;
	}

	.why-text {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 1rem;
		line-height: 1.5;
		margin: 0.25rem 0;
	}

	.evidence {
		margin: 0.5rem 0;
		padding: 0 0 0 1rem;
		font-size: 0.875rem;
		color: var(--fg-muted, #555);
	}

	.tile-foot {
		display: flex;
		justify-content: space-between;
		margin-top: var(--space-sm, 0.75rem);
		font-family: var(--font-meta, "IBM Plex Mono", monospace);
		font-size: 0.7rem;
		color: var(--fg-muted, #777);
	}

	.superseded {
		text-transform: uppercase;
		letter-spacing: 0.1em;
		font-weight: 600;
	}

	/* Reduced Motion fallback: no Z transform, no saturate/brightness animation; rely on
	   opacity and highlight fade only (per canonical contract §12.5). */
	@media (prefers-reduced-motion: reduce) {
		.loop-tile {
			transform: none !important;
			transition: opacity 120ms ease;
		}
		.loop-tile.depth-1,
		.loop-tile.depth-2,
		.loop-tile.depth-3,
		.loop-tile.depth-4 {
			filter: none;
		}
	}
</style>
