<script lang="ts">
/**
 * ReviewDock — deep-focus plane surfaced as a low-density discovery list.
 * Each row is mono-typed and narrow; the intent is to make Review feel
 * different from Continue (which is timeline-paced) and Changed (which
 * demands confirmation). Review items linger, waiting to be noticed.
 *
 * Why mono for Review: the editorial metaphor is a subscription box
 * margin — terse, indexed, scannable. It deliberately reads as
 * peripheral so the user doesn't confuse it with the primary NOW plane
 * (ADR-000831 §12 predictability contract).
 */

import type { KnowledgeLoopEntryData } from "$lib/connect/knowledge_loop";
import { loopPriorityAriaLabel } from "./loop-priority-labels";

let {
	entries,
	onOpen,
}: {
	entries: KnowledgeLoopEntryData[];
	onOpen?: (entry: KnowledgeLoopEntryData) => void;
} = $props();

function open(entry: KnowledgeLoopEntryData) {
	onOpen?.(entry);
}
</script>

<ul class="review-dock" data-testid="loop-review-dock">
	{#each entries as entry (entry.entryKey)}
		<li class="row" aria-label={loopPriorityAriaLabel[entry.loopPriority]}>
			<button
				type="button"
				class="entry"
				onclick={() => open(entry)}
				data-testid="loop-review-open"
				data-entry-key={entry.entryKey}
			>
				<span class="dot" aria-hidden="true">·</span>
				<span class="text">{entry.whyPrimary.text || entry.entryKey}</span>
				<span class="why-kind">{entry.whyPrimary.kind.replace(/_why$/, "")}</span>
			</button>
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
		appearance: none;
		background: transparent;
		border: none;
		width: 100%;
		padding: 0.4rem 0;
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
	.entry:hover .text {
		text-decoration: underline;
		text-decoration-thickness: 1px;
		text-underline-offset: 3px;
		color: var(--alt-charcoal, #1a1a1a);
	}
	.entry:focus-visible {
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
</style>
