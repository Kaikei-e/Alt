<script lang="ts">
	import { Loader2 } from "@lucide/svelte";
	import { getReadFeedsWithCursorClient } from "$lib/api/client/feeds";
	import type { RenderFeed } from "$lib/schema/feed";
	import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
	import DesktopFeedCard from "$lib/components/desktop/feeds/DesktopFeedCard.svelte";
	import FeedDetailModal from "$lib/components/desktop/feeds/FeedDetailModal.svelte";
	import { onMount } from "svelte";

	let selectedFeed = $state<RenderFeed | null>(null);
	let isModalOpen = $state(false);

	// Simple state for infinite scroll
	let feeds = $state<RenderFeed[]>([]);
	let isLoading = $state(true);
	let isFetchingNextPage = $state(false);
	let error = $state<Error | null>(null);
	let nextCursor = $state<string | undefined>(undefined);
	let hasNextPage = $state(true);

	let loadMoreTrigger = $state<HTMLDivElement | undefined>(undefined);

	async function loadFeeds(cursor?: string) {
		try {
			const result = await getReadFeedsWithCursorClient(cursor, 20);

			if (cursor) {
				// Append to existing feeds
				feeds = [...feeds, ...(result.data ?? [])];
			} else {
				// Initial load
				feeds = result.data ?? [];
			}

			nextCursor = result.next_cursor;
			hasNextPage = result.has_more ?? false;
		} catch (err) {
			error = err as Error;
		}
	}

	async function loadMore() {
		if (isFetchingNextPage || !hasNextPage) return;

		isFetchingNextPage = true;
		await loadFeeds(nextCursor);
		isFetchingNextPage = false;
	}

	onMount(async () => {
		try {
			isLoading = true;
			await loadFeeds();
		} catch (err) {
			error = err as Error;
		} finally {
			isLoading = false;
		}

		// Setup observer after initial load
		if (!loadMoreTrigger) return;

		const observer = new IntersectionObserver(
			(entries) => {
				const [entry] = entries;
				if (entry.isIntersecting && hasNextPage && !isFetchingNextPage) {
					loadMore();
				}
			},
			{ threshold: 0.5 }
		);

		observer.observe(loadMoreTrigger);

		return () => {
			observer.disconnect();
		};
	});

	function handleSelectFeed(feed: RenderFeed) {
		selectedFeed = feed;
		isModalOpen = true;
	}
</script>

<PageHeader title="Read History" description="Previously viewed feeds" />

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
	{:else if feeds.length === 0}
		<div class="text-center py-12">
			<p class="text-[var(--text-secondary)] text-sm">No viewed feeds yet</p>
		</div>
	{:else}
		<div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
			{#each feeds as feed (feed.id)}
				<DesktopFeedCard {feed} onSelect={handleSelectFeed} isRead={true} />
			{/each}
		</div>

		<div bind:this={loadMoreTrigger} class="py-8 text-center">
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

<FeedDetailModal
	bind:open={isModalOpen}
	feed={selectedFeed}
	onOpenChange={(open) => (isModalOpen = open)}
/>
