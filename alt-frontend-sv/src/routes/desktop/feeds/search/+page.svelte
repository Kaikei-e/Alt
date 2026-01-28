<script lang="ts">
	import { Search, Loader2 } from "@lucide/svelte";
	import { searchFeedsClient } from "$lib/api/client/feeds";
	import { type RenderFeed, sanitizeFeed, toRenderFeed } from "$lib/schema/feed";
	import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
	import DesktopFeedCard from "$lib/components/desktop/feeds/DesktopFeedCard.svelte";
	import FeedDetailModal from "$lib/components/desktop/feeds/FeedDetailModal.svelte";
	import { Input } from "$lib/components/ui/input";
	import { Button } from "$lib/components/ui/button";

	let selectedFeed = $state<RenderFeed | null>(null);
	let isModalOpen = $state(false);
	let searchQuery = $state("");
	let lastSearchedQuery = $state("");

	// Simple state for search
	let feeds = $state<RenderFeed[]>([]);
	let isLoading = $state(false);
	let error = $state<Error | null>(null);

	async function handleSearch() {
		if (!searchQuery.trim()) {
			feeds = [];
			error = null;
			lastSearchedQuery = "";
			return;
		}

		try {
			isLoading = true;
			error = null;
			lastSearchedQuery = searchQuery.trim();
			const result = await searchFeedsClient(searchQuery.trim(), undefined, 50);

			// Check for API error (following mobile pattern)
			if (result.error) {
				error = new Error(result.error);
				feeds = [];
				isLoading = false;
				return;
			}

			const rawResults = result.results ?? [];
			feeds = rawResults.map(item => toRenderFeed(sanitizeFeed(item), item.tags));
		} catch (err) {
			error = err as Error;
			feeds = [];
		} finally {
			isLoading = false;
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
	const hasNext = $derived(currentIndex >= 0 && currentIndex < feeds.length - 1);

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

	function handleNext() {
		if (currentIndex >= 0 && currentIndex < feeds.length - 1) {
			selectedFeed = feeds[currentIndex + 1];
			currentIndex = currentIndex + 1;
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
				Found {feeds.length} result{feeds.length === 1 ? "" : "s"} for "{lastSearchedQuery}"
			</p>
		</div>
		<div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-3 gap-4">
			{#each feeds as feed, index (feed.id)}
				<DesktopFeedCard {feed} onSelect={(f) => handleSelectFeed(f, index)} />
			{/each}
		</div>
	{/if}
</div>

<FeedDetailModal
	bind:open={isModalOpen}
	feed={selectedFeed}
	onOpenChange={(open) => (isModalOpen = open)}
	{hasPrevious}
	{hasNext}
	onPrevious={handlePrevious}
	onNext={handleNext}
	{feeds}
	{currentIndex}
/>
