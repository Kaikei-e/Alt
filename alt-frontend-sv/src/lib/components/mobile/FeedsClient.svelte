<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import {
	getFeedsWithCursorClient,
	getReadFeedsWithCursorClient,
	updateFeedReadStatusClient,
} from "$lib/api/client";
import { infiniteScroll } from "$lib/actions/infinite-scroll";
import type { RenderFeed, SanitizedFeed } from "$lib/schema/feed";
import { toRenderFeed } from "$lib/schema/feed";
import { canonicalize } from "$lib/utils/feed";
import EmptyFeedState from "./EmptyFeedState.svelte";
import FeedCard from "./FeedCard.svelte";

interface Props {
	initialFeeds?: RenderFeed[];
}

const { initialFeeds = [] }: Props = $props();

const PAGE_SIZE = 20;

// State
let feeds = $state<SanitizedFeed[]>([]);
let cursor = $state<string | null>(null);
let hasMore = $state(true);
let isLoading = $state(false);
let isInitialLoading = $state(true);
let error = $state<Error | null>(null);
let readFeeds = $state<Set<string>>(new Set());
let liveRegionMessage = $state("");
let isRetrying = $state(false);

let scrollContainerRef: HTMLDivElement | null = $state(null);

// Use the scroll container as root for IntersectionObserver
// This ensures the observer correctly detects when sentinel enters the scrollable area
// Use $derived() instead of $derived.by() to ensure reference stability
const getScrollRoot = $derived(browser ? scrollContainerRef : null);

// Initialize readFeeds set from backend on mount
onMount(() => {
	if (!browser) return;

	const initializeReadFeeds = async () => {
		try {
			const readFeedsResponse = await getReadFeedsWithCursorClient(
				undefined,
				32,
			);
			const readFeedLinks = new Set<string>();
			if (readFeedsResponse?.data) {
				readFeedsResponse.data.forEach((feed: SanitizedFeed) => {
					const canonical = canonicalize(feed.link);
					readFeedLinks.add(canonical);
				});
			}
			readFeeds = readFeedLinks;
		} catch (err) {
			// Log error but don't crash the app - read feeds initialization is optional
			const errorMessage = err instanceof Error ? err.message : String(err);
			console.error("Failed to initialize read feeds:", {
				error: errorMessage,
				message:
					"This is non-critical - feeds will still load, but read status may not be accurate",
			});
			// Set empty set to prevent further errors
			readFeeds = new Set();
		}
	};

	// Use requestIdleCallback to defer initialization
	if ("requestIdleCallback" in window) {
		const idleCallbackId = window.requestIdleCallback(
			() => {
				void initializeReadFeeds();
			},
			{ timeout: 2000 },
		);
		return () => {
			window.cancelIdleCallback(idleCallbackId);
		};
	} else {
		const timeoutId = setTimeout(() => {
			void initializeReadFeeds();
		}, 100);
		return () => clearTimeout(timeoutId);
	}
});

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
		const response = await getFeedsWithCursorClient(undefined, PAGE_SIZE);

		// If initialFeeds exist, filter out duplicates
		if (initialFeeds.length > 0) {
			const initialFeedUrls = new Set(
				initialFeeds.map((feed) => feed.normalizedUrl),
			);
			feeds = response.data.filter((feed: SanitizedFeed) => {
				const renderFeed = toRenderFeed(feed);
				return !initialFeedUrls.has(renderFeed.normalizedUrl);
			});
		} else {
			feeds = response.data;
		}

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
		const response = await getFeedsWithCursorClient(
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
		console.error("[FeedsClient] loadMore error:", err);
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

// Handle marking feed as read with optimistic update
const handleMarkAsRead = async (rawLink: string) => {
	const link =
		rawLink.includes("?") || rawLink.includes("#")
			? canonicalize(rawLink)
			: rawLink;

	// Optimistic update
	readFeeds = new Set(readFeeds).add(link);
	liveRegionMessage = "Feed marked as read";
	setTimeout(() => {
		liveRegionMessage = "";
	}, 1000);

	// Server update (rollback on failure)
	try {
		await updateFeedReadStatusClient(link);
	} catch (e) {
		readFeeds = new Set(readFeeds);
		readFeeds.delete(link);
		console.error("Failed to mark feed as read:", e);
	}
};

// Merge initialFeeds with fetched feeds and filter/memoize visible feeds
const renderFeeds = $derived.by(() => {
	// Start with initialFeeds (already RenderFeed[])
	const allFeeds: RenderFeed[] = [...initialFeeds];

	// Add fetched feeds (convert SanitizedFeed to RenderFeed)
	if (feeds.length > 0) {
		const fetchedRenderFeeds: RenderFeed[] = feeds.map((feed: SanitizedFeed) =>
			toRenderFeed(feed),
		);
		allFeeds.push(...fetchedRenderFeeds);
	}

	// Filter out read feeds using normalizedUrl
	return allFeeds.filter((feed) => !readFeeds.has(feed.normalizedUrl));
});

const hasVisibleContent = $derived(initialFeeds.length > 0 || feeds.length > 0);

const isInitialLoadingState = $derived(
	isInitialLoading && initialFeeds.length === 0 && feeds.length === 0,
);
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
		data-testid="feeds-scroll-container"
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
					<FeedCard
						{feed}
						isReadStatus={readFeeds.has(feed.normalizedUrl)}
						setIsReadStatus={(feedLink: string) => handleMarkAsRead(feedLink)}
					/>
				{/each}
			</div>

			<!-- No more feeds indicator -->
			{#if !hasMore && renderFeeds.length > 0}
				<p
					class="text-center text-sm mt-8 mb-4"
					style="color: var(--alt-text-secondary);"
				>
					No more feeds to load
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
			<EmptyFeedState />
		{/if}
	</div>
</div>
