<script lang="ts">
import type { EpisodeData } from "$lib/connect/knowledge_trail";
import EpisodeCard from "./EpisodeCard.svelte";

interface Props {
	episodes: EpisodeData[];
	loading: boolean;
	hasMore: boolean;
	hasEverLoaded: boolean;
	onLoadMore: () => void;
}

const { episodes, loading, hasMore, hasEverLoaded, onLoadMore }: Props =
	$props();

const isEmpty = $derived(hasEverLoaded && !loading && episodes.length === 0);
</script>

<!-- No lens chip bar: the raw tag union was a dead tag cloud (D25). Theme
     narrowing belongs to episodes; targeted rediscovery to trail search.
     Episodes are the spine's default display unit (D24) — date is a landmark
     on each episode header, never a grouping axis; there is no day-separator. -->
<section class="trail" data-testid="trail-spine">
	{#if isEmpty}
		<p class="trail-empty" data-testid="trail-empty">
			No footprints yet. As you read, ask, and return, your trail appears here.
		</p>
	{/if}

	{#each episodes as episode (episode.episodeKey)}
		<EpisodeCard {episode} />
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
