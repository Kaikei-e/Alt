<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import { getReadFeedsWithCursorClient } from "$lib/api/client";
import { infiniteScroll } from "$lib/actions/infinite-scroll";
import type { RenderFeed, SanitizedFeed } from "$lib/schema/feed";
import { toRenderFeed } from "$lib/schema/feed";
import EmptyViewedFeedsState from "./EmptyViewedFeedsState.svelte";
import ViewedFeedCard from "./ViewedFeedCard.svelte";

const PAGE_SIZE = 20;

// State
let feeds = $state<SanitizedFeed[]>([]);
let cursor = $state<string | null>(null);
let hasMore = $state(true);
let isLoading = $state(false);
let isInitialLoading = $state(true);
let error = $state<Error | null>(null);
let liveRegionMessage = $state("");
let isRetrying = $state(false);

let scrollContainerRef: HTMLDivElement | null = $state(null);

// Use the scroll container as root for IntersectionObserver
const getScrollRoot = $derived(browser ? scrollContainerRef : null);

// Ensure we start at the top of the list on first render
onMount(() => {
	if (scrollContainerRef) {
		scrollContainerRef.scrollTop = 0;
	}
});

// Load initial feeds
const loadInitial = async () => {
	isInitialLoading = true;
	isLoading = true;
	error = null;

	try {
		const response = await getReadFeedsWithCursorClient(undefined, PAGE_SIZE);
		feeds = response.data;
		cursor = response.next_cursor;
		hasMore = response.next_cursor !== null;
	} catch (err) {
		if (err instanceof Error && err.message.includes("404")) {
			feeds = [];
			cursor = null;
			hasMore = false;
			error = null;
		} else {
			error = err instanceof Error ? err : new Error("Failed to load data");
			feeds = [];
			hasMore = false;
		}
	} finally {
		isLoading = false;
		isInitialLoading = false;
	}
};

// Load more feeds
const loadMore = async () => {
	if (isLoading) return;
	if (!hasMore) return;

	const currentCursor = cursor;
	isLoading = true;
	error = null;

	try {
		const response = await getReadFeedsWithCursorClient(
			currentCursor ?? undefined,
			PAGE_SIZE,
		);

		if (response.data.length === 0) {
			hasMore = response.next_cursor !== null;
			if (response.next_cursor) {
				cursor = response.next_cursor;
			} else {
				hasMore = false;
				cursor = null;
			}
		} else {
			// Add new feeds
			feeds = [...feeds, ...response.data];
			cursor = response.next_cursor;
			hasMore = response.next_cursor !== null;
		}
	} catch (err) {
		if (err instanceof Error && err.message.includes("404")) {
			hasMore = false;
			cursor = null;
			error = null;
		} else {
			error =
				err instanceof Error ? err : new Error("Failed to load more data");
		}
		console.error("[ViewedFeedsClient] loadMore error:", err);
	} finally {
		isLoading = false;
	}
};

// Refresh feeds
const refresh = async () => {
	cursor = null;
	hasMore = true;
	await loadInitial();
};

// Retry functionality
const retryFetch = async () => {
	isRetrying = true;
	try {
		await refresh();
		liveRegionMessage = "Read feeds refreshed successfully";
		setTimeout(() => {
			liveRegionMessage = "";
		}, 1000);
	} catch (err) {
		console.error("Retry failed:", err);
		throw err;
	} finally {
		isRetrying = false;
	}
};

// Start loading feeds after initial render
onMount(() => {
	if (hasMore && !isLoading && feeds.length === 0) {
		void loadInitial();
	}
});

// Convert feeds to RenderFeed
const renderFeeds = $derived.by(() => {
	return feeds.map((feed: SanitizedFeed) => toRenderFeed(feed));
});

const hasVisibleContent = $derived(feeds.length > 0);

const isInitialLoadingState = $derived(isInitialLoading && feeds.length === 0);
</script>

<div class="h-full flex flex-col" style="background: var(--app-bg);">
	<div
		aria-live="polite"
		aria-atomic="true"
		class="absolute left-[-10000px] w-px h-px overflow-hidden"
	>
		{liveRegionMessage}
	</div>

	<div
		bind:this={scrollContainerRef}
		class="px-5 py-5 max-w-2xl mx-auto overflow-y-auto overflow-x-clip flex-1 min-h-0"
		data-testid="read-feeds-scroll-container"
		style="background: var(--app-bg);"
	>
		{#if isInitialLoadingState && !hasVisibleContent}
			<!-- Skeleton loading state -->
			<div class="flex flex-col gap-4">
				{#each Array(5) as _}
					<div
						class="p-4 rounded-2xl border-2 border-border animate-pulse"
						style="background: var(--surface-bg);"
					>
						<div class="h-4 bg-muted rounded w-3/4 mb-2"></div>
						<div class="h-3 bg-muted rounded w-full mb-1"></div>
						<div class="h-3 bg-muted rounded w-5/6"></div>
					</div>
				{/each}
			</div>
		{:else if error}
			<!-- Error state -->
			<div class="flex flex-col items-center justify-center min-h-[50vh] p-6">
				<div
					class="p-6 rounded-lg border text-center"
					style="background: var(--surface-bg); border-color: var(--destructive);"
				>
					<p class="text-destructive font-semibold mb-2">Error loading feeds</p>
					<p class="text-sm text-muted-foreground mb-4">{error.message}</p>
					<button
						onclick={() => void retryFetch()}
						disabled={isRetrying}
						class="px-4 py-2 rounded bg-primary text-primary-foreground disabled:opacity-50"
					>
						{isRetrying ? "Retrying..." : "Retry"}
					</button>
				</div>
			</div>
		{:else if renderFeeds.length > 0}
			<!-- Feed list rendering -->
			<div
				class="flex flex-col gap-4"
				data-testid="virtual-feed-list"
				style="content-visibility: auto; contain-intrinsic-size: 800px;"
			>
				{#each renderFeeds as feed (feed.link)}
					<ViewedFeedCard {feed} />
				{/each}
			</div>

			<!-- No more feeds indicator -->
			{#if !hasMore && renderFeeds.length > 0}
				<p
					class="text-center text-sm mt-8 mb-4"
					style="color: var(--alt-text-secondary);"
				>
					No more history to load
				</p>
			{/if}

			<!-- Loading indicator -->
			{#if isLoading}
				<div
					class="py-4 text-center text-sm"
					style="color: var(--alt-text-secondary);"
				>
					Loading more...
				</div>
			{/if}

			<!-- Infinite scroll sentinel -->
			{#if hasMore}
				<div
					use:infiniteScroll={{
						callback: loadMore,
						root: getScrollRoot,
						disabled: isLoading || !getScrollRoot,
						rootMargin: "0px 0px 200px 0px",
						threshold: 0.1,
					}}
					aria-hidden="true"
					style="height: 10px; min-height: 10px; width: 100%;"
					data-testid="infinite-scroll-sentinel"
				></div>
			{/if}
		{:else}
			<!-- Empty state -->
			<EmptyViewedFeedsState />
		{/if}
	</div>
</div>

