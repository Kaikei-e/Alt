<script lang="ts">
import type { FootprintData } from "$lib/connect/knowledge_trail";
import { groupFootprintsByDay } from "./trail-grouping";
import Footprint from "./Footprint.svelte";

interface Props {
	footprints: FootprintData[];
	loading: boolean;
	hasMore: boolean;
	hasEverLoaded: boolean;
	onLoadMore: () => void;
}

const { footprints, loading, hasMore, hasEverLoaded, onLoadMore }: Props =
	$props();

// `now` is read once at construction; grouping is pure over it.
const now = new Date();
const groups = $derived(groupFootprintsByDay(footprints, now));
const isEmpty = $derived(hasEverLoaded && !loading && footprints.length === 0);
</script>

<!-- No lens chip bar: the raw tag union was a dead tag cloud (D25). Theme
     narrowing belongs to episodes; targeted rediscovery to trail search. -->
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
