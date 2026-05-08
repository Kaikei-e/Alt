<script lang="ts">
/**
 * ChangedDiffCard — the mid-context plane's signature block. Renders a
 * single Changed entry as a redline-proof diptych. When the projector has
 * populated phrase / tag diff arrays the card draws editorial proof marks:
 * removed phrases get a strike-through in alt-terracotta; added phrases get
 * a bold underline; tags appear as chips with the same proof discipline.
 * When the new arrays are absent the card falls back to a Then / Now
 * single-line summary so the contract is fully additive.
 *
 * Vertical rules carry meaning here — this is the only place on /loop
 * where a 1px rule is intentional, mirroring a newspaper's then-and-now
 * layout. ADR-000831 §6.3 treats Changed as a first-class bucket.
 */

import type {
	ChangeSummaryData,
	KnowledgeLoopEntryData,
} from "$lib/connect/knowledge_loop";
import { loopPriorityAriaLabel } from "./loop-priority-labels";

let {
	entries,
	onCompare,
}: {
	entries: KnowledgeLoopEntryData[];
	/**
	 * Phase 3: Changed CTA emits acted_intent=compare, target_type=diff (per
	 * canonical contract §11 + transition-metadata::pickTargetForIntent).
	 * The parent typically forwards into useKnowledgeLoop's Act mutation,
	 * which wires up `buildTransitionMetadata(entry, {intent: "compare"})`
	 * and routes the diff drawer behavior. Replaced the v8-era `onConfirm`
	 * since "Confirm" was a placeholder — the semantic intent of Changed is
	 * comparison.
	 */
	onCompare?: (entry: KnowledgeLoopEntryData) => void;
} = $props();

function compare(entry: KnowledgeLoopEntryData) {
	onCompare?.(entry);
}

function hasRedline(
	cs: ChangeSummaryData | undefined,
): cs is ChangeSummaryData {
	if (!cs) return false;
	return Boolean(
		(cs.addedPhrases && cs.addedPhrases.length > 0) ||
			(cs.removedPhrases && cs.removedPhrases.length > 0) ||
			(cs.addedTags && cs.addedTags.length > 0) ||
			(cs.removedTags && cs.removedTags.length > 0),
	);
}

function thenLabel(entry: KnowledgeLoopEntryData): string {
	if (entry.changeSummary?.summary) return entry.changeSummary.summary;
	if (entry.supersededByEntryKey)
		return `Previous: ${entry.supersededByEntryKey}`;
	if (entry.changeSummary?.previousEntryKey)
		return `Previous: ${entry.changeSummary.previousEntryKey}`;
	return "Previous version";
}

function ariaSummary(entry: KnowledgeLoopEntryData): string {
	const cs = entry.changeSummary;
	const priority = loopPriorityAriaLabel[entry.loopPriority];
	if (!hasRedline(cs)) return priority;
	const phraseAdds = cs.addedPhrases?.length ?? 0;
	const phraseRemoves = cs.removedPhrases?.length ?? 0;
	const tagAdds = cs.addedTags?.length ?? 0;
	const tagRemoves = cs.removedTags?.length ?? 0;
	const parts: string[] = [];
	if (phraseAdds > 0)
		parts.push(
			`${phraseAdds} ${phraseAdds === 1 ? "phrase" : "phrases"} added`,
		);
	if (phraseRemoves > 0)
		parts.push(
			`${phraseRemoves} ${phraseRemoves === 1 ? "phrase" : "phrases"} removed`,
		);
	if (tagAdds > 0)
		parts.push(`${tagAdds} ${tagAdds === 1 ? "tag" : "tags"} added`);
	if (tagRemoves > 0)
		parts.push(`${tagRemoves} ${tagRemoves === 1 ? "tag" : "tags"} removed`);
	return parts.length > 0
		? `${priority}. Summary changed: ${parts.join(", ")}.`
		: priority;
}
</script>

<div class="diff-cards" data-testid="loop-changed-diff">
	{#each entries as entry (entry.entryKey)}
		{@const cs = entry.changeSummary}
		<article class="card" aria-label={ariaSummary(entry)}>
			{#if hasRedline(cs)}
				<div class="redline" data-testid="loop-changed-redline">
					{#if cs.removedPhrases && cs.removedPhrases.length > 0}
						<div class="band band-removed">
							<span class="kicker" aria-hidden="true">Struck</span>
							<ul class="phrase-list">
								{#each cs.removedPhrases as phrase, i (i)}
									<li class="phrase phrase-removed">{phrase}</li>
								{/each}
							</ul>
						</div>
					{/if}
					{#if cs.addedPhrases && cs.addedPhrases.length > 0}
						<div class="band band-added">
							<span class="kicker" aria-hidden="true">Set</span>
							<ul class="phrase-list">
								{#each cs.addedPhrases as phrase, i (i)}
									<li class="phrase phrase-added">{phrase}</li>
								{/each}
							</ul>
						</div>
					{/if}
					{#if (cs.removedTags && cs.removedTags.length > 0) || (cs.addedTags && cs.addedTags.length > 0)}
						<div class="band band-tags">
							<span class="kicker" aria-hidden="true">Tags</span>
							<ul class="chip-list">
								{#each cs.removedTags ?? [] as tag, i (`r${i}`)}
									<li class="chip chip-removed">{tag}</li>
								{/each}
								{#each cs.addedTags ?? [] as tag, i (`a${i}`)}
									<li class="chip chip-added">{tag}</li>
								{/each}
							</ul>
						</div>
					{/if}
				</div>
			{:else}
				<div class="col col-then">
					<span class="kicker" aria-hidden="true">Then</span>
					<p class="line">{thenLabel(entry)}</p>
				</div>
				<div class="rule" aria-hidden="true"></div>
				<div class="col col-now">
					<span class="kicker" aria-hidden="true">Now</span>
					<p class="line line-now">
						{entry.whyPrimary.text || entry.entryKey}
					</p>
				</div>
			{/if}
			{#if cs?.summary}
				<div class="update-hint" data-testid="loop-changed-update-hint">
					<span class="kicker" aria-hidden="true">Updated</span>
					<p class="hint-line">{cs.summary}</p>
				</div>
			{/if}
			<div class="actions">
				<button
					type="button"
					class="confirm"
					onclick={() => compare(entry)}
					data-testid="loop-changed-compare"
					data-entry-key={entry.entryKey}
				>
					Compare
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
	.redline {
		grid-column: 1 / -1;
		grid-row: 1;
		display: grid;
		gap: 0.55rem;
	}
	.band {
		display: grid;
		grid-template-columns: 4.5rem 1fr;
		align-items: baseline;
		gap: 0.75rem;
	}
	/* Explicit cell placement so the diptych never collapses onto the same
	 * column. Without these, the production rendering was placing all three
	 * children at column 1 row 1 — the THEN/NOW kickers overlapped and the
	 * narrative text superimposed (2026-04-27 production /loop screenshot).
	 * `grid-row: 1` on each keeps `.actions` (grid-column: 1 / -1) safely in
	 * row 2 even when auto-placement would otherwise pack into row 1. */
	.col-then {
		grid-column: 1;
		grid-row: 1;
		display: grid;
		gap: 0.3rem;
		min-width: 0;
	}
	.col-now {
		grid-column: 3;
		grid-row: 1;
		display: grid;
		gap: 0.3rem;
		min-width: 0;
	}
	.rule {
		grid-column: 2;
		grid-row: 1;
		background: var(--surface-border, #c8c8c8);
	}
	.kicker {
		font-family: var(--font-body, "Source Sans 3", system-ui, sans-serif);
		font-size: 0.6rem;
		font-weight: 700;
		letter-spacing: 0.16em;
		text-transform: uppercase;
		color: var(--alt-ash, #999);
	}
	.phrase-list {
		list-style: none;
		padding: 0;
		margin: 0;
		display: grid;
		gap: 0.2rem;
	}
	.phrase {
		font-family: var(--font-display, "Playfair Display", Georgia, serif);
		font-size: 0.95rem;
		line-height: 1.45;
	}
	.phrase-removed {
		color: var(--alt-terracotta, #6e1f1f);
		text-decoration: line-through;
		text-decoration-thickness: 1.5px;
	}
	.phrase-added {
		color: var(--alt-charcoal, #1a1a1a);
		font-weight: 600;
		text-decoration: underline;
		text-decoration-thickness: 1.5px;
		text-underline-offset: 3px;
	}
	.chip-list {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem;
	}
	.chip {
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.65rem;
		letter-spacing: 0.06em;
		padding: 0.15rem 0.45rem;
		border: 1px solid currentColor;
	}
	.chip-removed {
		color: var(--alt-ash, #999);
		text-decoration: line-through;
	}
	.chip-added {
		color: var(--alt-charcoal, #1a1a1a);
		background: rgba(0, 0, 0, 0.04);
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
	/* "What to update" band sits between the redline / Then-Now diptych and
	 * the Compare CTA. Renders only when change_summary.summary is populated
	 * (Phase 3 backend appends a deterministic update-hint clause to it). */
	.update-hint {
		grid-column: 1 / -1;
		display: grid;
		grid-template-columns: 4.5rem 1fr;
		align-items: baseline;
		gap: 0.75rem;
	}
	.hint-line {
		margin: 0;
		font-family: var(--font-body, "Source Sans 3", system-ui, sans-serif);
		font-size: 0.78rem;
		line-height: 1.5;
		color: var(--alt-slate, #666);
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
