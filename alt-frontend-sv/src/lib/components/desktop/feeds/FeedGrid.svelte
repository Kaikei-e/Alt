<script lang="ts" module>
	export type FeedGridApi = {
		removeFeedByUrl: (url: string) => void;
	};
</script>

<script lang="ts">
	import { Loader2 } from "@lucide/svelte";
	import { getFeedsWithCursorClient } from "$lib/api/client/feeds";
	import type { RenderFeed } from "$lib/schema/feed";
	import DesktopFeedCard from "./DesktopFeedCard.svelte";
	import { onMount } from "svelte";

	interface Props {
		onSelectFeed: (feed: RenderFeed) => void;
		unreadOnly?: boolean;
		sortBy?: string;
		onReady?: (api: FeedGridApi) => void;
	}

	let { onSelectFeed, unreadOnly = false, sortBy = "date_desc", onReady }: Props = $props();

	// Simple state for infinite scroll
	let feeds = $state<RenderFeed[]>([]);

	// Track removed feed URLs for optimistic updates
	let removedUrls = $state<Set<string>>(new Set());

	// Filter out removed feeds
	const visibleFeeds = $derived(
		feeds.filter(feed => !removedUrls.has(feed.normalizedUrl))
	);

	// Remove a feed by URL (for marking as read)
	function removeFeedByUrl(url: string) {
		removedUrls = new Set(removedUrls).add(url);
	}

	// Expose API to parent
	$effect(() => {
		onReady?.({ removeFeedByUrl });
	});
	let isLoading = $state(true);
	let isFetchingNextPage = $state(false);
	let error = $state<Error | null>(null);
	let nextCursor = $state<string | undefined>(undefined);
	let hasNextPage = $state(true);

	let loadMoreTrigger = $state<HTMLDivElement | undefined>(undefined);

	async function loadFeeds(cursor?: string) {
		try {
			const result = await getFeedsWithCursorClient(cursor, 20);

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

	// Intersection Observer for infinite scroll
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
		<div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
			{#each visibleFeeds as feed (feed.id)}
				<DesktopFeedCard {feed} onSelect={onSelectFeed} />
			{/each}
		</div>

		<!-- Load more trigger -->
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
