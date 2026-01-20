<script lang="ts">
import { Loader } from "@lucide/svelte";
import { browser } from "$app/environment";
import { infiniteScroll } from "$lib/actions/infinite-scroll";
import { searchFeedsClient } from "$lib/api/client";
import type { SearchFeedItem } from "$lib/schema/search";
import { transformFeedSearchResult } from "$lib/utils/transformFeedSearchResult";
import SearchResultItem from "./SearchResultItem.svelte";

interface Props {
	results: SearchFeedItem[];
	isLoading: boolean;
	searchQuery: string;
	searchTime?: number;
	cursor: string | null;
	hasMore: boolean;
	setResults: (results: SearchFeedItem[]) => void;
	setCursor: (cursor: string | null) => void;
	setHasMore: (hasMore: boolean) => void;
	setIsLoading: (loading: boolean) => void;
}

const {
	results,
	isLoading,
	searchQuery,
	searchTime,
	cursor,
	hasMore,
	setResults,
	setCursor,
	setHasMore,
	setIsLoading,
}: Props = $props();

// Use viewport as root for IntersectionObserver (no container reference needed)
// This allows infinite scroll to work with page-level scrolling
const getScrollRoot = $derived(browser ? null : null);

// Load more results for infinite scroll
const loadMore = async () => {
	if (isLoading) return;
	if (!hasMore) return;

	const currentCursor = cursor;
	setIsLoading(true);

	try {
		// Convert cursor string to number (offset)
		// If cursor is null, pass undefined (same pattern as ViewedFeedsClient)
		const cursorOffset = currentCursor
			? parseInt(currentCursor, 10)
			: undefined;
		if (cursorOffset !== undefined && isNaN(cursorOffset)) {
			console.error("Invalid cursor value:", currentCursor);
			setIsLoading(false);
			return;
		}

		const searchResult = await searchFeedsClient(searchQuery, cursorOffset, 20);

		if (searchResult.error) {
			console.error("Error loading more results:", searchResult.error);
			setIsLoading(false);
			return;
		}

		const newResults = transformFeedSearchResult(searchResult);

		if (newResults.length === 0) {
			// No new results, check if there's a next cursor
			setHasMore(searchResult.next_cursor !== null);
			if (
				searchResult.next_cursor !== null &&
				searchResult.next_cursor !== undefined
			) {
				setCursor(String(searchResult.next_cursor));
			} else {
				setHasMore(false);
				setCursor(null);
			}
		} else {
			// Add new results
			setResults([...results, ...newResults]);
			// Convert cursor number to string for state management
			setCursor(
				searchResult.next_cursor !== null &&
					searchResult.next_cursor !== undefined
					? String(searchResult.next_cursor)
					: null,
			);
			setHasMore(searchResult.next_cursor !== null);
		}
	} catch (error) {
		console.error("Error loading more results:", error);
	} finally {
		setIsLoading(false);
	}
};
</script>

{#if !searchQuery.trim()}
	<!-- No query state - don't show anything -->
{:else if isLoading && results.length === 0}
	<!-- Loading State (only for initial search) -->
	<div
		class="glass p-8 text-center rounded-[24px]"
		style="
			background: var(--surface-bg);
			border: 2px solid var(--surface-border);
			box-shadow: var(--shadow-sm);
		"
	>
		<div class="flex flex-col gap-4 items-center">
			<Loader class="h-8 w-8 animate-spin" style="color: var(--alt-primary);" />
			<p style="color: var(--text-secondary);">Searching feeds...</p>
		</div>
	</div>
{:else if results.length === 0}
	<!-- Empty State -->
	<div
		class="glass p-8 text-center rounded-[24px]"
		style="
			background: var(--alt-glass);
			border: 1px solid var(--alt-glass-border);
			box-shadow: var(--alt-glass-shadow);
		"
	>
		<div class="flex flex-col gap-3">
			<p class="text-2xl">🔍</p>
			<p class="font-medium" style="color: var(--text-secondary);">
				No results found
			</p>
			{#if searchQuery}
				<p class="text-sm" style="color: var(--text-muted);">
					No feeds match &quot;{searchQuery}&quot;. Try different keywords.
				</p>
			{/if}
		</div>
	</div>
{:else}
	<!-- Results List -->
	<div
		class="flex flex-col gap-4"
	>
		<!-- Search Stats -->
		<div class="flex justify-between items-center mb-4">
			<h2
				class="text-lg font-bold"
				style="color: var(--alt-primary);"
			>
				Search Results ({results.length})
			</h2>
			{#if searchTime}
				<p class="text-sm" style="color: var(--text-muted);">
					Found in {searchTime}ms
				</p>
			{/if}
		</div>

		<!-- Results -->
		<ul class="flex flex-col gap-6" role="list" aria-label="Search results">
			{#each results as result (result.link || result.title)}
				<li>
					<SearchResultItem {result} />
				</li>
			{/each}
		</ul>

		<!-- Loading more indicator -->
		{#if isLoading}
			<div class="py-4 text-center text-sm" style="color: var(--text-secondary);">
				<div class="flex items-center justify-center gap-2">
					<Loader class="h-4 w-4 animate-spin" style="color: var(--alt-primary);" />
					<span>Loading more...</span>
				</div>
			</div>
		{/if}

		<!-- No more results indicator -->
		{#if !hasMore && results.length > 0}
			<p
				class="text-center text-sm mt-8 mb-4"
				style="color: var(--text-secondary);"
			>
				No more results to load
			</p>
		{/if}

		<!-- Infinite scroll sentinel -->
		{#if hasMore}
			<div
				use:infiniteScroll={{
					callback: loadMore,
					root: getScrollRoot,
					disabled: isLoading,
					rootMargin: "0px 0px 200px 0px",
					threshold: 0.1,
				}}
				aria-hidden="true"
				style="height: 10px; min-height: 10px; width: 100%;"
				data-testid="infinite-scroll-sentinel"
			></div>
		{/if}
	</div>
{/if}

