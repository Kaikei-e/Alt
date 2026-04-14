<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import { infiniteScroll } from "$lib/actions/infinite-scroll";
import { getReadFeedsWithCursorClient } from "$lib/api/client";
import type { RenderFeed, SanitizedFeed } from "$lib/schema/feed";
import { toRenderFeed } from "$lib/schema/feed";
import EmptyViewedFeedsState from "./EmptyViewedFeedsState.svelte";
import MorgueClipping from "./MorgueClipping.svelte";

const PAGE_SIZE = 20;

let feeds = $state<SanitizedFeed[]>([]);
let cursor = $state<string | null>(null);
let hasMore = $state(true);
let isLoading = $state(false);
let isInitialLoading = $state(true);
let error = $state<Error | null>(null);
let liveRegionMessage = $state("");
let isRetrying = $state(false);

let scrollContainerRef: HTMLDivElement | null = $state(null);

const getScrollRoot = $derived(browser ? scrollContainerRef : null);

onMount(() => {
	if (scrollContainerRef) {
		scrollContainerRef.scrollTop = 0;
	}
});

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

const refresh = async () => {
	cursor = null;
	hasMore = true;
	await loadInitial();
};

const retryFetch = async () => {
	isRetrying = true;
	try {
		await refresh();
		liveRegionMessage = "Filed clippings refreshed successfully";
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

onMount(() => {
	if (hasMore && !isLoading && feeds.length === 0) {
		void loadInitial();
	}
});

const renderFeeds = $derived.by(() => {
	return feeds.map((feed: SanitizedFeed) => toRenderFeed(feed));
});

const hasVisibleContent = $derived(feeds.length > 0);
const isInitialLoadingState = $derived(isInitialLoading && feeds.length === 0);
</script>

<div class="morgue-container">
	<div
		aria-live="polite"
		aria-atomic="true"
		class="sr-only"
	>
		{liveRegionMessage}
	</div>

	<div
		bind:this={scrollContainerRef}
		class="morgue-scroll"
		data-role="morgue-feed-list"
	>
		{#if isInitialLoadingState && !hasVisibleContent}
			<div class="loading-state">
				<span class="loading-pulse"></span>
				<span class="loading-text">Retrieving filed clippings&hellip;</span>
			</div>
		{:else if error}
			<div class="error-state-container">
				<div class="error-stripe">
					<p class="error-stripe-title">Error loading filings</p>
					<p>{error.message}</p>
					<button
						onclick={() => void retryFetch()}
						disabled={isRetrying}
						class="retry-btn"
					>
						{isRetrying ? "Retrying\u2026" : "Retry"}
					</button>
				</div>
			</div>
		{:else if renderFeeds.length > 0}
			<div
				class="morgue-list"
				style="content-visibility: auto; contain-intrinsic-size: 800px;"
			>
				{#each renderFeeds as feed, index (feed.link)}
					<div class="morgue-item" style="--stagger: {index};">
						<MorgueClipping {feed} />
					</div>
				{/each}
			</div>

			{#if !hasMore && renderFeeds.length > 0}
				<p class="scroll-hint">End of filings</p>
			{/if}

			{#if isLoading}
				<div class="loading-state loading-state--compact">
					<span class="loading-pulse"></span>
					<span class="loading-text">Retrieving more&hellip;</span>
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
				></div>
			{/if}
		{:else}
			<EmptyViewedFeedsState />
		{/if}
	</div>
</div>

<style>
	.morgue-container {
		height: 100%;
		display: flex;
		flex-direction: column;
		background: var(--app-bg);
	}

	.sr-only {
		position: absolute;
		left: -10000px;
		width: 1px;
		height: 1px;
		overflow: hidden;
	}

	.morgue-scroll {
		padding: 0 1.25rem 1.25rem;
		max-width: 42rem;
		margin: 0 auto;
		overflow-y: auto;
		overflow-x: clip;
		flex: 1;
		min-height: 0;
		width: 100%;
	}

	.morgue-list {
		display: flex;
		flex-direction: column;
	}

	.morgue-item {
		opacity: 0;
		animation: entry-in 0.3s ease forwards;
		animation-delay: calc(var(--stagger) * 40ms);
	}

	.loading-state {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 2rem 0;
		justify-content: center;
	}

	.loading-state--compact {
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
		font-size: 0.85rem;
		font-style: italic;
		color: var(--alt-ash);
	}

	.error-state-container {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		min-height: 50dvh;
		padding: 1.5rem;
	}

	.error-stripe {
		padding: 0.75rem 1rem;
		border-left: 3px solid var(--alt-terracotta);
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-terracotta);
	}

	.error-stripe-title {
		font-weight: 600;
		margin: 0 0 0.25rem;
	}

	.error-stripe p {
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
		padding: 0.4rem 0.75rem;
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

	.scroll-hint {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-ash);
		text-align: center;
		margin: 1.5rem 0 0.5rem;
	}

	@keyframes pulse {
		0%, 100% { opacity: 0.3; }
		50% { opacity: 1; }
	}

	@keyframes entry-in {
		to { opacity: 1; }
	}

	@media (prefers-reduced-motion: reduce) {
		.morgue-item {
			animation: none;
			opacity: 1;
		}
		.loading-pulse {
			animation: none;
			opacity: 1;
		}
	}
</style>
