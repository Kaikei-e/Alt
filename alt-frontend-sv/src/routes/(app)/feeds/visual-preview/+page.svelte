<script lang="ts">
import { onMount } from "svelte";
import {
	listSubscriptionsClient,
	updateFeedReadStatusClient,
} from "$lib/api/client/feeds";
import { batchPrefetchImagesClient } from "$lib/api/client/articles";
import FeedDetailModal from "$lib/components/desktop/feeds/FeedDetailModal.svelte";
import FeedFilters from "$lib/components/desktop/feeds/FeedFilters.svelte";
import FeedGrid from "$lib/components/desktop/feeds/FeedGrid.svelte";
import type { FeedGridApi } from "$lib/components/desktop/feeds/feed-grid-types";
import VisualFeedCard from "$lib/components/desktop/feeds/VisualFeedCard.svelte";
import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import { Button } from "$lib/components/ui/button";
import type { ConnectFeedSource } from "$lib/connect/feeds";
import type { RenderFeed } from "$lib/schema/feed";
import { useViewport } from "$lib/stores/viewport.svelte";

interface PageData {
	initialFeeds?: RenderFeed[];
	error?: string;
}

const { data }: { data: PageData } = $props();
const { isDesktop } = useViewport();

// --- Desktop state ---
let selectedFeedUrl = $state<string | null>(null);
let isModalOpen = $state(false);
let filters = $state({
	unreadOnly: false,
	sortBy: "date_desc",
	excludedFeedLinkIds: [] as string[],
});
let feedGridApi = $state<FeedGridApi | null>(null);
let feedSources = $state<ConnectFeedSource[]>([]);
let isProcessingMarkAsRead = $state(false);
let isMarkingAsRead = $state(false);

// --- OG Image prefetch tracking ---
let prefetchedCount = $state(0);

onMount(async () => {
	try {
		feedSources = await listSubscriptionsClient();
	} catch (e) {
		console.error("Failed to load feed sources:", e);
	}
});

// Batch prefetch OG images for visible feeds that have articleId but no ogImageProxyUrl
$effect(() => {
	if (!feedGridApi) return;
	const visibleFeeds = feedGridApi.getVisibleFeeds();
	// Only trigger when feed count changes (new page loaded)
	if (visibleFeeds.length === prefetchedCount) return;

	const needsPrefetch = visibleFeeds.filter(
		(f: RenderFeed) => f.articleId && !f.ogImageProxyUrl,
	);

	if (needsPrefetch.length > 0) {
		const articleIds = needsPrefetch
			.map((f: RenderFeed) => f.articleId)
			.filter((id): id is string => id != null);
		batchPrefetchImagesClient(articleIds)
			.then((results) => {
				for (const result of results) {
					const feed = visibleFeeds.find(
						(f: RenderFeed) => f.articleId === result.articleId,
					);
					if (feed) {
						feed.ogImageProxyUrl = result.proxyUrl;
					}
				}
			})
			.catch((err) => {
				console.error("Failed to prefetch OG images:", err);
			});
	}

	prefetchedCount = visibleFeeds.length;
});

const selectedFeed = $derived.by(() => {
	if (!selectedFeedUrl || !feedGridApi) return null;
	return feedGridApi.getFeedByUrl(selectedFeedUrl);
});

const currentIndex = $derived.by(() => {
	if (!selectedFeedUrl || !feedGridApi) return -1;
	const feeds = feedGridApi.getVisibleFeeds();
	return feeds.findIndex(
		(f: RenderFeed) => f.normalizedUrl === selectedFeedUrl,
	);
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

function handleFilterChange(newFilters: {
	unreadOnly: boolean;
	sortBy: string;
	excludedFeedLinkIds: string[];
}) {
	filters = newFilters;
}

async function handleMarkAsReadInModal() {
	const feed = selectedFeed;
	if (!feed || isMarkingAsRead) return;

	try {
		isMarkingAsRead = true;
		await updateFeedReadStatusClient(feed.normalizedUrl);
		handleMarkAsRead(feed.normalizedUrl);
	} catch (error) {
		console.error("Failed to mark feed as read:", error);
	} finally {
		isMarkingAsRead = false;
	}
}

function handleMarkAsRead(feedUrl: string) {
	if (isProcessingMarkAsRead || !feedGridApi) return;

	isProcessingMarkAsRead = true;

	const currentFeeds = feedGridApi.getVisibleFeeds();
	const currentIdx = currentFeeds.findIndex(
		(f: RenderFeed) => f.normalizedUrl === feedUrl,
	);
	const isLastFeed = currentIdx === currentFeeds.length - 1;

	const { nextFeedUrl, totalCount } = feedGridApi.removeFeedByUrl(feedUrl);

	if (totalCount === 0 || isLastFeed) {
		isModalOpen = false;
		selectedFeedUrl = null;
	} else if (nextFeedUrl !== null) {
		selectedFeedUrl = nextFeedUrl;
	} else {
		isModalOpen = false;
		selectedFeedUrl = null;
	}

	feedGridApi.fetchReplacementFeed();
	isProcessingMarkAsRead = false;
}

function handleFeedGridReady(api: FeedGridApi) {
	feedGridApi = api;
}
</script>

<svelte:head>
	<title>Visual Preview - Alt</title>
</svelte:head>

{#if isDesktop}
	<!-- Desktop: Visual card grid with modal -->
	<PageHeader title="Visual Preview" description="Browse feeds with image thumbnails" />

	<FeedFilters
		unreadOnly={filters.unreadOnly}
		sortBy={filters.sortBy}
		excludedFeedLinkIds={filters.excludedFeedLinkIds}
		{feedSources}
		onFilterChange={handleFilterChange}
	/>

	<FeedGrid
		onSelectFeed={handleSelectFeed}
		unreadOnly={filters.unreadOnly}
		sortBy={filters.sortBy}
		excludedFeedLinkIds={filters.excludedFeedLinkIds}
		onReady={handleFeedGridReady}
		gridClass="grid grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-5"
	>
		{#snippet cardRenderer({ feed, index, isRead, onSelect }: { feed: RenderFeed; index: number; isRead: boolean; onSelect: (feed: RenderFeed) => void })}
			<VisualFeedCard {feed} {isRead} {onSelect} />
		{/snippet}
	</FeedGrid>

	<FeedDetailModal
		bind:open={isModalOpen}
		feed={selectedFeed}
		onOpenChange={(open: boolean) => (isModalOpen = open)}
		{hasPrevious}
		{hasNext}
		onPrevious={handlePrevious}
		onNext={handleNext}
		feeds={feedGridApi?.getVisibleFeeds() ?? []}
		{currentIndex}
	>
		{#snippet footerActions()}
			<Button
				onclick={handleMarkAsReadInModal}
				variant="outline"
				disabled={isMarkingAsRead || isProcessingMarkAsRead}
			>
				{isMarkingAsRead ? "Marking..." : "Mark as Read"}
			</Button>
		{/snippet}
	</FeedDetailModal>
{:else}
	<!-- Mobile: Redirect to swipe visual-preview -->
	<div class="flex flex-col items-center justify-center py-24 text-center">
		<p class="text-lg font-medium text-[var(--text-primary)] mb-2">
			Visual Preview has a swipe interface on mobile
		</p>
		<p class="text-sm text-[var(--text-secondary)] mb-6">
			Tap below to use the mobile-optimized experience.
		</p>
		<a
			href="/feeds/swipe/visual-preview"
			class="px-4 py-2 text-sm font-medium transition-colors hover:opacity-90"
			style="background: var(--accent-primary); color: var(--accent-primary-foreground);"
		>
			Go to Swipe Visual Preview
		</a>
	</div>
{/if}
