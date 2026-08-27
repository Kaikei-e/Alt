<script lang="ts">
import { onMount } from "svelte";
import { batchPrefetchImagesClient } from "$lib/api/client/articles";
import {
	listSubscriptionsClient,
	updateFeedReadStatusClient,
} from "$lib/api/client/feeds";
import FeedDetailModal from "$lib/components/desktop/feeds/FeedDetailModal.svelte";
import FeedFilters from "$lib/components/desktop/feeds/FeedFilters.svelte";
import FeedGrid from "$lib/components/desktop/feeds/FeedGrid.svelte";
import type { FeedGridApi } from "$lib/components/desktop/feeds/feed-grid-types";
import VisualFeedCard from "$lib/components/desktop/feeds/VisualFeedCard.svelte";
import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import MobileGalleryTile from "$lib/components/mobile/feeds/gallery/MobileGalleryTile.svelte";
import type { ConnectFeedSource } from "$lib/connect/feeds";
import type { RenderFeed } from "$lib/schema/feed";
import { ogImageOverlay } from "$lib/stores/ogImageOverlay.svelte";
import { useViewport } from "$lib/stores/viewport.svelte";
import { selectOgImagePrefetchIds } from "$lib/utils/ogImagePrefetch";

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
// Non-reactive set of articleIds already requested (or in-flight). Content-based,
// not count-based: a mark-as-read remove + replacement append can leave the
// visible count unchanged, so keying backfill off the count drops the
// replacement's image. Keying off articleId never misses a new card.
//
// Plain `Set`, not `SvelteSet`, on purpose: the effect below both reads and
// writes it, and a reactive one would re-run the effect on its own writes.
const requestedOgImageArticleIds = new Set<string>();

onMount(async () => {
	try {
		feedSources = await listSubscriptionsClient();
	} catch (e) {
		console.error("Failed to load feed sources:", e);
	}
});

// Batch prefetch OG images for visible feeds that have articleId but no ogImageProxyUrl.
//
// The result is written to the shared overlay keyed by article, never onto the
// feed objects. Two reasons, both of which used to lose the picture silently:
// the array read here is a snapshot taken before the request, so the feed it
// would find afterwards may be one the grid has already replaced; and a feed
// that came from SSR — or through a `$derived` that rebuilds its items — is not
// a `$state` proxy, so assigning a field on it updates nothing on screen. A
// keyed overlay is read by whichever object the card is rendering, so the
// answer lands wherever the card ended up.
$effect(() => {
	if (!feedGridApi) return;
	const visibleFeeds = feedGridApi.getVisibleFeeds();

	const articleIds = selectOgImagePrefetchIds(
		visibleFeeds,
		requestedOgImageArticleIds,
		ogImageOverlay.has,
	);
	if (articleIds.length === 0) return;

	// Mark in-flight before awaiting so re-runs don't re-request the same ids.
	for (const id of articleIds) requestedOgImageArticleIds.add(id);

	batchPrefetchImagesClient(articleIds)
		.then((results) => {
			for (const result of results) {
				ogImageOverlay.resolve(result.articleId, result.proxyUrl);
			}
		})
		.catch((err) => {
			console.error("Failed to prefetch OG images:", err);
			// Allow retry on the next change.
			for (const id of articleIds) requestedOgImageArticleIds.delete(id);
		});
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
		selectedFeedUrl = feeds[currentIndex - 1]!.normalizedUrl;
	}
}

function handleNext() {
	if (!feedGridApi || currentIndex >= totalCount - 1) return;
	const feeds = feedGridApi.getVisibleFeeds();
	if (feeds[currentIndex + 1]) {
		selectedFeedUrl = feeds[currentIndex + 1]!.normalizedUrl;
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
{:else}
	<!--
		Mobile: a vertically scrolling, two-column thumbnail gallery.

		This used to be a hand-off notice pointing at the swipe screen, which made
		the route a dead end on the device most of the reading happens on. Swipe is
		a one-at-a-time judgment surface; this is the scanning surface — the reader
		sees ~6 covers per screen and drills into one, which is a different job and
		now has its own layout rather than a redirect.

		Two columns rather than a list of small thumbnails: at half a phone's width
		the image is still large enough to recognise, which is the threshold below
		which list thumbnails stop earning their place.
	-->
	<header class="gallery-header">
		<h1 class="gallery-title">Visual Preview</h1>
		<p class="gallery-subtitle">Browse feeds by their cover</p>
	</header>

	<FeedGrid
		onSelectFeed={handleSelectFeed}
		unreadOnly={filters.unreadOnly}
		sortBy={filters.sortBy}
		excludedFeedLinkIds={filters.excludedFeedLinkIds}
		onReady={handleFeedGridReady}
		gridClass="grid grid-cols-2 gap-2"
		gridTestId="gallery-grid"
	>
		{#snippet cardRenderer({ feed, isRead, onSelect }: { feed: RenderFeed; index: number; isRead: boolean; onSelect: (feed: RenderFeed) => void })}
			<MobileGalleryTile {feed} {isRead} {onSelect} />
		{/snippet}
	</FeedGrid>
{/if}

<!--
	Shared by both layouts. Opening the article in a modal rather than navigating
	is what keeps the reader's place: the gallery grows by infinite scroll, so a
	round-trip to a detail route and back would drop them at the top of a list
	that no longer contains the items they had already scrolled past.
-->
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
	onMarkAsRead={handleMarkAsReadInModal}
	isMarkingAsRead={isMarkingAsRead || isProcessingMarkAsRead}
/>

<style>
	.gallery-header {
		padding: 0.75rem 0 1rem;
	}

	.gallery-title {
		font-family: var(--font-display);
		font-size: 1.35rem;
		font-weight: 700;
		color: var(--text-primary);
		margin: 0;
	}

	.gallery-subtitle {
		font-family: var(--font-body);
		font-size: 0.8rem;
		color: var(--text-muted);
		margin: 0.2rem 0 0;
	}
</style>
