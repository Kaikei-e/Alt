<script lang="ts" module>
export type { FeedGridApi, RemoveFeedResult } from "./feed-grid-types";
</script>

<script lang="ts">
	import type { Snippet } from "svelte";
	import { getFeedsWithCursorClient, getAllFeedsWithCursorClient } from "$lib/api/client/feeds";
	import { appendUniqueFeeds } from "$lib/domain/feed/dedupe";
	import type { RenderFeed } from "$lib/schema/feed";
	import type { FeedGridApi, RemoveFeedResult } from "./feed-grid-types";
	import DesktopFeedCard from "./DesktopFeedCard.svelte";
	import { onDestroy, onMount } from "svelte";
	import { infiniteScroll } from "$lib/actions/infinite-scroll";

	interface Props {
		onSelectFeed: (feed: RenderFeed, index: number, totalCount: number) => void;
		unreadOnly?: boolean;
		sortBy?: string;
		excludedFeedLinkIds?: string[];
		onReady?: (api: FeedGridApi) => void;
		fetchFn?: (cursor?: string, limit?: number) => Promise<import("$lib/api").CursorResponse<RenderFeed>>;
		cardRenderer?: Snippet<[{ feed: RenderFeed; index: number; isRead: boolean; onSelect: (feed: RenderFeed) => void }]>;
		gridClass?: string;
		/** data-testid for the grid element itself, so a caller can assert its layout. */
		gridTestId?: string;
		emptyText?: string;
		loadingText?: string;
		/**
		 * Rows the page already has — server-rendered on the first paint of
		 * `/feeds`. They are shown immediately and the first cursor page is
		 * merged in behind them, deduped by id.
		 */
		initialFeeds?: RenderFeed[];
		/**
		 * Richer stand-ins for `emptyText` / `loadingText`. The phone layouts each
		 * brought their own — a labelled empty region with an ornament and a way
		 * out, and a shimmer skeleton — and those are the ones worth keeping, so
		 * they are passed in rather than lost when their bespoke list was deleted.
		 */
		emptyContent?: Snippet;
		loadingContent?: Snippet;
	}

	let { onSelectFeed, unreadOnly = false, sortBy = "date_desc", excludedFeedLinkIds = [], onReady, fetchFn, cardRenderer, gridClass = "grid grid-cols-2 md:grid-cols-3 lg:grid-cols-3 gap-4", gridTestId, emptyText = "No dispatches on the wire", loadingText = "Retrieving dispatches", initialFeeds = [], emptyContent, loadingContent }: Props = $props();

	// Track initial load completion to only animate first batch
	let initialLoadDone = $state(false);

	// Simple state for infinite scroll.
	//
	// Seeded from `initialFeeds` so a server-rendered first screen is on the
	// page before any request goes out; `loadFeeds` merges the first cursor page
	// into it rather than replacing it.
	// svelte-ignore state_referenced_locally
	let feeds = $state<RenderFeed[]>([...initialFeeds]);

	/**
	 * The server-rendered rows, held separately so a *fresh* load can put them
	 * back under the first cursor page instead of blinking them away.
	 *
	 * Dropped the moment the list stops being about the same query — a filter
	 * change or an explicit refresh — because the seed was rendered for the
	 * unfiltered wire and would otherwise reappear under a filter that excludes
	 * it. Plain `let`, not `$state`: nothing reads it outside a load.
	 *
	 * A one-time capture on purpose — the seed is what the server rendered for
	 * this arrival, and a later value would be a different page's first screen.
	 */
	// svelte-ignore state_referenced_locally
	let seedFeeds: RenderFeed[] = [...initialFeeds];

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

	/**
	 * Connect-RPC serialises an absent cursor as the empty string — proto3 has no
	 * nullable `string` — so a terminal page arrives as `{ next_cursor: "",
	 * has_more: false }`. Reading `next_cursor` for truthiness rather than for
	 * `!== null` is what stops the infinite-scroll sentinel re-requesting page
	 * one forever (the `each_key_duplicate` crash on /feeds).
	 */
	function nextCursorOf(result: { next_cursor?: string | null }): string | undefined {
		return result.next_cursor ? result.next_cursor : undefined;
	}

	function hasMorePages(result: { next_cursor?: string | null; has_more?: boolean }): boolean {
		return result.has_more ?? Boolean(result.next_cursor);
	}

	/**
	 * A backend with nothing filed answers 404. That is an empty shelf, not a
	 * failure, and rendering it as an error stripe told readers something had
	 * broken when the honest answer was "nothing here yet".
	 */
	function isNotFound(err: unknown): boolean {
		return err instanceof Error && err.message.includes("404");
	}

	/** Fetch feeds using the correct API based on unreadOnly or custom fetchFn */
	function fetchFeedsApi(cursor?: string, limit: number = 20) {
		if (fetchFn) return fetchFn(cursor, limit);
		if (unreadOnly) {
			return getFeedsWithCursorClient(cursor, limit, excludedFeedLinkIds.length > 0 ? excludedFeedLinkIds : undefined);
		}
		return getAllFeedsWithCursorClient(cursor, limit, excludedFeedLinkIds.length > 0 ? excludedFeedLinkIds : undefined);
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
			nextFeedUrl: newVisibleFeeds[currentIndex]!.normalizedUrl,
			totalCount,
		};
	}

	/**
	 * Puts a removed feed back. The feed object itself was never dropped from
	 * `feeds` — only masked by `removedUrls` — so this is the exact inverse of
	 * `removeFeedByUrl`, order included.
	 */
	function restoreFeedByUrl(url: string): void {
		if (!removedUrls.has(url)) return;
		const next = new Set(removedUrls);
		next.delete(url);
		removedUrls = next;
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
					feeds = appendUniqueFeeds(feeds, result.data);
					nextCursor = nextCursorOf(result);
					hasNextPage = hasMorePages(result);
				}
			})
			.catch((err) => {
				console.error("Failed to fetch replacement feed:", err);
			});
	}

	/**
	 * Refresh the feed list from scratch.
	 * Used for Safari connection recovery after prolonged background.
	 */
	function refresh(): void {
		seedFeeds = [];
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

	// Track if onReady has been called
	let onReadyCalled = false;

	// Expose API to parent - only on initial mount
	$effect(() => {
		if (onReadyCalled || isLoading) return;

		onReadyCalled = true;
		onReady?.({
			removeFeedByUrl,
			restoreFeedByUrl,
			getVisibleFeeds: () => visibleFeeds,
			getFeedByUrl,
			fetchReplacementFeed,
			refresh,
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

			// Always merged rather than concatenated, on the first page as well as
			// on later ones. The keyed `{#each}` below cannot hold two siblings
			// under the same key — a duplicate is a hard runtime error, not a
			// warning (https://svelte.dev/e/each_key_duplicate) — and duplicates
			// arrive from two directions: the server-rendered `initialFeeds` the
			// first cursor page overlaps with, and cursor pages that overlap each
			// other when the wire gained a row between requests. `appendUniqueFeeds`
			// also matches on `normalizedUrl`, because the SSR read and the client
			// read are two separate queries and the same article can come back
			// under two different row ids.
			feeds = appendUniqueFeeds(cursor ? feeds : [...seedFeeds], result.data ?? []);

			nextCursor = nextCursorOf(result);
			hasNextPage = hasMorePages(result);
		} catch (err) {
			if (isNotFound(err)) {
				if (!cursor) feeds = [...seedFeeds];
				nextCursor = undefined;
				hasNextPage = false;
				error = null;
				return;
			}
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

	/** The error stripe's way out. Same reset `refresh()` performs. */
	let isRetrying = $state(false);

	/**
	 * Announced to screen readers when a retry lands. A retry replaces the error
	 * stripe with a grid of cards, which is a big visual change and a silent one:
	 * focus stays on a Retry button that is no longer there. The phone lists this
	 * grid replaces each carried a live region for exactly this, so it moves here
	 * rather than being lost with them.
	 */
	let statusMessage = $state("");
	let statusTimer: ReturnType<typeof setTimeout> | null = null;

	function announce(message: string) {
		if (statusTimer) clearTimeout(statusTimer);
		statusMessage = message;
		statusTimer = setTimeout(() => {
			statusMessage = "";
			statusTimer = null;
		}, 2000);
	}

	onDestroy(() => {
		if (statusTimer) clearTimeout(statusTimer);
	});

	async function retry() {
		if (isRetrying) return;
		isRetrying = true;
		try {
			error = null;
			isLoading = true;
			seedFeeds = [];
			feeds = [];
			nextCursor = undefined;
			hasNextPage = true;
			await loadFeeds();
			if (!error) announce("The list was reloaded.");
		} finally {
			isLoading = false;
			isRetrying = false;
		}
	}

	// Track filter key to detect changes
	let prevFilterKey = $state("");

	// Reset and reload when filters change
	$effect(() => {
		const filterKey = `${unreadOnly}:${sortBy}:${excludedFeedLinkIds.join(',')}`;

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
			const excludeChanged = (parts[2] ?? '') !== excludedFeedLinkIds.join(',');
			prevFilterKey = filterKey;

			if (unreadOnlyChanged || excludeChanged) {
				// Reset state and reload
				seedFeeds = [];
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
			// Mark initial load done so subsequent infinite scroll items appear instantly
			setTimeout(() => { initialLoadDone = true; }, 600);
		}
	});

</script>

<div class="wire-container">
	<div class="sr-only" role="status" aria-live="polite" aria-atomic="true">{statusMessage}</div>

	<!--
		`isLoading` alone would hide a server-rendered first screen behind a
		spinner until the first client request answered — which is the whole
		point of having sent it. Show what is already here; the fetch merges
		into it.
	-->
	{#if isLoading && visibleFeeds.length === 0}
		{#if loadingContent}
			{@render loadingContent()}
		{:else}
			<div class="loading-state">
				<span class="loading-pulse"></span>
				<span class="loading-text">{loadingText}&hellip;</span>
			</div>
		{/if}
	{:else if error}
		<div class="error-state" role="alert">
			<p class="error-message">{error.message}</p>
			<button type="button" class="retry-btn" onclick={() => void retry()} disabled={isRetrying}>
				{isRetrying ? "Retrying\u2026" : "Retry"}
			</button>
		</div>
	{:else if visibleFeeds.length === 0}
		{#if emptyContent}
			{@render emptyContent()}
		{:else}
			<p class="empty-state">{emptyText}</p>
		{/if}
	{:else}
		<div class={gridClass} data-testid={gridTestId}>
			{#each visibleFeeds as feed, index (feed.id)}
				<div class={initialLoadDone ? "" : "dispatch-item"} style={initialLoadDone ? "" : `--stagger: ${index};`}>
					{#if cardRenderer}
						{@render cardRenderer({ feed, index, isRead: feed.isRead ?? false, onSelect: (f) => onSelectFeed(f, index, visibleFeeds.length) })}
					{:else}
						<DesktopFeedCard {feed} isRead={feed.isRead ?? false} onSelect={(f) => onSelectFeed(f, index, visibleFeeds.length)} />
					{/if}
				</div>
			{/each}
		</div>

		<div
			use:infiniteScroll={{
				callback: loadMore,
				// `isLoading` is in the guard because a server-rendered first
				// screen is on the page *before* the first request answers, and on
				// a phone that screen is short enough that the sentinel is already
				// in view. Without it the sentinel fired straight away and page one
				// was requested twice.
				disabled: isLoading || isFetchingNextPage || !hasNextPage,
				threshold: 0.1,
				rootMargin: "0px 0px 200px 0px",
			}}
			class="load-more"
			role="status"
			aria-live="polite"
		>
			{#if isFetchingNextPage}
				<div class="loading-state">
					<span class="loading-pulse"></span>
					<span class="loading-text">Loading more&hellip;</span>
				</div>
			{:else if hasNextPage}
				<p class="scroll-hint">Scroll for more</p>
			{:else}
				<p class="scroll-hint">End of wire</p>
			{/if}
		</div>
	{/if}
</div>

<style>
	.wire-container {
		width: 100%;
	}

	.sr-only {
		position: absolute;
		left: -10000px;
		width: 1px;
		height: 1px;
		overflow: hidden;
	}

	.dispatch-item {
		opacity: 0;
		animation: entry-in 0.3s ease forwards;
		animation-delay: calc(var(--stagger) * 40ms);
	}

	.load-more {
		padding: 1.5rem 0;
	}

	.loading-state {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 2rem 0;
		justify-content: center;
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
		font-size: 0.85rem;
		font-style: italic;
		color: var(--alt-ash);
	}

	.error-state {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-terracotta);
		padding: 1rem 0;
		border-left: 3px solid var(--alt-terracotta);
		padding-left: 0.75rem;
	}

	.error-message {
		margin: 0;
	}

	.retry-btn {
		margin-top: 0.75rem;
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
		transition: background 0.15s, color 0.15s;
	}

	.retry-btn:active {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	.retry-btn:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}

	.empty-state {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-ash);
		padding: 2rem 0;
		margin: 0;
		text-align: center;
	}

	.scroll-hint {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-ash);
		text-align: center;
		margin: 0;
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
		.dispatch-item {
			animation: none;
			opacity: 1;
		}
		.loading-pulse {
			animation: none;
			opacity: 1;
		}
	}
</style>
