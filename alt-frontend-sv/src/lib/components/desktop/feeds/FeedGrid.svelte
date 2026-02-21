<script lang="ts" module>
import type { RenderFeed } from "$lib/schema/feed";

export type RemoveFeedResult = {
	nextFeedUrl: string | null;
	totalCount: number;
};

export type FeedGridApi = {
	/** Synchronously removes a feed and returns navigation info */
	removeFeedByUrl: (url: string) => RemoveFeedResult;
	/** Get all currently visible feeds */
	getVisibleFeeds: () => RenderFeed[];
	/** Get a specific feed by URL */
	getFeedByUrl: (url: string) => RenderFeed | null;
	/** Fetch a replacement feed in the background (fire-and-forget) */
	fetchReplacementFeed: () => void;
};
</script>

<script lang="ts">
	import { Loader2 } from "@lucide/svelte";
	import { getFeedsWithCursorClient, getAllFeedsWithCursorClient } from "$lib/api/client/feeds";
	import DesktopFeedCard from "./DesktopFeedCard.svelte";
	import { onMount } from "svelte";
	import { infiniteScroll } from "$lib/actions/infinite-scroll";

	interface Props {
		onSelectFeed: (feed: RenderFeed, index: number, totalCount: number) => void;
		unreadOnly?: boolean;
		sortBy?: string;
		excludedFeedLinkId?: string | null;
		onReady?: (api: FeedGridApi) => void;
		fetchFn?: (cursor?: string, limit?: number) => Promise<import("$lib/api").CursorResponse<RenderFeed>>;
	}

	let { onSelectFeed, unreadOnly = false, sortBy = "date_desc", excludedFeedLinkId = null, onReady, fetchFn }: Props = $props();

	// Simple state for infinite scroll
	let feeds = $state<RenderFeed[]>([]);

	// Track removed feed URLs for optimistic updates
	let removedUrls = $state<Set<string>>(new Set());

	// Sort feeds client-side
	function sortFeeds(items: RenderFeed[], sort: string): RenderFeed[] {
		const sorted = [...items];
		switch (sort) {
			case "date_asc":
				sorted.sort((a, b) => (a.created_at ?? "").localeCompare(b.created_at ?? ""));
				break;
			case "title_asc":
				sorted.sort((a, b) => (a.title ?? "").localeCompare(b.title ?? "", undefined, { sensitivity: "base" }));
				break;
			case "title_desc":
				sorted.sort((a, b) => (b.title ?? "").localeCompare(a.title ?? "", undefined, { sensitivity: "base" }));
				break;
			case "date_desc":
			default:
				// Server default order is date_desc — no re-sort needed for fresh data,
				// but we sort explicitly to handle mixed pages correctly
				sorted.sort((a, b) => (b.created_at ?? "").localeCompare(a.created_at ?? ""));
				break;
		}
		return sorted;
	}

	// Filter out removed feeds, then apply sort
	const visibleFeeds = $derived(
		sortFeeds(
			feeds.filter(feed => !removedUrls.has(feed.normalizedUrl)),
			sortBy,
		)
	);

	/** Fetch feeds using the correct API based on unreadOnly or custom fetchFn */
	function fetchFeedsApi(cursor?: string, limit: number = 20) {
		if (fetchFn) return fetchFn(cursor, limit);
		if (unreadOnly) {
			return getFeedsWithCursorClient(cursor, limit, excludedFeedLinkId ?? undefined);
		}
		return getAllFeedsWithCursorClient(cursor, limit, excludedFeedLinkId ?? undefined);
	}

	/**
	 * Synchronously removes a feed by URL and returns navigation info.
	 * This is the key fix for the race condition - no async operations here.
	 *
	 * Navigation behavior:
	 * - If there's a next feed, return its URL (navigate forward)
	 * - If no next feed (was viewing last item), return null (close modal)
	 */
	function removeFeedByUrl(url: string): RemoveFeedResult {
		// Find the index of the feed being removed BEFORE mutation
		const currentIndex = visibleFeeds.findIndex((f) => f.normalizedUrl === url);

		// If URL not found, return null to close modal (defensive)
		if (currentIndex === -1) {
			return { nextFeedUrl: null, totalCount: visibleFeeds.length };
		}

		const wasLastItem = currentIndex === visibleFeeds.length - 1;

		// Synchronously update removed URLs
		removedUrls = new Set(removedUrls).add(url);

		// Calculate the new visible feeds (after removal)
		const newVisibleFeeds = feeds.filter((f) => !removedUrls.has(f.normalizedUrl));
		const totalCount = newVisibleFeeds.length;

		if (totalCount === 0) {
			return { nextFeedUrl: null, totalCount: 0 };
		}

		// If the removed item was the last one, return null to signal "close modal"
		// (Don't navigate to previous - this matches expected UX)
		if (wasLastItem) {
			return { nextFeedUrl: null, totalCount };
		}

		// Return the item at the same index (which is now the "next" item)
		// Safety check: ensure index is within bounds
		if (currentIndex >= newVisibleFeeds.length) {
			return { nextFeedUrl: null, totalCount };
		}

		return {
			nextFeedUrl: newVisibleFeeds[currentIndex].normalizedUrl,
			totalCount,
		};
	}

	/**
	 * Get a feed by its URL.
	 */
	function getFeedByUrl(url: string): RenderFeed | null {
		return visibleFeeds.find((f) => f.normalizedUrl === url) ?? null;
	}

	/**
	 * Fetch a replacement feed in the background (fire-and-forget).
	 * Separated from removeFeedByUrl to avoid race conditions.
	 */
	function fetchReplacementFeed(): void {
		if (!hasNextPage || !nextCursor) return;

		// Fire-and-forget: don't await, let it complete in the background
		fetchFeedsApi(nextCursor, 1)
			.then((result) => {
				if (result.data?.length > 0) {
					feeds = [...feeds, ...result.data];
					nextCursor = result.next_cursor ?? undefined;
					hasNextPage = result.has_more ?? false;
				}
			})
			.catch((err) => {
				console.error("Failed to fetch replacement feed:", err);
			});
	}

	// Track if onReady has been called
	let onReadyCalled = false;

	// Expose API to parent - only on initial mount
	$effect(() => {
		if (onReadyCalled || isLoading) return;

		onReadyCalled = true;
		onReady?.({
			removeFeedByUrl,
			getVisibleFeeds: () => visibleFeeds,
			getFeedByUrl,
			fetchReplacementFeed,
		});
	});
	let isLoading = $state(true);
	let isFetchingNextPage = $state(false);
	let error = $state<Error | null>(null);
	let nextCursor = $state<string | undefined>(undefined);
	let hasNextPage = $state(true);

	async function loadFeeds(cursor?: string) {
		try {
			const result = await fetchFeedsApi(cursor, 20);

			if (cursor) {
				// Append to existing feeds
				feeds = [...feeds, ...(result.data ?? [])];
			} else {
				// Initial load
				feeds = result.data ?? [];
			}

			nextCursor = result.next_cursor ?? undefined;
			hasNextPage = result.has_more ?? false;
		} catch (err) {
			error = err as Error;
		}
	}

	async function loadMore() {
		if (isFetchingNextPage || !hasNextPage) return;

		isFetchingNextPage = true;
		try {
			await loadFeeds(nextCursor);
		} finally {
			isFetchingNextPage = false;
		}
	}

	// Track filter key to detect changes
	let prevFilterKey = $state("");

	// Reset and reload when filters change
	$effect(() => {
		const filterKey = `${unreadOnly}:${sortBy}:${excludedFeedLinkId ?? ''}`;

		// Skip the initial run (handled by onMount)
		if (prevFilterKey === "") {
			prevFilterKey = filterKey;
			return;
		}

		// Only reload from server if unreadOnly or excludedFeedLinkId changed (different data source)
		// Sort changes are handled client-side via the derived visibleFeeds
		if (filterKey !== prevFilterKey) {
			const parts = prevFilterKey.split(":");
			const unreadOnlyChanged = parts[0] !== String(unreadOnly);
			const excludeChanged = (parts[2] ?? '') !== (excludedFeedLinkId ?? '');
			prevFilterKey = filterKey;

			if (unreadOnlyChanged || excludeChanged) {
				// Reset state and reload
				feeds = [];
				nextCursor = undefined;
				hasNextPage = true;
				removedUrls = new Set();
				error = null;
				isLoading = true;

				loadFeeds().finally(() => {
					isLoading = false;
				});
			}
		}
	});

	// Initial data load
	onMount(async () => {
		try {
			isLoading = true;
			await loadFeeds();
		} catch (err) {
			error = err as Error;
		} finally {
			isLoading = false;
		}
	});

</script>

<div class="w-full">
	{#if isLoading}
		<div class="flex items-center justify-center py-24">
			<Loader2 class="h-8 w-8 animate-spin text-[var(--accent-primary)]" />
		</div>
	{:else if error}
		<div class="text-center py-12">
			<p class="text-[var(--alt-error)] text-sm">
				Error loading feeds: {error.message}
			</p>
		</div>
	{:else if visibleFeeds.length === 0}
		<div class="text-center py-12">
			<p class="text-[var(--text-secondary)] text-sm">No feeds found</p>
		</div>
	{:else}
		<!-- Grid layout -->
		<div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-3 gap-4">
			{#each visibleFeeds as feed, index (feed.id)}
				<DesktopFeedCard {feed} isRead={feed.isRead ?? false} onSelect={(f) => onSelectFeed(f, index, visibleFeeds.length)} />
			{/each}
		</div>

		<!-- Load more trigger -->
		<div
			use:infiniteScroll={{
				callback: loadMore,
				disabled: isFetchingNextPage || !hasNextPage,
				threshold: 0.1,
				rootMargin: "0px 0px 200px 0px",
			}}
			class="py-8 text-center"
		>
			{#if isFetchingNextPage}
				<Loader2 class="h-6 w-6 animate-spin text-[var(--accent-primary)] mx-auto" />
			{:else if hasNextPage}
				<p class="text-xs text-[var(--text-muted)]">Scroll for more</p>
			{:else}
				<p class="text-xs text-[var(--text-muted)]">No more feeds</p>
			{/if}
		</div>
	{/if}
</div>
