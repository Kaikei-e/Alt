<script lang="ts">
import { useViewport } from "$lib/stores/viewport.svelte";
import { Loader2 } from "@lucide/svelte";

// Desktop components
import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import FeedDetailModal from "$lib/components/desktop/feeds/FeedDetailModal.svelte";
import FeedGrid, {
	type FeedGridApi,
} from "$lib/components/desktop/feeds/FeedGrid.svelte";

// Mobile components
import FeedCard from "$lib/components/mobile/FeedCard.svelte";

// API
import { getFavoriteFeedsWithCursorClient } from "$lib/api/client/feeds";
import { updateFeedReadStatusClient } from "$lib/api/client/feeds";
import { infiniteScroll } from "$lib/actions/infinite-scroll";

import type { RenderFeed } from "$lib/schema/feed";

const { isDesktop } = useViewport();

// --- Desktop state ---
let selectedFeedUrl = $state<string | null>(null);
let isModalOpen = $state(false);
let feedGridApi = $state<FeedGridApi | null>(null);

const selectedFeed = $derived.by(() => {
	if (!selectedFeedUrl || !feedGridApi) return null;
	return feedGridApi.getFeedByUrl(selectedFeedUrl);
});

const currentIndex = $derived.by(() => {
	if (!selectedFeedUrl || !feedGridApi) return -1;
	const feeds = feedGridApi.getVisibleFeeds();
	return feeds.findIndex((f) => f.normalizedUrl === selectedFeedUrl);
});

const totalCount = $derived(feedGridApi?.getVisibleFeeds().length ?? 0);
const hasPrevious = $derived(currentIndex > 0);
const hasNext = $derived(currentIndex >= 0 && currentIndex < totalCount - 1);

function handlePrevious() {
	if (!feedGridApi || currentIndex <= 0) return;
	const feeds = feedGridApi.getVisibleFeeds();
	if (feeds[currentIndex - 1]) {
		selectedFeedUrl = feeds[currentIndex - 1].normalizedUrl;
	}
}

function handleNext() {
	if (!feedGridApi || currentIndex >= totalCount - 1) return;
	const feeds = feedGridApi.getVisibleFeeds();
	if (feeds[currentIndex + 1]) {
		selectedFeedUrl = feeds[currentIndex + 1].normalizedUrl;
	}
}

function handleSelectFeed(feed: RenderFeed, _index: number, _total: number) {
	selectedFeedUrl = feed.normalizedUrl;
	isModalOpen = true;
}

function handleFeedGridReady(api: FeedGridApi) {
	feedGridApi = api;
}

// --- Mobile state ---
let mobileFeeds = $state<RenderFeed[]>([]);
let mobileIsLoading = $state(true);
let mobileIsFetchingNext = $state(false);
let mobileError = $state<Error | null>(null);
let mobileNextCursor = $state<string | undefined>(undefined);
let mobileHasMore = $state(true);
let mobileReadUrls = $state<Set<string>>(new Set());

async function loadMobileFeeds(cursor?: string) {
	try {
		const result = await getFavoriteFeedsWithCursorClient(cursor, 20);
		if (cursor) {
			mobileFeeds = [...mobileFeeds, ...(result.data ?? [])];
		} else {
			mobileFeeds = result.data ?? [];
		}
		mobileNextCursor = result.next_cursor ?? undefined;
		mobileHasMore = result.has_more ?? false;
	} catch (err) {
		mobileError = err as Error;
	}
}

async function loadMoreMobile() {
	if (mobileIsFetchingNext || !mobileHasMore) return;
	mobileIsFetchingNext = true;
	try {
		await loadMobileFeeds(mobileNextCursor);
	} finally {
		mobileIsFetchingNext = false;
	}
}

function handleSetIsReadStatus(feedLink: string) {
	mobileReadUrls = new Set(mobileReadUrls).add(feedLink);
	updateFeedReadStatusClient(feedLink).catch((err) => {
		console.error("Failed to mark feed as read:", err);
	});
}

import { onMount } from "svelte";

onMount(async () => {
	if (!isDesktop) {
		try {
			await loadMobileFeeds();
		} finally {
			mobileIsLoading = false;
		}
	}
});
</script>

<svelte:head>
	<title>Favorites - Alt</title>
</svelte:head>

{#if isDesktop}
	<!-- Desktop: Grid view with modal -->
	<PageHeader title="Favorites" description="Your starred feeds" />

	<FeedGrid
		onSelectFeed={handleSelectFeed}
		onReady={handleFeedGridReady}
		fetchFn={getFavoriteFeedsWithCursorClient}
	/>

	<FeedDetailModal
		bind:open={isModalOpen}
		feed={selectedFeed}
		onOpenChange={(open) => (isModalOpen = open)}
		{hasPrevious}
		{hasNext}
		onPrevious={handlePrevious}
		onNext={handleNext}
		feeds={feedGridApi?.getVisibleFeeds() ?? []}
		{currentIndex}
	/>
{:else}
	<!-- Mobile: Card list with infinite scroll -->
	<div class="min-h-screen flex flex-col" style="background: var(--app-bg);">
		<div class="px-4 pt-6 pb-4">
			<h1 class="text-xl font-bold text-[var(--text-primary)]">Favorites</h1>
			<p class="text-sm text-[var(--text-secondary)] mt-1">Your starred feeds</p>
		</div>

		{#if mobileIsLoading}
			<div class="flex items-center justify-center py-24">
				<Loader2 class="h-8 w-8 animate-spin text-[var(--accent-primary)]" />
			</div>
		{:else if mobileError}
			<div class="text-center py-12 px-4">
				<p class="text-[var(--alt-error)] text-sm">
					Error loading favorites: {mobileError.message}
				</p>
			</div>
		{:else if mobileFeeds.length === 0}
			<div class="text-center py-12 px-4">
				<p class="text-[var(--text-secondary)] text-sm">
					No favorites yet. Star feeds from the swipe view to see them here.
				</p>
			</div>
		{:else}
			<div class="flex flex-col items-center gap-4 px-4 pb-8">
				{#each mobileFeeds as feed (feed.id)}
					<FeedCard
						{feed}
						isReadStatus={mobileReadUrls.has(feed.normalizedUrl)}
						setIsReadStatus={handleSetIsReadStatus}
					/>
				{/each}
			</div>

			<!-- Infinite scroll trigger -->
			<div
				use:infiniteScroll={{
					callback: loadMoreMobile,
					disabled: mobileIsFetchingNext || !mobileHasMore,
					threshold: 0.1,
					rootMargin: "0px 0px 200px 0px",
				}}
				class="py-8 text-center"
			>
				{#if mobileIsFetchingNext}
					<Loader2 class="h-6 w-6 animate-spin text-[var(--accent-primary)] mx-auto" />
				{:else if mobileHasMore}
					<p class="text-xs text-[var(--text-muted)]">Scroll for more</p>
				{:else}
					<p class="text-xs text-[var(--text-muted)]">No more favorites</p>
				{/if}
			</div>
		{/if}
	</div>
{/if}
