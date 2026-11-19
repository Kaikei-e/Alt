<script lang="ts">
import { onMount } from "svelte";
import { batchPrefetchImagesClient } from "#lib/api/client/articles.js";
import {
	listSubscriptionsClient,
	updateFeedReadStatusClient,
} from "#lib/api/client/feeds.js";
import FeedFilters from "#lib/components/desktop/feeds/FeedFilters.svelte";
import FeedGrid from "#lib/components/desktop/feeds/FeedGrid.svelte";
import type { FeedGridApi } from "#lib/components/desktop/feeds/feed-grid-types.js";
import VisualFeedCard from "#lib/components/desktop/feeds/VisualFeedCard.svelte";
import PageHeader from "#lib/components/desktop/layout/PageHeader.svelte";
import Toast from "#lib/components/knowledge-home/Toast.svelte";
import MobileGalleryTile from "#lib/components/mobile/feeds/gallery/MobileGalleryTile.svelte";
import type { ConnectFeedSource } from "#lib/connect/feeds.js";
import { MARK_AS_READ_FAILED_MESSAGE } from "#lib/feeds/mark-as-read-feedback.js";
import { renderFeedToReaderItem } from "#lib/reader/mappers.js";
import {
	closeReaderOverlay,
	openReaderOverlay,
	replaceReaderOverlayItems,
} from "#lib/reader/reader-overlay.svelte.js";
import type { ReaderArticleItem } from "#lib/reader/types.js";
import {
	ReadingSurface,
	recordSurfaceViewed,
} from "#lib/reading/instrumentation.js";
import type { RenderFeed } from "#lib/schema/feed.js";
import { ogImageOverlay } from "#lib/stores/ogImageOverlay.svelte.js";
import { useToastStore } from "#lib/stores/toast.svelte.js";
import { isDesktop } from "#lib/stores/viewport.svelte.js";
import { selectOgImagePrefetchIds } from "#lib/utils/ogImagePrefetch.js";
import { createRequestQueue } from "#lib/utils/requestQueue.js";

interface PageData {
	initialFeeds?: RenderFeed[];
	error?: string;
}

const { data }: { data: PageData } = $props();
const toast = useToastStore();

let filters = $state({
	unreadOnly: false,
	sortBy: "date_desc",
	excludedFeedLinkIds: [] as string[],
});
let feedGridApi = $state<FeedGridApi | null>(null);
let feedSources = $state<ConnectFeedSource[]>([]);
let isProcessingMarkAsRead = $state(false);
let isMarkingAsRead = $state(false);
let liveRegionMessage = $state("");
let liveRegionTimer: ReturnType<typeof setTimeout> | null = null;

function announce(message: string, holdMs: number) {
	if (liveRegionTimer) clearTimeout(liveRegionTimer);
	liveRegionMessage = message;
	liveRegionTimer = setTimeout(() => {
		liveRegionMessage = "";
		liveRegionTimer = null;
	}, holdMs);
}

const requestedOgImageArticleIds = new Set<string>();

onMount(async () => {
	recordSurfaceViewed(ReadingSurface.GALLERY);
	try {
		feedSources = await listSubscriptionsClient();
	} catch (e) {
		console.error("Failed to load feed sources:", e);
	}
});

const articlePrefetchQueue = createRequestQueue({ concurrency: 4 });

$effect(() => {
	if (!feedGridApi) return;
	const visibleFeeds = feedGridApi.getVisibleFeeds();

	const articleIds = selectOgImagePrefetchIds(
		visibleFeeds,
		requestedOgImageArticleIds,
		ogImageOverlay.has,
	);
	if (articleIds.length === 0) return;

	for (const id of articleIds) requestedOgImageArticleIds.add(id);

	for (const id of articleIds) {
		articlePrefetchQueue
			.add(() => batchPrefetchImagesClient([id]))
			.then((results) => {
				for (const result of results) {
					ogImageOverlay.resolve(result.articleId, result.proxyUrl);
				}
			})
			.catch((err) => {
				console.error(`Failed to prefetch OG image for article ${id}:`, err);
				requestedOgImageArticleIds.delete(id);
			});
	}
});

function handleSelectFeed(feed: RenderFeed, index: number, _total: number) {
	const visibleFeeds = feedGridApi?.getVisibleFeeds() ?? [feed];
	const resolvedIndex = visibleFeeds.findIndex(
		(f) => f.normalizedUrl === feed.normalizedUrl,
	);
	openReaderOverlay({
		surface: ReadingSurface.GALLERY,
		items: visibleFeeds.map(renderFeedToReaderItem),
		currentIndex: resolvedIndex >= 0 ? resolvedIndex : index,
		onMarkAsRead: handleMarkAsReadInOverlay,
		isMarkingAsRead: () => isMarkingAsRead || isProcessingMarkAsRead,
	});
}

function handleFilterChange(newFilters: {
	unreadOnly: boolean;
	sortBy: string;
	excludedFeedLinkIds: string[];
}) {
	filters = newFilters;
}

async function handleMarkAsReadInOverlay(item: ReaderArticleItem) {
	if (!item.url || isMarkingAsRead) return;

	try {
		isMarkingAsRead = true;
		await updateFeedReadStatusClient(item.url);
		handleMarkAsRead(item.url);
		announce("Feed marked as read", 1000);
	} catch (error) {
		console.error("Failed to mark feed as read:", error);
		announce(MARK_AS_READ_FAILED_MESSAGE, 4000);
		toast.push(MARK_AS_READ_FAILED_MESSAGE, "error", 4000);
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
	const remainingFeeds = feedGridApi.getVisibleFeeds();

	if (totalCount === 0 || isLastFeed || nextFeedUrl === null) {
		closeReaderOverlay();
	} else {
		const nextIdx = remainingFeeds.findIndex(
			(f: RenderFeed) => f.normalizedUrl === nextFeedUrl,
		);
		replaceReaderOverlayItems(
			remainingFeeds.map(renderFeedToReaderItem),
			nextIdx >= 0 ? nextIdx : 0,
		);
	}

	feedGridApi.fetchReplacementFeed();
	isProcessingMarkAsRead = false;
}

const GALLERY_GRID_CLASS =
	"grid grid-cols-2 gap-2 md:gap-5 lg:grid-cols-3 xl:grid-cols-4";

function handleFeedGridReady(api: FeedGridApi) {
	feedGridApi = api;
}
</script>

<svelte:head>
	<title>Visual Preview - Alt</title>
</svelte:head>

<Toast items={toast.items} onDismiss={toast.remove} />

<div
	aria-live="assertive"
	aria-atomic="true"
	class="sr-only"
>
	{liveRegionMessage}
</div>

{#if isDesktop()}
	<PageHeader title="Visual Preview" description="Browse feeds with image thumbnails" />

	<FeedFilters
		unreadOnly={filters.unreadOnly}
		sortBy={filters.sortBy}
		excludedFeedLinkIds={filters.excludedFeedLinkIds}
		{feedSources}
		onFilterChange={handleFilterChange}
	/>
{:else}
	<header class="gallery-header">
		<h1 class="gallery-title">Visual Preview</h1>
		<p class="gallery-subtitle">Browse feeds by their cover</p>
	</header>
{/if}

<FeedGrid
	onSelectFeed={handleSelectFeed}
	unreadOnly={filters.unreadOnly}
	sortBy={filters.sortBy}
	excludedFeedLinkIds={filters.excludedFeedLinkIds}
	onReady={handleFeedGridReady}
	gridClass={GALLERY_GRID_CLASS}
	gridTestId="gallery-grid"
>
	{#snippet cardRenderer({ feed, isRead, onSelect }: { feed: RenderFeed; index: number; isRead: boolean; onSelect: (feed: RenderFeed) => void })}
		{#if isDesktop()}
			<VisualFeedCard {feed} {isRead} {onSelect} />
		{:else}
			<MobileGalleryTile {feed} {isRead} {onSelect} />
		{/if}
	{/snippet}
</FeedGrid>

<style>
	.sr-only {
		position: absolute;
		left: -10000px;
		width: 1px;
		height: 1px;
		overflow: hidden;
	}
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
