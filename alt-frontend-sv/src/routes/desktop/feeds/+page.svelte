<script lang="ts">
	import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
	import FeedFilters from "$lib/components/desktop/feeds/FeedFilters.svelte";
	import FeedGrid, { type FeedGridApi } from "$lib/components/desktop/feeds/FeedGrid.svelte";
	import FeedDetailModal from "$lib/components/desktop/feeds/FeedDetailModal.svelte";
	import type { RenderFeed } from "$lib/schema/feed";

	let selectedFeed = $state<RenderFeed | null>(null);
	let isModalOpen = $state(false);
	let filters = $state({ unreadOnly: false, sortBy: "date_desc" });
	let feedGridApi = $state<FeedGridApi | null>(null);

	function handleSelectFeed(feed: RenderFeed) {
		selectedFeed = feed;
		isModalOpen = true;
	}

	function handleFilterChange(newFilters: { unreadOnly: boolean; sortBy: string }) {
		filters = newFilters;
	}

	function handleMarkAsRead(feedUrl: string) {
		// Remove the feed from the grid
		feedGridApi?.removeFeedByUrl(feedUrl);
		isModalOpen = false;
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
/>
