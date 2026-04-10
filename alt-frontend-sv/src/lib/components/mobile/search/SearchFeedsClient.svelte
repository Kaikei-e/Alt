<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import type { SearchFeedItem, SearchQuery } from "$lib/schema/search";
import FloatingMenu from "../feeds/swipe/FloatingMenu.svelte";
import SearchResults from "./SearchResults.svelte";
import SearchWindow from "./SearchWindow.svelte";

interface Props {
	initialQuery?: string;
}
const { initialQuery = "" }: Props = $props();

// svelte-ignore state_referenced_locally
let searchQuery = $state<SearchQuery>({ query: initialQuery });
let results = $state<SearchFeedItem[]>([]);
let isLoading = $state(false);
let searchTime = $state<number | undefined>(undefined);
let cursor = $state<string | null>(null);
let hasMore = $state(false);

onMount(() => {
	if (browser && window.scrollTo) {
		window.scrollTo(0, 0);
	}
});
</script>

<div class="archive-mobile-page" data-role="archive-desk-mobile">
	<div class="flex flex-col gap-6 max-w-[600px] mx-auto p-4">
		<div class="archive-header-mobile">
			<h1 class="archive-title-mobile">Search Feeds</h1>
			<p class="archive-subtitle-mobile">
				Search across your RSS feeds
			</p>
		</div>

		<div class="archive-search-container">
			<SearchWindow
				{searchQuery}
				autoSearch={!!initialQuery.trim()}
				setSearchQuery={(query) => {
					searchQuery = query;
				}}
				setFeedResults={(newResults) => {
					results = newResults;
				}}
				setCursor={(newCursor) => {
					cursor = newCursor;
				}}
				setHasMore={(newHasMore) => {
					hasMore = newHasMore;
				}}
				{isLoading}
				setIsLoading={(loading) => {
					isLoading = loading;
				}}
				setSearchTime={(time) => {
					searchTime = time;
				}}
			/>
		</div>

		<SearchResults
			{results}
			{isLoading}
			searchQuery={searchQuery.query || ""}
			{searchTime}
			{cursor}
			{hasMore}
			setResults={(newResults) => {
				results = newResults;
			}}
			setCursor={(newCursor) => {
				cursor = newCursor;
			}}
			setHasMore={(newHasMore) => {
				hasMore = newHasMore;
			}}
			setIsLoading={(loading) => {
				isLoading = loading;
			}}
		/>

		{#if !searchQuery.query && !isLoading && results.length === 0}
			<div class="archive-tip">
				<p class="archive-tip-text">
					Try searching for topics like "AI", "technology", or "news"
				</p>
			</div>
		{/if}
	</div>

	<FloatingMenu />
</div>

<style>
	.archive-mobile-page {
		min-height: 100dvh;
		background: var(--surface-bg);
		color: var(--alt-charcoal);
	}

	.archive-header-mobile {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		margin-top: 0.5rem;
		text-align: center;
	}

	.archive-title-mobile {
		font-family: var(--font-display);
		font-size: 1.3rem;
		font-weight: 700;
		color: var(--alt-charcoal);
		margin: 0;
	}

	.archive-subtitle-mobile {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-ash);
		margin: 0;
	}

	.archive-search-container {
		border: 1px solid var(--surface-border);
		padding: 0.75rem;
		background: var(--surface-bg);
	}

	.archive-tip {
		border: 1px solid var(--surface-border);
		padding: 0.75rem;
		background: var(--surface-bg);
	}

	.archive-tip-text {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-ash);
		font-style: italic;
		text-align: center;
		margin: 0;
	}
</style>
