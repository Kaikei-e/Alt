<script lang="ts">
/**
 * ContinueStream — the mid-context plane for entries the user has already
 * engaged with but not completed. Laid out as an editorial timeline:
 * each entry is a short row with a monospace freshness stamp, a serif
 * title carrying the why_text, and a reference badge for accessibility.
 *
 * Design choice (ADR-000831 §6.2 + Alt-Paper shared language): this is not
 * a copy of the foreground tile — it is a lower-density reading surface,
 * because the user already has context on each item. Depth is implied by
 * saturation (LoopSurfacePlane plane="mid-context") and by a reduced font
 * scale, not by translateZ.
 */

import type { KnowledgeLoopEntryData } from "$lib/connect/knowledge_loop";
import {
	loopPriorityAriaLabel,
	loopPriorityLabel,
} from "./loop-priority-labels";

let {
	entries,
	onResume,
}: {
	entries: KnowledgeLoopEntryData[];
	onResume?: (entry: KnowledgeLoopEntryData) => void;
} = $props();

function resume(entry: KnowledgeLoopEntryData) {
	onResume?.(entry);
}

function formatFreshness(iso: string): string {
	if (!iso) return "—";
	const ms = Date.parse(iso);
	if (Number.isNaN(ms)) return "—";
	const delta = Date.now() - ms;
	const min = 60 * 1000;
	const hour = 60 * min;
	const day = 24 * hour;
	if (delta < hour) return `${Math.max(1, Math.floor(delta / min))}m`;
	if (delta < day) return `${Math.floor(delta / hour)}h`;
	return `${Math.floor(delta / day)}d`;
}
</script>

<ol class="continue-stream" data-testid="loop-continue-stream">
	{#each entries as entry (entry.entryKey)}
		<li class="row" aria-label={loopPriorityAriaLabel[entry.loopPriority]}>
			<span class="stamp" aria-hidden="true">{formatFreshness(entry.freshnessAt)}</span>
			<button
				type="button"
				class="title"
				onclick={() => resume(entry)}
				data-testid="loop-continue-resume"
				data-entry-key={entry.entryKey}
			>
				{entry.whyPrimary.text || entry.entryKey}
			</button>
			<span class="badge">{loopPriorityLabel[entry.loopPriority]}</span>
		</li>
	{/each}
</ol>

<style>
	.continue-stream {
		list-style: none;
		padding: 0;
		margin: 0;
		display: grid;
		gap: 0.55rem;
	}
	.row {
		display: grid;
		grid-template-columns: 3ch 1fr auto;
		align-items: baseline;
		gap: 0.85rem;
		padding: 0.55rem 0;
		border-bottom: 1px solid var(--surface-border, #c8c8c8);
	}
	.row:last-child {
		border-bottom: none;
	}
	.stamp {
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.68rem;
		color: var(--alt-ash, #999);
		letter-spacing: 0.04em;
	}
	.title {
		appearance: none;
		background: transparent;
		border: none;
		padding: 0;
		margin: 0;
		text-align: left;
		cursor: pointer;
		font-family: var(--font-display, "Playfair Display", Georgia, serif);
		font-size: 0.95rem;
		line-height: 1.45;
		color: var(--alt-charcoal, #1a1a1a);
	}
	.title:hover {
		text-decoration: underline;
		text-decoration-thickness: 1px;
		text-underline-offset: 3px;
	}
	.title:focus-visible {
		outline: 2px solid var(--alt-charcoal, #1a1a1a);
		outline-offset: 2px;
	}
	.badge {
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.6rem;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--alt-slate, #666);
	}

	@media (prefers-reduced-motion: reduce) {
		.title {
			transition: none;
		}
	}
</style>
