<script lang="ts">
import { onMount } from "svelte";
import {
	listSubscriptionsClient,
	updateFeedReadStatusClient,
} from "$lib/api/client/feeds";
// Desktop components
import FeedDetailModal from "$lib/components/desktop/feeds/FeedDetailModal.svelte";
import FeedFilters from "$lib/components/desktop/feeds/FeedFilters.svelte";
import FeedGrid, {
	type FeedGridApi,
} from "$lib/components/desktop/feeds/FeedGrid.svelte";
import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
// Mobile components
import FeedsClient from "$lib/components/mobile/FeedsClient.svelte";
import MobileFeedExcludeFilter from "$lib/components/mobile/feeds/MobileFeedExcludeFilter.svelte";
import MobileFeedsHero from "$lib/components/mobile/MobileFeedsHero.svelte";
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

// --- Mobile state ---
let mobileExcludedFeedLinkId = $state<string | null>(null);

// --- Desktop state ---
let selectedFeedUrl = $state<string | null>(null);
let isModalOpen = $state(false);
let filters = $state({
	unreadOnly: false,
	sortBy: "date_desc",
	excludedFeedLinkId: null as string | null,
});
let feedGridApi = $state<FeedGridApi | null>(null);
let feedSources = $state<ConnectFeedSource[]>([]);
let isProcessingMarkAsRead = $state(false);
let isMarkingAsRead = $state(false);

onMount(async () => {
	try {
		feedSources = await listSubscriptionsClient();
	} catch (e) {
		console.error("Failed to load feed sources:", e);
	}
});

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

function handleFilterChange(newFilters: {
	unreadOnly: boolean;
	sortBy: string;
	excludedFeedLinkId: string | null;
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
	const currentIdx = currentFeeds.findIndex((f) => f.normalizedUrl === feedUrl);
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
	<title>Feeds - Alt</title>
</svelte:head>

{#if isDesktop}
	<!-- Desktop: Grid view with modal -->
	<PageHeader title="Feeds" description="Browse all RSS feeds" />

	<FeedFilters
		unreadOnly={filters.unreadOnly}
		sortBy={filters.sortBy}
		excludedFeedLinkId={filters.excludedFeedLinkId}
		{feedSources}
		onFilterChange={handleFilterChange}
	/>

	<FeedGrid
		onSelectFeed={handleSelectFeed}
		unreadOnly={filters.unreadOnly}
		sortBy={filters.sortBy}
		excludedFeedLinkId={filters.excludedFeedLinkId}
		onReady={handleFeedGridReady}
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
	<!-- Mobile: Swipe card layout -->
	<div
		class="h-screen overflow-hidden flex flex-col"
		style="background: var(--app-bg);"
	>
		<MobileFeedsHero />
		<MobileFeedExcludeFilter
			sources={feedSources}
			excludedSourceId={mobileExcludedFeedLinkId}
			onExclude={(id) => (mobileExcludedFeedLinkId = id)}
			onClearExclusion={() => (mobileExcludedFeedLinkId = null)}
		/>
		<div class="flex-1 min-h-0 flex flex-col">
			<FeedsClient
				initialFeeds={data.initialFeeds || []}
				excludeFeedLinkId={mobileExcludedFeedLinkId}
			/>
		</div>
	</div>
{/if}
