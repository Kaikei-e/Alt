<script lang="ts">
import { onMount } from "svelte";
import { page } from "$app/state";
import { useViewport } from "$lib/stores/viewport.svelte";

import { searchFeedsClient } from "$lib/api/client/feeds";
import { type RenderFeed, sanitizeFeed, toRenderFeed } from "$lib/schema/feed";
import { infiniteScroll } from "$lib/actions/infinite-scroll";
import DesktopFeedCard from "$lib/components/desktop/feeds/DesktopFeedCard.svelte";
import FeedDetailModal from "$lib/components/desktop/feeds/FeedDetailModal.svelte";

import SearchFeedsClient from "$lib/components/mobile/search/SearchFeedsClient.svelte";

const { isDesktop } = useViewport();

const initialQuery = page.url.searchParams.get("q")?.trim() ?? "";

let selectedFeed = $state<RenderFeed | null>(null);
let isModalOpen = $state(false);
let searchQuery = $state(initialQuery);
let lastSearchedQuery = $state("");

let feeds = $state<RenderFeed[]>([]);
let isLoading = $state(false);
let error = $state<Error | null>(null);

let cursor = $state<number | null>(null);
let hasMore = $state(false);
let isLoadingMore = $state(false);

let revealed = $state(false);
let initialLoadDone = $state(false);

const dateStr = $derived(
	new Date().toLocaleDateString("en-US", {
		weekday: "long",
		year: "numeric",
		month: "long",
		day: "numeric",
	}),
);

onMount(() => {
	requestAnimationFrame(() => {
		revealed = true;
	});
	if (initialQuery) {
		handleSearch();
	}
});

async function handleSearch() {
	if (!searchQuery.trim()) {
		feeds = [];
		error = null;
		lastSearchedQuery = "";
		cursor = null;
		hasMore = false;
		return;
	}

	try {
		isLoading = true;
		error = null;
		initialLoadDone = false;
		lastSearchedQuery = searchQuery.trim();
		const result = await searchFeedsClient(searchQuery.trim(), undefined, 20);

		if (result.error) {
			error = new Error(result.error);
			feeds = [];
			cursor = null;
			hasMore = false;
			isLoading = false;
			return;
		}

		const rawResults = result.results ?? [];
		feeds = rawResults.map((item) =>
			toRenderFeed(sanitizeFeed(item), item.tags),
		);
		cursor = result.next_cursor ?? null;
		hasMore = result.has_more ?? false;
	} catch (err) {
		error = err as Error;
		feeds = [];
		cursor = null;
		hasMore = false;
	} finally {
		isLoading = false;
		// Mark initial load done after animation completes so infinite scroll items render instantly
		setTimeout(() => {
			initialLoadDone = true;
		}, 600);
	}
}

const MAX_SEARCH_RESULTS = 200;

async function loadMore() {
	if (isLoadingMore || !hasMore) return;
	if (feeds.length >= MAX_SEARCH_RESULTS) {
		hasMore = false;
		return;
	}
	isLoadingMore = true;
	try {
		const result = await searchFeedsClient(
			lastSearchedQuery,
			cursor ?? undefined,
			20,
		);
		if (result.error) {
			hasMore = false;
			return;
		}
		const newFeeds = (result.results ?? []).map((item) =>
			toRenderFeed(sanitizeFeed(item), item.tags),
		);
		if (newFeeds.length === 0) {
			hasMore = false;
			return;
		}
		feeds = [...feeds, ...newFeeds];
		cursor = result.next_cursor ?? null;
		hasMore = result.has_more ?? false;
	} finally {
		isLoadingMore = false;
	}
}

function handleKeyDown(event: KeyboardEvent) {
	if (event.key === "Enter") {
		event.preventDefault();
		handleSearch();
	}
}

let currentIndex = $state(-1);

const hasPrevious = $derived(currentIndex > 0);
const hasNextFeed = $derived(
	(currentIndex >= 0 && currentIndex < feeds.length - 1) ||
		(currentIndex === feeds.length - 1 && hasMore),
);

function handleSelectFeed(feed: RenderFeed, index: number) {
	selectedFeed = feed;
	currentIndex = index;
	isModalOpen = true;
}

function handlePrevious() {
	if (currentIndex > 0) {
		selectedFeed = feeds[currentIndex - 1];
		currentIndex = currentIndex - 1;
	}
}

async function handleNext() {
	if (currentIndex >= 0 && currentIndex < feeds.length - 1) {
		selectedFeed = feeds[currentIndex + 1];
		currentIndex = currentIndex + 1;
	} else if (hasMore && !isLoadingMore) {
		await loadMore();
		if (currentIndex < feeds.length - 1) {
			selectedFeed = feeds[currentIndex + 1];
			currentIndex = currentIndex + 1;
		}
	}
}
</script>

<svelte:head>
	<title>Search - Alt</title>
</svelte:head>

{#if isDesktop}
	<div class="archive-page" class:revealed data-role="archive-desk-page">
		<header class="archive-header">
			<span class="archive-date">{dateStr}</span>
			<h1 class="archive-title">Archive Desk</h1>
			<div class="archive-rule" aria-hidden="true"></div>
		</header>

		<form onsubmit={(e) => { e.preventDefault(); handleSearch(); }} class="archive-search-form">
			<input
				type="search"
				class="archive-input"
				data-role="archive-search-input"
				bind:value={searchQuery}
				onkeydown={handleKeyDown}
				placeholder="Search by title, content, or author..."
				disabled={isLoading}
			/>
			<button
				type="submit"
				class="archive-btn"
				data-role="archive-search-btn"
				disabled={isLoading || !searchQuery.trim()}
			>
				{#if isLoading}
					<span class="loading-pulse"></span>
					<span class="archive-btn-text">Searching...</span>
				{:else}
					SEARCH
				{/if}
			</button>
		</form>

		<div class="w-full">
			{#if !lastSearchedQuery && !isLoading}
				<div class="archive-empty">
					<p class="archive-empty-text">Enter a query to search the archive.</p>
				</div>
			{:else if isLoading}
				<div class="archive-loading">
					<span class="loading-pulse"></span>
					<span class="archive-loading-text">Searching...</span>
				</div>
			{:else if error}
				<div class="error-stripe" role="alert">
					Error searching: {error.message}
				</div>
			{:else if feeds.length === 0}
				<div class="archive-empty">
					<p class="archive-empty-text">
						No results found for "{lastSearchedQuery}"
					</p>
				</div>
			{:else}
				<p class="archive-result-count">
					{feeds.length} result{feeds.length === 1 ? "" : "s"} for "{lastSearchedQuery}"
					{#if hasMore}<span class="archive-result-more">(scroll for more)</span>{/if}
				</p>
				<div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-3 gap-4">
					{#each feeds as feed, index (feed.id)}
						<div class={initialLoadDone ? "" : "stagger-entry"} style={initialLoadDone ? "" : `--stagger: ${index}`}>
							<DesktopFeedCard {feed} onSelect={(f: RenderFeed) => handleSelectFeed(f, index)} />
						</div>
					{/each}
				</div>

				{#if isLoadingMore}
					<div class="archive-loading" style="padding: 1.5rem 0;">
						<span class="loading-pulse"></span>
						<span class="archive-loading-text">Loading more...</span>
					</div>
				{/if}

				{#if !hasMore && feeds.length > 0}
					<p class="archive-end">No more results</p>
				{/if}

				{#if hasMore}
					<div
						use:infiniteScroll={{ callback: loadMore, disabled: isLoadingMore }}
						aria-hidden="true"
						style="height: 50px; min-height: 50px; width: 100%;"
					></div>
				{/if}
			{/if}
		</div>

		<FeedDetailModal
			bind:open={isModalOpen}
			feed={selectedFeed}
			onOpenChange={(open: boolean) => (isModalOpen = open)}
			{hasPrevious}
			hasNext={hasNextFeed}
			onPrevious={handlePrevious}
			onNext={handleNext}
			{feeds}
			{currentIndex}
		/>
	</div>
{:else}
	<SearchFeedsClient {initialQuery} />
{/if}

<style>
	.archive-page {
		opacity: 0;
		transform: translateY(6px);
		transition: opacity 0.4s ease, transform 0.4s ease;
	}

	.archive-page.revealed {
		opacity: 1;
		transform: translateY(0);
	}

	.archive-header {
		padding: 1.5rem 0 0;
	}

	.archive-date {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-ash);
		letter-spacing: 0.06em;
	}

	.archive-title {
		font-family: var(--font-display);
		font-size: 1.6rem;
		font-weight: 800;
		color: var(--alt-charcoal);
		letter-spacing: -0.01em;
		margin: 0.15rem 0 0;
		line-height: 1.2;
	}

	.archive-rule {
		height: 1px;
		background: var(--surface-border);
		margin-top: 0.75rem;
	}

	.archive-search-form {
		display: flex;
		gap: 0.5rem;
		max-width: 640px;
		margin-top: 1.25rem;
		margin-bottom: 1.5rem;
	}

	.archive-input {
		flex: 1;
		padding: 0.625rem 0.75rem;
		font-family: var(--font-body);
		font-size: 1rem;
		color: var(--alt-charcoal);
		background: var(--surface-bg);
		border: 1px solid var(--surface-border);
		border-radius: 0;
		outline: none;
		transition: border-color 0.15s;
	}

	.archive-input:focus {
		border-color: var(--alt-charcoal);
	}

	.archive-input::placeholder {
		color: var(--alt-ash);
	}

	.archive-btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 0.4rem;
		padding: 0 1.5rem;
		min-height: 44px;
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

	.archive-btn:hover:not(:disabled) {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	.archive-btn:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}

	.archive-btn-text {
		font-style: italic;
	}

	.archive-result-count {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-ash);
		letter-spacing: 0.04em;
		margin: 0 0 0.75rem;
	}

	.archive-result-more {
		color: var(--alt-ash);
	}

	.archive-empty {
		padding: 3rem 0;
		text-align: center;
	}

	.archive-empty-text {
		font-family: var(--font-body);
		font-size: 0.9rem;
		color: var(--alt-ash);
		font-style: italic;
		margin: 0;
	}

	.archive-loading {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 0.5rem;
		padding: 3rem 0;
	}

	.archive-loading-text {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-ash);
		font-style: italic;
	}

	.archive-end {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-ash);
		text-align: center;
		padding: 1rem 0;
		margin: 0;
	}

	.error-stripe {
		padding: 0.75rem 1rem;
		border-left: 3px solid var(--alt-terracotta);
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-terracotta);
	}

	.stagger-entry {
		opacity: 0;
		animation: reveal 0.3s ease forwards;
		animation-delay: calc(var(--stagger) * 60ms);
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
		.archive-page {
			opacity: 1;
			transform: none;
			transition: none;
		}

		.stagger-entry {
			animation: none;
			opacity: 1;
		}

		.loading-pulse {
			animation: none;
			opacity: 1;
		}
	}
</style>
