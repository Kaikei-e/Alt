<script lang="ts">
	import { tick } from "svelte";
	import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
	import FeedFilters from "$lib/components/desktop/feeds/FeedFilters.svelte";
	import FeedGrid, { type FeedGridApi } from "$lib/components/desktop/feeds/FeedGrid.svelte";
	import FeedDetailModal from "$lib/components/desktop/feeds/FeedDetailModal.svelte";
	import type { RenderFeed } from "$lib/schema/feed";

	let selectedFeed = $state<RenderFeed | null>(null);
	let isModalOpen = $state(false);
	let filters = $state({ unreadOnly: false, sortBy: "date_desc" });
	let feedGridApi = $state<FeedGridApi | null>(null);

	// Navigation state - track index and total count directly
	let currentIndex = $state(-1);
	let totalCount = $state(0);

	const hasPrevious = $derived(currentIndex > 0);
	const hasNext = $derived(currentIndex >= 0 && currentIndex < totalCount - 1);

	function handlePrevious() {
		if (!feedGridApi || currentIndex <= 0) return;
		const feeds = feedGridApi.getVisibleFeeds();
		if (feeds[currentIndex - 1]) {
			selectedFeed = feeds[currentIndex - 1];
			currentIndex = currentIndex - 1;
		}
	}

	function handleNext() {
		if (!feedGridApi || currentIndex >= totalCount - 1) return;
		const feeds = feedGridApi.getVisibleFeeds();
		if (feeds[currentIndex + 1]) {
			selectedFeed = feeds[currentIndex + 1];
			currentIndex = currentIndex + 1;
		}
	}

	function handleSelectFeed(feed: RenderFeed, index: number, total: number) {
		selectedFeed = feed;
		currentIndex = index;
		totalCount = total;
		isModalOpen = true;
	}

	function handleFilterChange(newFilters: { unreadOnly: boolean; sortBy: string }) {
		filters = newFilters;
	}

	async function handleMarkAsRead(feedUrl: string) {
		// Check if there's a next feed BEFORE removing
		const hadNext = currentIndex < totalCount - 1;

		// Remove the feed from the grid
		await feedGridApi?.removeFeedByUrl(feedUrl);

		// Wait for Svelte to update derived state
		await tick();

		// Get updated feeds list and navigate
		const feeds = feedGridApi?.getVisibleFeeds() ?? [];
		totalCount = feeds.length;

		if (feeds.length === 0 || !hadNext) {
			// No more feeds OR was on the last feed (no next), close modal
			isModalOpen = false;
			selectedFeed = null;
			currentIndex = -1;
		} else {
			// Show the feed that's now at the current index (this is the "next" feed)
			selectedFeed = feeds[currentIndex];
		}
	}

	function handleFeedGridReady(api: FeedGridApi) {
		feedGridApi = api;
	}
</script>

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
/>
