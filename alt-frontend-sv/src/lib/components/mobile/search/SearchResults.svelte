<script lang="ts">
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

const getScrollRoot = $derived(browser ? null : null);

// Track initial load so infinite scroll additions render instantly
let initialLoadDone = $state(false);
let prevResultsLen = $state(0);

$effect(() => {
	const len = results.length;
	// Initial search result arrived — animate, then mark done
	if (len > 0 && prevResultsLen === 0) {
		initialLoadDone = false;
		setTimeout(() => {
			initialLoadDone = true;
		}, 600);
	}
	// Reset on new search (results cleared)
	if (len === 0) {
		initialLoadDone = false;
	}
	prevResultsLen = len;
});

const loadMore = async () => {
	if (isLoading) return;
	if (!hasMore) return;

	const currentCursor = cursor;
	setIsLoading(true);

	try {
		const cursorOffset = currentCursor
			? parseInt(currentCursor, 10)
			: undefined;
		if (cursorOffset !== undefined && Number.isNaN(cursorOffset)) {
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
			setResults([...results, ...newResults]);
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
	<!-- No query state -->
{:else if isLoading && results.length === 0}
	<div class="archive-loading-container">
		<div class="archive-loading">
			<span class="loading-pulse"></span>
			<span class="archive-loading-text">Searching feeds...</span>
		</div>
	</div>
{:else if results.length === 0}
	<div class="archive-empty">
		<p class="archive-empty-label">No results found</p>
		{#if searchQuery}
			<p class="archive-empty-hint">
				No feeds match "{searchQuery}". Try different keywords.
			</p>
		{/if}
	</div>
{:else}
	<div class="flex flex-col gap-4">
		<div class="archive-stats-row">
			<h2 class="archive-stats">
				Search Results ({results.length})
			</h2>
			{#if searchTime}
				<span class="archive-stats-time">Found in {searchTime}ms</span>
			{/if}
		</div>

		<ul class="flex flex-col gap-4" role="list" aria-label="Search results">
			{#each results as result, i (result.article_id || `${result.link}-${i}`)}
				<li class={initialLoadDone ? "" : "stagger-entry"} style={initialLoadDone ? "" : `--stagger: ${i}`}>
					<SearchResultItem {result} />
				</li>
			{/each}
		</ul>

		{#if isLoading}
			<div class="archive-loading">
				<span class="loading-pulse"></span>
				<span class="archive-loading-text">Loading more...</span>
			</div>
		{/if}

		{#if !hasMore && results.length > 0}
			<p class="archive-end">No more results to load</p>
		{/if}

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
				style="height: 50px; min-height: 50px; width: 100%;"
				data-testid="infinite-scroll-sentinel"
			></div>
		{/if}

		{#if hasMore && !isLoading}
			<div class="flex justify-center">
				<button
					type="button"
					onclick={loadMore}
					class="archive-btn-mobile"
				>
					LOAD MORE RESULTS
				</button>
			</div>
		{/if}
	</div>
{/if}

<style>
	.archive-stats-row {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}

	.archive-stats {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		font-weight: 700;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-ash);
		margin: 0;
	}

	.archive-stats-time {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-ash);
		letter-spacing: 0.04em;
	}

	.archive-loading-container {
		border: 1px solid var(--surface-border);
		padding: 2rem;
		background: var(--surface-bg);
	}

	.archive-loading {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 0.5rem;
		padding: 0.75rem 0;
	}

	.archive-loading-text {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-ash);
		font-style: italic;
	}

	.archive-empty {
		border: 1px solid var(--surface-border);
		padding: 2rem;
		text-align: center;
		background: var(--surface-bg);
	}

	.archive-empty-label {
		font-family: var(--font-body);
		font-size: 0.9rem;
		color: var(--alt-ash);
		font-style: italic;
		margin: 0;
	}

	.archive-empty-hint {
		font-family: var(--font-body);
		font-size: 0.8rem;
		color: var(--alt-ash);
		margin: 0.3rem 0 0;
	}

	.archive-btn-mobile {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 0.4rem;
		width: 100%;
		min-height: 44px;
		padding: 0.5rem 1rem;
		font-family: var(--font-body);
		font-size: 0.75rem;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--alt-charcoal);
		background: transparent;
		border: 1.5px solid var(--alt-charcoal);
		cursor: pointer;
		transition: background 0.15s, color 0.15s;
	}

	.archive-btn-mobile:hover {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	.archive-end {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-ash);
		text-align: center;
		padding: 1rem 0;
		margin: 0;
	}

	.stagger-entry {
		opacity: 0;
		animation: reveal 0.3s ease forwards;
		animation-delay: calc(var(--stagger) * 40ms);
	}

	@keyframes reveal {
		to {
			opacity: 1;
		}
	}

	.loading-pulse {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: currentColor;
		animation: pulse 1.2s ease-in-out infinite;
	}

	@keyframes pulse {
		0%,
		100% {
			opacity: 0.3;
		}
		50% {
			opacity: 1;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.loading-pulse {
			animation: none;
			opacity: 1;
		}

		.stagger-entry {
			animation: none;
			opacity: 1;
		}
	}
</style>
