<script lang="ts">
import { useViewport } from "$lib/stores/viewport.svelte";

// Desktop components & deps
import { Loader2 } from "@lucide/svelte";
import { getReadFeedsWithCursorClient } from "$lib/api/client/feeds";
import type { RenderFeed } from "$lib/schema/feed";
import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import DesktopFeedCard from "$lib/components/desktop/feeds/DesktopFeedCard.svelte";
import FeedDetailModal from "$lib/components/desktop/feeds/FeedDetailModal.svelte";
import { onMount } from "svelte";
import { infiniteScroll } from "$lib/actions/infinite-scroll";

// Mobile components
import ViewedFeedsClient from "$lib/components/mobile/ViewedFeedsClient.svelte";

const { isDesktop } = useViewport();

// --- Desktop state ---
let selectedFeed = $state<RenderFeed | null>(null);
let isModalOpen = $state(false);

let feeds = $state<RenderFeed[]>([]);
let isLoading = $state(true);
let isFetchingNextPage = $state(false);
let error = $state<Error | null>(null);
let nextCursor = $state<string | undefined>(undefined);
let hasNextPage = $state(true);

async function loadFeeds(cursor?: string) {
	try {
		const result = await getReadFeedsWithCursorClient(cursor, 20);

		if (cursor) {
			feeds = [...feeds, ...(result.data ?? [])];
		} else {
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

onMount(() => {
	if (!isDesktop) return;

	void (async () => {
		try {
			isLoading = true;
			await loadFeeds();
		} catch (err) {
			error = err as Error;
		} finally {
			isLoading = false;
		}
	})();
});

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
	<title>History - Alt</title>
</svelte:head>

{#if isDesktop}
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
			<div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-3 gap-4">
				{#each feeds as feed, index (feed.id)}
					<DesktopFeedCard {feed} onSelect={(f) => handleSelectFeed(f, index)} isRead={true} />
				{/each}
			</div>

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
{:else}
	<div
		class="h-screen overflow-hidden flex flex-col"
		style="background: var(--app-bg);"
	>
		<div class="px-5 pt-4 pb-2">
			<h1
				class="text-2xl font-bold text-center"
				style="color: var(--alt-primary); font-family: var(--font-outfit, sans-serif);"
				data-testid="read-feeds-title"
			>
				History
			</h1>
		</div>

		<div class="flex-1 min-h-0 flex flex-col">
			<ViewedFeedsClient />
		</div>
	</div>
{/if}
