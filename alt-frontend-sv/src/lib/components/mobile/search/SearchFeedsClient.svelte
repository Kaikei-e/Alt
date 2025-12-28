<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import type { SearchFeedItem, SearchQuery } from "$lib/schema/search";
import FloatingMenu from "../feeds/swipe/FloatingMenu.svelte";
import SearchResults from "./SearchResults.svelte";
import SearchWindow from "./SearchWindow.svelte";

let searchQuery = $state<SearchQuery>({ query: "" });
let results = $state<SearchFeedItem[]>([]);
let isLoading = $state(false);
let searchTime = $state<number | undefined>(undefined);
let cursor = $state<string | null>(null);
let hasMore = $state(false);

// Ensure we start at the top on mount
onMount(() => {
	if (browser && window.scrollTo) {
		window.scrollTo(0, 0);
	}
});
</script>

<div
	class="min-h-screen p-4"
	style="background: var(--app-bg); color: var(--foreground);"
>
	<div class="flex flex-col gap-6 max-w-[600px] mx-auto">
		<!-- Header Section -->
		<div class="flex flex-col gap-3 mt-2 mb-2">
			<h1
				class="text-2xl font-bold text-center tracking-tight"
				style="color: var(--text-primary);"
			>
				Search Feeds
			</h1>
			<p
				class="text-center text-base max-w-[400px] mx-auto leading-relaxed"
				style="color: var(--text-secondary);"
			>
				Discover content across your RSS feeds with intelligent search
			</p>
		</div>

		<!-- Search Input Section -->
		<div
			class="glass p-2 rounded-[24px]"
			style="
				background: var(--alt-glass);
				border: 1px solid var(--alt-glass-border);
				box-shadow: var(--alt-glass-shadow);
			"
		>
			<SearchWindow
				{searchQuery}
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

		<!-- Search Results Section -->
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

		<!-- Quick Tips -->
		{#if !searchQuery.query && !isLoading && results.length === 0}
			<div
				class="glass p-4 rounded-[24px]"
				style="
					background: var(--alt-glass);
					border: 1px solid var(--alt-glass-border);
					box-shadow: var(--alt-glass-shadow);
				"
			>
				<p
					class="text-sm text-center leading-relaxed"
					style="color: var(--text-secondary);"
				>
					💡 Try searching for topics like &quot;AI&quot;,
					&quot;technology&quot;, or &quot;news&quot;
				</p>
			</div>
		{/if}
	</div>

	<FloatingMenu />
</div>

