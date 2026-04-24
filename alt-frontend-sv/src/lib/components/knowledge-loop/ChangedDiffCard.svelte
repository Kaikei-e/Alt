<script lang="ts">
/**
 * ChangedDiffCard — the mid-context plane's signature block. Renders a
 * single Changed entry as an editorial diptych: THEN on the left shows
 * the supersede target (what the entry was), NOW on the right shows the
 * current why_text / evidence_refs. A 1px vertical rule separates them —
 * this is the only place on /loop where a vertical rule carries meaning,
 * intentionally mirroring a newspaper's "then-and-now" layout.
 *
 * The Changed plane is the feature most uniquely Alt's: surfacing
 * revisions to the user's mental model. ADR-000831 §6.3 treats it as a
 * first-class bucket, not a badge on the NOW plane.
 */

import type { KnowledgeLoopEntryData } from "$lib/connect/knowledge_loop";
import { loopPriorityAriaLabel } from "./loop-priority-labels";

let {
	entries,
	onConfirm,
}: {
	entries: KnowledgeLoopEntryData[];
	onConfirm?: (entry: KnowledgeLoopEntryData) => void;
} = $props();

function confirm(entry: KnowledgeLoopEntryData) {
	onConfirm?.(entry);
}

function thenLabel(entry: KnowledgeLoopEntryData): string {
	// Prefer change_summary.summary when the projector populated it.
	// Fall back to the supersede pointer so the card is never empty.
	if (entry.changeSummary?.summary) return entry.changeSummary.summary;
	if (entry.supersededByEntryKey)
		return `Previous: ${entry.supersededByEntryKey}`;
	if (entry.changeSummary?.previousEntryKey)
		return `Previous: ${entry.changeSummary.previousEntryKey}`;
	return "Previous version";
}
</script>

<div class="diff-cards" data-testid="loop-changed-diff">
	{#each entries as entry (entry.entryKey)}
		<article class="card" aria-label={loopPriorityAriaLabel[entry.loopPriority]}>
			<div class="col col-then">
				<span class="kicker" aria-hidden="true">Then</span>
				<p class="line">{thenLabel(entry)}</p>
			</div>
			<div class="rule" aria-hidden="true"></div>
			<div class="col col-now">
				<span class="kicker" aria-hidden="true">Now</span>
				<p class="line line-now">{entry.whyPrimary.text || entry.entryKey}</p>
			</div>
			<div class="actions">
				<button
					type="button"
					class="confirm"
					onclick={() => confirm(entry)}
					data-testid="loop-changed-confirm"
					data-entry-key={entry.entryKey}
				>
					Confirm
				</button>
			</div>
		</article>
	{/each}
</div>

<style>
	.diff-cards {
		display: grid;
		gap: 0.85rem;
	}
	.card {
		display: grid;
		grid-template-columns: 1fr 1px 1fr;
		grid-template-rows: auto auto;
		gap: 0.6rem 1rem;
		padding: 0.85rem 0;
		border-top: 1px solid var(--surface-border, #c8c8c8);
		border-bottom: 1px solid var(--surface-border, #c8c8c8);
	}
	.col {
		display: grid;
		gap: 0.3rem;
	}
	.rule {
		background: var(--surface-border, #c8c8c8);
		grid-row: 1;
	}
	.kicker {
		font-family: var(--font-body, "Source Sans 3", system-ui, sans-serif);
		font-size: 0.6rem;
		font-weight: 700;
		letter-spacing: 0.16em;
		text-transform: uppercase;
		color: var(--alt-ash, #999);
	}
	.line {
		margin: 0;
		font-family: var(--font-display, "Playfair Display", Georgia, serif);
		font-size: 0.95rem;
		line-height: 1.45;
		color: var(--alt-slate, #666);
	}
	.line-now {
		color: var(--alt-charcoal, #1a1a1a);
	}
	.actions {
		grid-column: 1 / -1;
		display: flex;
		justify-content: flex-end;
	}
	.confirm {
		appearance: none;
		background: transparent;
		border: 1px solid var(--alt-charcoal, #1a1a1a);
		padding: 0.3rem 0.85rem;
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.68rem;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-charcoal, #1a1a1a);
		cursor: pointer;
	}
	.confirm:hover {
		background: var(--alt-charcoal, #1a1a1a);
		color: #fff;
	}
	.confirm:focus-visible {
		outline: 2px solid var(--alt-terracotta, #b85450);
		outline-offset: 2px;
	}
</style>
