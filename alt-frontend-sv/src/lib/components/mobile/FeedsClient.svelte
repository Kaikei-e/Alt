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

			// Update visibleCount to show new feeds
			const allFeeds: RenderFeed[] = [...initialFeeds];
			const renderFeeds: RenderFeed[] = feeds.map((f: SanitizedFeed) =>
				toRenderFeed(f),
			);
			allFeeds.push(...renderFeeds);
			const filteredCount = allFeeds.filter(
				(feed) => !readFeeds.has(feed.normalizedUrl),
			).length;

			// 新しいフィードが追加されたが、すべて既読の場合
			// 無限ループを防ぐために hasMore を false にする
			if (filteredCount === 0 && allFeeds.length > 0 && response.next_cursor === null) {
				hasMore = false;
				cursor = null;
				console.log(
					"[FeedsClient] All loaded feeds are read and no more cursor, setting hasMore=false",
				);
			}

			visibleCount = Math.min(
				visibleCount + response.data.length,
				filteredCount,
			);
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
// This is handled by infiniteScroll action, but we still need to manage visibleCount
// when new feeds are loaded
$effect(() => {
	if (!browser) return;

	const allFeedsCount = initialFeeds.length + feeds.length;

	// When new feeds are added, increase visibleCount progressively
	if (visibleCount < allFeedsCount) {
		// Gradually increase visibleCount, but don't exceed allFeedsCount
		const targetCount = Math.min(visibleCount + STEP, allFeedsCount);
		visibleCount = targetCount;
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
	// Always show at least visibleCount items, but ensure sentinel is visible
	const allFeedsCount = filtered.length;
	// Show at least visibleCount items, but don't exceed allFeedsCount
	// This ensures sentinel is always visible when there are more feeds to load
	const countToShow = Math.min(visibleCount, allFeedsCount);
	return filtered.slice(0, countToShow);
});

const hasVisibleContent = $derived(initialFeeds.length > 0 || feeds.length > 0);

const isInitialLoadingState = $derived(
	isInitialLoading && initialFeeds.length === 0 && feeds.length === 0,
);

// visibleFeeds が 0 だが hasMore/cursor があるときは、自動で次ページを読む
// 無限ループ防止: 連続実行防止と既読フィードのみの場合の処理
let lastAutoLoadTime = $state(0);
let autoLoadAttempts = $state(0);
let lastFeedsLength = $state(0);
let lastVisibleFeedsLength = $state(-1); // Track previous visibleFeeds.length to avoid unnecessary re-runs
const AUTO_LOAD_COOLDOWN = 1000; // 1秒のクールダウン
const MAX_AUTO_LOAD_ATTEMPTS = 3; // 最大試行回数

// Track visibleFeeds.length separately to optimize $effect dependencies
const visibleFeedsLength = $derived(visibleFeeds.length);

$effect(() => {
	if (!browser) return;
	if (isLoading) return;

	const currentVisibleFeedsLength = visibleFeedsLength;
	const hasAnyFetched = initialFeeds.length > 0 || feeds.length > 0;
	const now = Date.now();
	const currentFeedsLength = initialFeeds.length + feeds.length;

	// Skip if visibleFeeds.length hasn't changed (optimization to reduce unnecessary re-runs)
	if (currentVisibleFeedsLength === lastVisibleFeedsLength && lastVisibleFeedsLength !== -1) {
		return;
	}
	lastVisibleFeedsLength = currentVisibleFeedsLength;

	// 連続実行防止: クールダウン期間内は実行しない
	if (now - lastAutoLoadTime < AUTO_LOAD_COOLDOWN) {
		return;
	}

	// フィード数が増えた場合は試行回数をリセット
	if (currentFeedsLength > lastFeedsLength) {
		autoLoadAttempts = 0;
		lastFeedsLength = currentFeedsLength;
	}

	// visibleFeedsが0で、hasMoreとcursorがある場合のみ自動読み込み
	if (hasAnyFetched && currentVisibleFeedsLength === 0 && hasMore && cursor) {
		// 最大試行回数を超えた場合は、すべて既読と判断して hasMore を false にする
		if (autoLoadAttempts >= MAX_AUTO_LOAD_ATTEMPTS) {
			console.log(
				"[FeedsClient] Max auto-load attempts reached, setting hasMore=false to prevent infinite loop",
			);
			hasMore = false;
			cursor = null;
			return;
		}

		const allFeedsCount = initialFeeds.length + feeds.length;
		if (allFeedsCount > 0 && currentVisibleFeedsLength === 0) {
			// 既にフィードがあるのにvisibleFeedsが0 = すべて既読
			// この場合、新しいフィードを読み込んでみる
			console.log(
				"[FeedsClient] visibleFeeds=0 & hasMore=true -> auto loadMore (all feeds read, attempt:",
				autoLoadAttempts + 1,
				")",
			);
			lastAutoLoadTime = now;
			autoLoadAttempts++;
			void loadMore();
		} else if (allFeedsCount === 0) {
			// まだフィードがない場合のみ自動読み込み
			console.log(
				"[FeedsClient] visibleFeeds=0 & hasMore=true -> auto loadMore (no feeds yet)",
			);
			lastAutoLoadTime = now;
			autoLoadAttempts++;
			void loadMore();
		}
	} else {
		// 条件が満たされない場合は試行回数をリセット
		autoLoadAttempts = 0;
		lastFeedsLength = currentFeedsLength;
	}
});
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
		class="p-5 max-w-2xl mx-auto overflow-y-auto overflow-x-hidden flex-1 min-h-0"
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

			<!-- Loading indicator -->
			{#if isLoading}
				<div class="py-4 text-center text-sm" style="color: var(--alt-text-secondary);">
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
				></div>
			{/if}
		{:else}
			<!-- Empty state -->
			<EmptyFeedState />
		{/if}
	</div>
</div>
