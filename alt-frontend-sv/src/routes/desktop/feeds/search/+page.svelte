<script lang="ts">
	import { Search, Loader2 } from "@lucide/svelte";
	import { searchFeedsClient } from "$lib/api/client/feeds";
	import type { RenderFeed } from "$lib/schema/feed";
	import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
	import DesktopFeedCard from "$lib/components/desktop/feeds/DesktopFeedCard.svelte";
	import FeedDetailModal from "$lib/components/desktop/feeds/FeedDetailModal.svelte";
	import { Input } from "$lib/components/ui/input";

	let selectedFeed = $state<RenderFeed | null>(null);
	let isModalOpen = $state(false);
	let searchQuery = $state("");
	let debouncedQuery = $state("");

	// Simple state for search
	let feeds = $state<RenderFeed[]>([]);
	let isLoading = $state(false);
	let error = $state<Error | null>(null);

	// Debounce search query
	let debounceTimeout: ReturnType<typeof setTimeout>;
	$effect(() => {
		clearTimeout(debounceTimeout);
		debounceTimeout = setTimeout(() => {
			debouncedQuery = searchQuery;
		}, 500);
	});

	// Trigger search when debounced query changes
	$effect(() => {
		async function search() {
			if (!debouncedQuery.trim()) {
				feeds = [];
				error = null;
				return;
			}

			try {
				isLoading = true;
				error = null;
				const result = await searchFeedsClient(debouncedQuery, undefined, 50);
				feeds = result.results ?? [];
			} catch (err) {
				error = err as Error;
				feeds = [];
			} finally {
				isLoading = false;
			}
		}

		search();
	});

	function handleSelectFeed(feed: RenderFeed) {
		selectedFeed = feed;
		isModalOpen = true;
	}
</script>

<PageHeader title="Search Feeds" description="Search across all your feeds" />

<!-- Search input -->
<div class="mb-6">
	<div class="relative max-w-2xl">
		<Search class="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-[var(--text-secondary)]" />
		<Input
			type="search"
			bind:value={searchQuery}
			placeholder="Search by title, content, or author..."
			class="pl-10 h-12"
		/>
	</div>
</div>

<!-- Results -->
<div class="w-full">
	{#if searchQuery.trim().length === 0}
		<div class="text-center py-12">
			<Search class="h-12 w-12 text-[var(--text-muted)] mx-auto mb-4" />
			<p class="text-[var(--text-secondary)] text-sm">Enter a search query to get started</p>
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
				No results found for "{debouncedQuery}"
			</p>
		</div>
	{:else}
		<div class="mb-4">
			<p class="text-sm text-[var(--text-secondary)]">
				Found {feeds.length} result{feeds.length === 1 ? "" : "s"}
			</p>
		</div>
		<div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
			{#each feeds as feed (feed.id)}
				<DesktopFeedCard {feed} onSelect={handleSelectFeed} />
			{/each}
		</div>
	{/if}
</div>

<FeedDetailModal
	bind:open={isModalOpen}
	feed={selectedFeed}
	onOpenChange={(open) => (isModalOpen = open)}
/>
