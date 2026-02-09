<script lang="ts">
import { Search, Loader2 } from "@lucide/svelte";
import { searchFeedsClient } from "$lib/api/client/feeds";
import { type RenderFeed, sanitizeFeed, toRenderFeed } from "$lib/schema/feed";
import { infiniteScroll } from "$lib/actions/infinite-scroll";
import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import DesktopFeedCard from "$lib/components/desktop/feeds/DesktopFeedCard.svelte";
import FeedDetailModal from "$lib/components/desktop/feeds/FeedDetailModal.svelte";
import { Input } from "$lib/components/ui/input";
import { Button } from "$lib/components/ui/button";

let selectedFeed = $state<RenderFeed | null>(null);
let isModalOpen = $state(false);
let searchQuery = $state("");
let lastSearchedQuery = $state("");

// Search state
let feeds = $state<RenderFeed[]>([]);
let isLoading = $state(false);
let error = $state<Error | null>(null);

// Pagination state
let cursor = $state<number | null>(null);
let hasMore = $state(false);
let isLoadingMore = $state(false);

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
		lastSearchedQuery = searchQuery.trim();
		const result = await searchFeedsClient(searchQuery.trim(), undefined, 20);

		// Check for API error (following mobile pattern)
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
	}
}

async function loadMore() {
	if (isLoadingMore || !hasMore) return;
	isLoadingMore = true;
	try {
		const result = await searchFeedsClient(
			lastSearchedQuery,
			cursor ?? undefined,
			20,
		);
		if (result.error) return;
		const newFeeds = (result.results ?? []).map((item) =>
			toRenderFeed(sanitizeFeed(item), item.tags),
		);
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

// Navigation state
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

<PageHeader title="Search Feeds" description="Search across all your feeds" />

<!-- Search input -->
<div class="mb-6">
	<form onsubmit={(e) => { e.preventDefault(); handleSearch(); }} class="max-w-2xl">
		<div class="flex gap-2">
			<div class="relative flex-1">
				<Search class="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-[var(--text-secondary)]" />
				<Input
					type="search"
					bind:value={searchQuery}
					onkeydown={handleKeyDown}
					placeholder="Search by title, content, or author..."
					class="pl-10 h-12"
					disabled={isLoading}
				/>
			</div>
			<Button
				type="submit"
				disabled={isLoading || !searchQuery.trim()}
				class="h-12 px-6 hover:opacity-90"
				style="background: var(--accent-primary); color: var(--accent-primary-foreground);"
			>
				{#if isLoading}
					<Loader2 class="h-4 w-4 animate-spin" />
				{:else}
					Search
				{/if}
			</Button>
		</div>
	</form>
</div>

<!-- Results -->
<div class="w-full">
	{#if !lastSearchedQuery && !isLoading}
		<div class="text-center py-12">
			<Search class="h-12 w-12 text-[var(--text-muted)] mx-auto mb-4" />
			<p class="text-[var(--text-secondary)] text-sm">Enter a search query and press Enter or click Search</p>
		</div>
	{:else if isLoading}
		<div class="flex items-center justify-center py-24">
			<Loader2 class="h-8 w-8 animate-spin text-[var(--accent-primary)]" />
		</div>
	{:else if error}
		<div class="text-center py-12">
			<p class="text-[var(--alt-error)] text-sm">
				Error searching: {error.message}
			</p>
		</div>
	{:else if feeds.length === 0}
		<div class="text-center py-12">
			<p class="text-[var(--text-secondary)] text-sm">
				No results found for "{lastSearchedQuery}"
			</p>
		</div>
	{:else}
		<div class="mb-4">
			<p class="text-sm text-[var(--text-secondary)]">
				{feeds.length} result{feeds.length === 1 ? "" : "s"} for "{lastSearchedQuery}"
				{#if hasMore}<span class="text-[var(--text-muted)]">(scroll for more)</span>{/if}
			</p>
		</div>
		<div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-3 gap-4">
			{#each feeds as feed, index (feed.id)}
				<DesktopFeedCard {feed} onSelect={(f) => handleSelectFeed(f, index)} />
			{/each}
		</div>

		{#if isLoadingMore}
			<div class="flex items-center justify-center py-6">
				<Loader2 class="h-6 w-6 animate-spin text-[var(--accent-primary)]" />
				<span class="ml-2 text-sm text-[var(--text-secondary)]">Loading more...</span>
			</div>
		{/if}

		{#if !hasMore && feeds.length > 0}
			<p class="text-center text-sm py-4 text-[var(--text-secondary)]">
				No more results
			</p>
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
	onOpenChange={(open) => (isModalOpen = open)}
	{hasPrevious}
	hasNext={hasNextFeed}
	onPrevious={handlePrevious}
	onNext={handleNext}
	{feeds}
	{currentIndex}
/>
