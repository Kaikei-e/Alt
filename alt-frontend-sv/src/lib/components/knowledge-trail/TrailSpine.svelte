<script lang="ts">
import type { FootprintData } from "$lib/connect/knowledge_trail";
import { groupFootprintsByDay } from "./trail-grouping";
import Footprint from "./Footprint.svelte";

interface Props {
	footprints: FootprintData[];
	loading: boolean;
	hasMore: boolean;
	hasEverLoaded: boolean;
	activeTags: string[];
	availableTags: string[];
	onLoadMore: () => void;
	onSelectLens: (tags: string[]) => void;
}

const {
	footprints,
	loading,
	hasMore,
	hasEverLoaded,
	activeTags,
	availableTags,
	onLoadMore,
	onSelectLens,
}: Props = $props();

// `now` is read once at construction; grouping is pure over it.
const now = new Date();
const groups = $derived(groupFootprintsByDay(footprints, now));
const isEmpty = $derived(hasEverLoaded && !loading && footprints.length === 0);
const lensActive = $derived(activeTags.length > 0);
</script>

{#if availableTags.length > 0}
	<div class="lenses" data-testid="trail-lenses">
		<span class="lens-label">Lens</span>
		<button
			class="lens"
			class:active={!lensActive}
			onclick={() => onSelectLens([])}
		>
			All footprints
		</button>
		{#each availableTags as tag (tag)}
			<button
				class="lens"
				class:active={activeTags.includes(tag)}
				onclick={() => onSelectLens([tag])}
			>
				{tag}
			</button>
		{/each}
	</div>
{/if}

<section class="trail" data-testid="trail-spine">
	{#if isEmpty}
		<p class="trail-empty" data-testid="trail-empty">
			No footprints yet. As you read, ask, and return, your trail appears here.
		</p>
	{/if}

	{#each groups as group (group.dayKey)}
		<div class="day-sep">
			<span class="day">{group.label}</span>
			<span class="rule"></span>
			<span class="count">{group.footprints.length} footprints</span>
		</div>
		{#each group.footprints as fp (fp.footprintKey)}
			<Footprint footprint={fp} />
		{/each}
	{/each}

	{#if hasMore}
		<button class="load-more" onclick={onLoadMore} disabled={loading}>
			{loading ? "Loading…" : "Load earlier footprints"}
		</button>
	{/if}
</section>

<style>
	.lenses {
		display: flex;
		align-items: center;
		gap: 0.45rem;
		margin-top: 1.3rem;
		flex-wrap: wrap;
	}
	.lens-label {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--alt-ash, #999);
		margin-right: 0.3rem;
	}
	.lens {
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--chip-text, #49443d);
		border: 1px solid var(--chip-border, #d0c8bb);
		background: var(--chip-bg, #ebe7df);
		padding: 0.28rem 0.7rem;
		cursor: pointer;
	}
	.lens.active {
		border-color: var(--alt-primary, #2f4f4f);
		color: var(--alt-primary, #2f4f4f);
		background: color-mix(in srgb, var(--alt-primary, #2f4f4f) 8%, transparent);
		font-weight: 600;
	}
	.trail {
		margin-top: 1.4rem;
		max-width: 880px;
	}
	.trail-empty {
		font-family: var(--font-body);
		font-size: 0.9rem;
		color: var(--alt-ash, #999);
		font-style: italic;
	}
	.day-sep {
		display: flex;
		align-items: center;
		gap: 0.8rem;
		margin: 1.5rem 0 0.8rem;
	}
	.day-sep:first-child {
		margin-top: 0.4rem;
	}
	.day {
		font-family: var(--font-display);
		font-size: 1rem;
		font-weight: 700;
		color: var(--alt-charcoal, #1a1a1a);
	}
	.rule {
		flex: 1;
		height: 1px;
		background: var(--surface-border, #c8c8c8);
	}
	.count {
		font-family: var(--font-mono);
		font-size: 0.66rem;
		color: var(--alt-ash, #999);
		letter-spacing: 0.06em;
		text-transform: uppercase;
	}
	.load-more {
		margin-top: 0.8rem;
		border: 1px solid var(--chip-border, #d0c8bb);
		background: var(--action-surface, #ebe8e1);
		color: var(--interactive-text, #2f4f4f);
		font-family: var(--font-body);
		font-size: 0.85rem;
		padding: 0.5rem 0.9rem;
		cursor: pointer;
	}
	.load-more:disabled {
		opacity: 0.5;
		cursor: default;
	}
</style>
