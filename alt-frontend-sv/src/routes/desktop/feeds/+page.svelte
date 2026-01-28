<script lang="ts">
import FeedDetailModal from "$lib/components/desktop/feeds/FeedDetailModal.svelte";
import FeedFilters from "$lib/components/desktop/feeds/FeedFilters.svelte";
import FeedGrid, {
	type FeedGridApi,
} from "$lib/components/desktop/feeds/FeedGrid.svelte";
import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import type { RenderFeed } from "$lib/schema/feed";

// URL-based tracking to prevent race conditions
let selectedFeedUrl = $state<string | null>(null);
let isModalOpen = $state(false);
let filters = $state({ unreadOnly: false, sortBy: "date_desc" });
let feedGridApi = $state<FeedGridApi | null>(null);

// Processing flag to prevent duplicate clicks
let isProcessingMarkAsRead = $state(false);

// Derive selectedFeed from URL - stable across array mutations
const selectedFeed = $derived.by(() => {
	if (!selectedFeedUrl || !feedGridApi) return null;
	return feedGridApi.getFeedByUrl(selectedFeedUrl);
});

// Get current index and total count from the API
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
}) {
	filters = newFilters;
}

function handleMarkAsRead(feedUrl: string) {
	// Prevent duplicate clicks
	if (isProcessingMarkAsRead || !feedGridApi) return;

	isProcessingMarkAsRead = true;

	// Check current position BEFORE removal
	const currentFeeds = feedGridApi.getVisibleFeeds();
	const currentIdx = currentFeeds.findIndex((f) => f.normalizedUrl === feedUrl);
	const isLastFeed = currentIdx === currentFeeds.length - 1;

	// Remove the feed
	const { nextFeedUrl, totalCount } = feedGridApi.removeFeedByUrl(feedUrl);

	// Decide navigation: close if last feed or no feeds left
	if (totalCount === 0 || isLastFeed) {
		// Close modal when marking the last feed as read
		isModalOpen = false;
		selectedFeedUrl = null;
	} else if (nextFeedUrl !== null) {
		// Navigate to next feed
		selectedFeedUrl = nextFeedUrl;
	} else {
		// Fallback: close modal
		isModalOpen = false;
		selectedFeedUrl = null;
	}

	// Fire-and-forget: fetch replacement feed in the background
	feedGridApi.fetchReplacementFeed();

	// Reset processing flag
	isProcessingMarkAsRead = false;
}

function handleFeedGridReady(api: FeedGridApi) {
	feedGridApi = api;
}
</script>

<svelte:head>
	<title>Feeds - Alt</title>
</svelte:head>

<PageHeader title="Feeds" description="Browse all RSS feeds" />

<FeedFilters
	unreadOnly={filters.unreadOnly}
	sortBy={filters.sortBy}
	onFilterChange={handleFilterChange}
/>

<FeedGrid
	onSelectFeed={handleSelectFeed}
	unreadOnly={filters.unreadOnly}
	sortBy={filters.sortBy}
	onReady={handleFeedGridReady}
/>

<FeedDetailModal
	bind:open={isModalOpen}
	feed={selectedFeed}
	onOpenChange={(open) => (isModalOpen = open)}
	onMarkAsRead={handleMarkAsRead}
	{hasPrevious}
	{hasNext}
	onPrevious={handlePrevious}
	onNext={handleNext}
	feeds={feedGridApi?.getVisibleFeeds() ?? []}
	{currentIndex}
	disableMarkAsRead={isProcessingMarkAsRead}
/>
