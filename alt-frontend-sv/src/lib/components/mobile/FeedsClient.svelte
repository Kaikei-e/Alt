<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import {
	getFeedsWithCursorClient,
	getReadFeedsWithCursorClient,
	updateFeedReadStatusClient,
} from "$lib/api/client";
import type { RenderFeed, SanitizedFeed } from "$lib/schema/feed";
import { toRenderFeed } from "$lib/schema/feed";
import { canonicalize } from "$lib/utils/feed";
import EmptyFeedState from "./EmptyFeedState.svelte";
import VirtualFeedList from "./VirtualFeedList.svelte";

interface Props {
	initialFeeds?: RenderFeed[];
}

const { initialFeeds = [] }: Props = $props();

const PAGE_SIZE = 20;
const INITIAL_VISIBLE_CARDS = 3;
const STEP = 5;

// State
let feeds = $state<SanitizedFeed[]>([]);
let cursor = $state<string | null>(null);
let hasMore = $state(true);
let isLoading = $state(false);
let isInitialLoading = $state(false);
let error = $state<Error | null>(null);
let readFeeds = $state<Set<string>>(new Set());
let visibleCount = $state(INITIAL_VISIBLE_CARDS);
let liveRegionMessage = $state("");
let isRetrying = $state(false);

let scrollContainerRef: HTMLDivElement | null = $state(null);
let sentinelRef: HTMLDivElement | null = $state(null);

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
				message: "This is non-critical - feeds will still load, but read status may not be accurate",
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
	if (isLoading || !hasMore || !cursor) {
		return;
	}

	isLoading = true;
	error = null;

	try {
		const response = await getFeedsWithCursorClient(cursor, PAGE_SIZE);
		feeds = [...feeds, ...response.data];
		cursor = response.next_cursor;
		hasMore = response.next_cursor !== null;
	} catch (err) {
		if (err instanceof Error && err.message.includes("404")) {
			hasMore = false;
			cursor = null;
			error = null;
		} else {
			error =
				err instanceof Error ? err : new Error("Failed to load more data");
		}
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

// Initialize isInitialLoading based on initialFeeds
onMount(() => {
	isInitialLoading = initialFeeds.length === 0;
});

// Start loading feeds after initial render
onMount(() => {
	if (hasMore && !isLoading && feeds.length === 0) {
		const shouldDefer = initialFeeds.length > 0;

		if (shouldDefer && "requestIdleCallback" in window) {
			const idleCallbackId = window.requestIdleCallback(
				() => {
					void loadInitial();
				},
				{ timeout: 2000 },
			);
			return () => {
				window.cancelIdleCallback(idleCallbackId);
			};
		} else {
			const timeoutId = setTimeout(
				() => {
					void loadInitial();
				},
				shouldDefer ? 500 : 100,
			);
			return () => clearTimeout(timeoutId);
		}
	}
});

// Progressive rendering: increase visibleCount when user scrolls near the end
onMount(() => {
	if (!browser) return;

	const observer = new IntersectionObserver(
		(entries) => {
			entries.forEach((entry) => {
				if (entry.isIntersecting) {
					const allFeedsCount = initialFeeds.length + feeds.length;
					const nextCount = Math.min(visibleCount + STEP, allFeedsCount);

					// If we've shown all initial feeds and need more, trigger API load
					if (nextCount >= initialFeeds.length && hasMore && !isLoading) {
						void loadMore();
					}

					visibleCount = nextCount;
				}
			});
		},
		{
			rootMargin: "200px 0px",
			threshold: 0.1,
		},
	);

	if (sentinelRef) {
		observer.observe(sentinelRef);
	}

	return () => {
		observer.disconnect();
	};
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
const visibleFeeds = $derived.by(() => {
	// Start with initialFeeds (already RenderFeed[])
	const allFeeds: RenderFeed[] = [...initialFeeds];

	// Add fetched feeds (convert SanitizedFeed to RenderFeed)
	if (feeds.length > 0) {
		const renderFeeds: RenderFeed[] = feeds.map((feed: SanitizedFeed) =>
			toRenderFeed(feed),
		);
		allFeeds.push(...renderFeeds);
	}

	// Filter out read feeds using normalizedUrl
	const filtered = allFeeds.filter(
		(feed) => !readFeeds.has(feed.normalizedUrl),
	);

	// Limit to visibleCount items for progressive rendering
	return filtered.slice(0, visibleCount);
});

const hasVisibleContent = $derived(initialFeeds.length > 0 || feeds.length > 0);

const isInitialLoadingState = $derived(
	isInitialLoading && initialFeeds.length === 0 && feeds.length === 0,
);
</script>

<div
	class="min-h-screen relative flex flex-col"
	style="background: var(--app-bg);"
>
	<div
		aria-live="polite"
		aria-atomic="true"
		class="absolute left-[-10000px] w-px h-px overflow-hidden"
	>
		{liveRegionMessage}
	</div>

	<div
		bind:this={scrollContainerRef}
		class="p-5 max-w-2xl mx-auto overflow-y-auto overflow-x-hidden flex-1"
		data-testid="feeds-scroll-container"
		style="background: var(--app-bg);"
	>
		{#if isInitialLoadingState && !hasVisibleContent}
			<!-- Skeleton loading state -->
			<div class="flex flex-col gap-4">
				{#each Array(INITIAL_VISIBLE_CARDS) as _}
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
		{:else if visibleFeeds.length > 0}
			<!-- Feed list rendering -->
			<VirtualFeedList
				feeds={visibleFeeds}
				{readFeeds}
				onMarkAsRead={handleMarkAsRead}
			/>

			<!-- No more feeds indicator -->
			{#if !hasMore && visibleFeeds.length > 0}
				<p
					class="text-center text-sm mt-8 mb-4"
					style="color: var(--alt-text-secondary);"
				>
					No more feeds to load
				</p>
			{/if}

			<!-- Infinite scroll sentinel -->
			{#if visibleFeeds.length > 0 && hasMore}
				<div
					bind:this={sentinelRef}
					class="h-[50px] w-full bg-transparent my-[10px] relative z-[1] flex items-center justify-center flex-shrink-0"
					data-testid="infinite-scroll-sentinel"
				>
					{#if isLoading}
						<p
							class="text-sm"
							style="color: var(--alt-text-secondary);"
						>
							Loading more...
						</p>
					{/if}
				</div>
			{/if}
		{:else}
			<!-- Empty state -->
			<EmptyFeedState />
		{/if}
	</div>
</div>

