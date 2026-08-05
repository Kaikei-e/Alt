<script lang="ts">
import { getContext, onMount } from "svelte";
import { browser } from "$app/environment";
import { infiniteScroll } from "$lib/actions/infinite-scroll";
import {
	getFeedsWithCursorClient,
	getReadFeedsWithCursorClient,
	updateFeedReadStatusClient,
} from "$lib/api/client";
import Toast from "$lib/components/knowledge-home/Toast.svelte";
import { MARK_AS_READ_FAILED_MESSAGE } from "$lib/feeds/mark-as-read-feedback";
import type { RenderFeed, SanitizedFeed } from "$lib/schema/feed";
import { toRenderFeed } from "$lib/schema/feed";
import {
	CONNECTION_RECOVERY_KEY,
	type ConnectionRecoveryStore,
} from "$lib/stores/connection-recovery.svelte";
import { useToastStore } from "$lib/stores/toast.svelte";
import { canonicalize } from "$lib/utils/feed";
import EmptyFeedState from "./EmptyFeedState.svelte";
import FeedCard from "./FeedCard.svelte";

interface Props {
	initialFeeds?: RenderFeed[];
	excludeFeedLinkIds?: string[];
}

const { initialFeeds = [], excludeFeedLinkIds = [] }: Props = $props();
const toast = useToastStore();
const connectionRecovery = getContext<ConnectionRecoveryStore | undefined>(
	CONNECTION_RECOVERY_KEY,
);

const PAGE_SIZE = 20;

/**
 * Connect-RPC serialises an absent cursor as the empty string — proto3 has no
 * nullable `string` — so a last page arrives as `{ next_cursor: "", has_more:
 * false }` and the old `next_cursor !== null` check stayed true forever. The
 * infinite-scroll sentinel then re-requested page one indefinitely. Prefer the
 * explicit `has_more` flag; fall back to cursor truthiness when it is absent.
 */
function hasMorePages(response: {
	next_cursor: string | null;
	has_more?: boolean;
}): boolean {
	return response.has_more ?? Boolean(response.next_cursor);
}

/** Normalise the empty-string cursor to `null` so it is never sent back. */
function nextCursorOf(response: { next_cursor: string | null }): string | null {
	return response.next_cursor ? response.next_cursor : null;
}

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
let useInitialFeeds = $state(true);

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
		const response = await getFeedsWithCursorClient(
			undefined,
			PAGE_SIZE,
			excludeFeedLinkIds.length > 0 ? excludeFeedLinkIds : undefined,
		);

		// If initialFeeds exist and still in use, filter out duplicates
		if (useInitialFeeds && initialFeeds.length > 0) {
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

		cursor = nextCursorOf(response);
		hasMore = hasMorePages(response);
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
			excludeFeedLinkIds.length > 0 ? excludeFeedLinkIds : undefined,
		);

		if (response.data.length === 0) {
			cursor = nextCursorOf(response);
			hasMore = cursor !== null && hasMorePages(response);
		} else {
			// Add new feeds
			feeds = [...feeds, ...response.data];
			cursor = nextCursorOf(response);
			hasMore = hasMorePages(response);
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

// Track previous excludeFeedLinkIds to detect changes
let prevExcludeKey = $state<string | undefined>(undefined);

// React to excludeFeedLinkIds changes: reset and reload feeds
$effect(() => {
	const currentKey = excludeFeedLinkIds.join(",");
	if (prevExcludeKey === undefined) {
		// First run - just record the initial value
		prevExcludeKey = currentKey;
		return;
	}
	if (prevExcludeKey !== currentKey) {
		prevExcludeKey = currentKey;
		feeds = [];
		cursor = null;
		hasMore = true;
		useInitialFeeds = false;
		void loadInitial();
	}
});

// Start loading feeds after initial render
onMount(() => {
	if (hasMore && !isLoading && feeds.length === 0) {
		void loadInitial();
	}
});

// Safari connection recovery: refresh feeds when tab returns from extended background
$effect(() => {
	if (!connectionRecovery) return;
	const unsubscribe = connectionRecovery.subscribe((info) => {
		console.info("[FeedsClient] Connection recovery triggered:", info.reason);
		void refresh();
	});
	return unsubscribe;
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
	const clearSuccessLiveRegion = setTimeout(() => {
		liveRegionMessage = "";
	}, 1000);

	// Server update (rollback on failure)
	try {
		await updateFeedReadStatusClient(link);
	} catch (e) {
		clearTimeout(clearSuccessLiveRegion);
		readFeeds = new Set(readFeeds);
		readFeeds.delete(link);
		console.error("Failed to mark feed as read:", e);
		liveRegionMessage = MARK_AS_READ_FAILED_MESSAGE;
		toast.push(MARK_AS_READ_FAILED_MESSAGE, "error", 4000);
		setTimeout(() => {
			liveRegionMessage = "";
		}, 4000);
	}
};

// Merge initialFeeds with fetched feeds and filter/memoize visible feeds
const renderFeeds = $derived.by(() => {
	// Start with initialFeeds only if filter hasn't changed
	const allFeeds: RenderFeed[] = useInitialFeeds ? [...initialFeeds] : [];

	// Add fetched feeds (convert SanitizedFeed to RenderFeed)
	if (feeds.length > 0) {
		const fetchedRenderFeeds: RenderFeed[] = feeds.map((feed: SanitizedFeed) =>
			toRenderFeed(feed),
		);
		allFeeds.push(...fetchedRenderFeeds);
	}

	// Filter out read feeds, and collapse repeats.
	//
	// The each block below is keyed by `feed.link`, and a keyed block cannot
	// hold two siblings under the same key — a duplicate is a hard runtime
	// error, not a warning (https://svelte.dev/e/each_key_duplicate). Only
	// `loadInitial` deduped against `initialFeeds`; `loadMore` appends blind, so
	// the guard belongs on the merge rather than at each append site. Same
	// reason `appendUniqueById` exists on /feeds/search.
	const seenLinks = new Set<string>();
	return allFeeds.filter((feed) => {
		if (readFeeds.has(feed.normalizedUrl)) return false;
		if (seenLinks.has(feed.link)) return false;
		seenLinks.add(feed.link);
		return true;
	});
});

const hasVisibleContent = $derived(initialFeeds.length > 0 || feeds.length > 0);

const isInitialLoadingState = $derived(
	isInitialLoading && initialFeeds.length === 0 && feeds.length === 0,
);
</script>

<div class="h-full flex flex-col" style="background: var(--app-bg);">
	<Toast items={toast.items} onDismiss={toast.remove} />
	<div
		aria-live="assertive"
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
			<div class="flex flex-col">
				{#each Array(5) as _, i}
					<div
						class="skeleton-entry"
						style="animation-delay: {i * 80}ms;"
					>
						<div class="skeleton-line skeleton-line--title animate-shimmer-warm"></div>
						<div class="skeleton-line skeleton-line--full animate-shimmer-warm"></div>
						<div class="skeleton-line skeleton-line--short animate-shimmer-warm"></div>
					</div>
				{/each}
			</div>
		{:else if error}
			<div class="error-state">
				<p class="error-title">Error loading feeds</p>
				<p class="error-message">{error.message}</p>
				<button
					onclick={() => void retryFetch()}
					disabled={isRetrying}
					class="retry-btn"
				>
					{isRetrying ? "Retrying\u2026" : "Retry"}
				</button>
			</div>
		{:else if renderFeeds.length > 0}
			<div
				class="flex flex-col"
				data-testid="virtual-feed-list"
				style="content-visibility: auto; contain-intrinsic-size: 800px;"
			>
				{#each renderFeeds as feed, i (feed.link)}
					<div class="feed-entry" style="--stagger: {i};">
						<FeedCard
							{feed}
							isReadStatus={readFeeds.has(feed.normalizedUrl)}
							setIsReadStatus={(feedLink: string) => handleMarkAsRead(feedLink)}
						/>
					</div>
				{/each}
			</div>

			{#if !hasMore && renderFeeds.length > 0}
				<p class="end-hint">End of wire</p>
			{/if}

			{#if isLoading}
				<div class="loading-more">
					<span class="loading-pulse"></span>
					<span class="loading-text">Loading more dispatches&hellip;</span>
				</div>
			{/if}

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
			<EmptyFeedState />
		{/if}
	</div>
</div>

<style>
	.skeleton-entry {
		padding: 0.75rem 0;
		border-bottom: 1px solid var(--surface-border);
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
	}

	.skeleton-line {
		height: 0.75rem;
	}

	.skeleton-line--title {
		width: 75%;
		height: 1rem;
	}

	.skeleton-line--full {
		width: 100%;
	}

	.skeleton-line--short {
		width: 60%;
	}

	.error-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		min-height: 50dvh;
		padding: 1.5rem;
		text-align: center;
	}

	.error-title {
		font-family: var(--font-body);
		font-size: 0.9rem;
		font-weight: 600;
		color: var(--alt-terracotta);
		margin: 0 0 0.4rem;
	}

	.error-message {
		font-family: var(--font-body);
		font-size: 0.82rem;
		color: var(--alt-slate);
		margin: 0 0 1rem;
	}

	.retry-btn {
		font-family: var(--font-body);
		font-size: 0.75rem;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--alt-charcoal);
		background: transparent;
		border: 1.5px solid var(--alt-charcoal);
		padding: 0.4rem 1rem;
		min-height: 44px;
		cursor: pointer;
	}

	.retry-btn:disabled {
		opacity: 0.4;
	}

	.feed-entry {
		opacity: 0;
		animation: entry-in 0.3s ease forwards;
		animation-delay: calc(var(--stagger) * 40ms);
	}

	.end-hint {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-ash);
		text-align: center;
		margin: 2rem 0 1rem;
	}

	.loading-more {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 0.5rem;
		padding: 1rem 0;
	}

	.loading-pulse {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--alt-ash);
		animation: pulse 1.2s ease-in-out infinite;
	}

	.loading-text {
		font-family: var(--font-body);
		font-size: 0.82rem;
		font-style: italic;
		color: var(--alt-ash);
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

	@keyframes entry-in {
		to {
			opacity: 1;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.feed-entry {
			animation: none;
			opacity: 1;
		}
		.loading-pulse {
			animation: none;
			opacity: 1;
		}
	}
</style>
