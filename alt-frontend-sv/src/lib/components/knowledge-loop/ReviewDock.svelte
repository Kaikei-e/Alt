<script lang="ts">
/**
 * ReviewDock — deep-focus plane surfaced as a low-density discovery list.
 * Each row is mono-typed and narrow; the intent is to make Review feel
 * different from Continue (which is timeline-paced) and Changed (which
 * demands confirmation). Review items linger, waiting to be noticed.
 *
 * fb.md §F (Review re-evaluation engine) elevates Review from a leftover
 * bucket to a deliberate re-evaluation queue. Each row exposes three
 * actions:
 *
 *   Open       — same as before; opens the source URL via onOpen.
 *   Recheck    — re-surfaces the entry as NOW with fresh freshness_at.
 *   Archive    — permanently dismisses (dismiss_state = completed).
 *   Mark Reviewed — acknowledges without re-surfacing unless new evidence
 *                   arrives.
 *
 * The action labels are functional words (Alt-Paper: metaphor lives in the
 * visual layer; CTA text stays operational).
 *
 * Why mono for Review: the editorial metaphor is a subscription box
 * margin — terse, indexed, scannable. It deliberately reads as
 * peripheral so the user doesn't confuse it with the primary NOW plane
 * (ADR-000831 §12 predictability contract).
 */

import type { KnowledgeLoopEntryData } from "$lib/connect/knowledge_loop";
import type { ReviewAction } from "$lib/hooks/useKnowledgeLoop.svelte";
import { loopPriorityAriaLabel } from "./loop-priority-labels";

let {
	entries,
	onOpen,
	onReviewAction,
}: {
	entries: KnowledgeLoopEntryData[];
	onOpen?: (entry: KnowledgeLoopEntryData) => void;
	onReviewAction?: (
		entry: KnowledgeLoopEntryData,
		action: ReviewAction,
	) => void;
} = $props();

function open(entry: KnowledgeLoopEntryData) {
	onOpen?.(entry);
}

function act(entry: KnowledgeLoopEntryData, action: ReviewAction) {
	onReviewAction?.(entry, action);
}
</script>

<ul class="review-dock" data-testid="loop-review-dock">
	{#each entries as entry (entry.entryKey)}
		<li class="row" aria-label={loopPriorityAriaLabel[entry.loopPriority]}>
			<div class="entry">
				<button
					type="button"
					class="open"
					onclick={() => open(entry)}
					data-testid="loop-review-open"
					data-entry-key={entry.entryKey}
				>
					<span class="dot" aria-hidden="true">·</span>
					<span class="text">{entry.whyPrimary.text || entry.entryKey}</span>
					<span class="why-kind">{entry.whyPrimary.kind.replace(/_why$/, "")}</span>
				</button>
				{#if onReviewAction}
					<div class="actions" aria-label="Review actions">
						<button
							type="button"
							class="action"
							data-testid="loop-review-recheck"
							onclick={() => act(entry, "recheck")}
						>
							Recheck
						</button>
						<button
							type="button"
							class="action"
							data-testid="loop-review-mark-reviewed"
							onclick={() => act(entry, "mark_reviewed")}
						>
							Mark Reviewed
						</button>
						<button
							type="button"
							class="action action--archive"
							data-testid="loop-review-archive"
							onclick={() => act(entry, "archive")}
						>
							Archive
						</button>
					</div>
				{/if}
			</div>
		</li>
	{/each}
</ul>

<style>
	.review-dock {
		list-style: none;
		padding: 0;
		margin: 0;
		display: grid;
		gap: 0.25rem;
	}
	.row {
		border-top: 1px dotted var(--surface-border, #c8c8c8);
	}
	.row:last-child {
		border-bottom: 1px dotted var(--surface-border, #c8c8c8);
	}
	.entry {
		display: grid;
		grid-template-columns: 1fr;
		gap: 0.25rem;
		padding: 0.4rem 0;
	}
	.open {
		appearance: none;
		background: transparent;
		border: none;
		width: 100%;
		padding: 0;
		display: grid;
		grid-template-columns: 1ch 1fr auto;
		align-items: baseline;
		gap: 0.7rem;
		cursor: pointer;
		text-align: left;
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.78rem;
		line-height: 1.5;
		color: var(--alt-slate, #666);
	}
	.open:hover .text {
		text-decoration: underline;
		text-decoration-thickness: 1px;
		text-underline-offset: 3px;
		color: var(--alt-charcoal, #1a1a1a);
	}
	.open:focus-visible {
		outline: 2px solid var(--alt-charcoal, #1a1a1a);
		outline-offset: 2px;
	}
	.dot {
		color: var(--alt-ash, #999);
	}
	.text {
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.why-kind {
		font-size: 0.62rem;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--alt-ash, #999);
	}

	/* Review actions row — sits below the open link; functional buttons in
	 * Alt-Paper monospace. Archive gets a slight terracotta accent so it
	 * reads as the destructive option. */
	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		padding-left: 1.7ch; /* align with .open's text column */
	}
	.action {
		appearance: none;
		background: transparent;
		border: 1px solid var(--surface-border, #c8c8c8);
		padding: 0.18rem 0.55rem;
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.66rem;
		letter-spacing: 0.06em;
		color: var(--alt-charcoal, #1a1a1a);
		cursor: pointer;
		border-radius: 0;
	}
	.action:hover {
		background: var(--surface-2, #f5f4f1);
	}
	.action:focus-visible {
		outline: 2px solid var(--alt-charcoal, #1a1a1a);
		outline-offset: 2px;
	}
	.action--archive {
		border-color: var(--alt-terracotta, #b85450);
		color: var(--alt-terracotta, #b85450);
	}
</style>
