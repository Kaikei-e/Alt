<script lang="ts">
import { getContext, onMount } from "svelte";
import {
	listSubscriptionsClient,
	updateFeedReadStatusClient,
} from "$lib/api/client/feeds";
import FeedDetailModal from "$lib/components/desktop/feeds/FeedDetailModal.svelte";
import FeedFilters from "$lib/components/desktop/feeds/FeedFilters.svelte";
import FeedGrid from "$lib/components/desktop/feeds/FeedGrid.svelte";
import type { FeedGridApi } from "$lib/components/desktop/feeds/feed-grid-types";
import FeedsClient from "$lib/components/mobile/FeedsClient.svelte";
import MobileFeedExcludeFilter from "$lib/components/mobile/feeds/MobileFeedExcludeFilter.svelte";
import type { ConnectFeedSource } from "$lib/connect/feeds";
import type { RenderFeed } from "$lib/schema/feed";
import { useViewport } from "$lib/stores/viewport.svelte";
import {
	CONNECTION_RECOVERY_KEY,
	type ConnectionRecoveryStore,
} from "$lib/stores/connection-recovery.svelte";

interface PageData {
	initialFeeds?: RenderFeed[];
	error?: string;
}

const { data }: { data: PageData } = $props();
const { isDesktop } = useViewport();
const connectionRecovery = getContext<ConnectionRecoveryStore | undefined>(
	CONNECTION_RECOVERY_KEY,
);

let mobileExcludedFeedLinkIds = $state<string[]>([]);
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
let revealed = $state(false);

const dateStr = new Date().toLocaleDateString("en-US", {
	weekday: "long",
	year: "numeric",
	month: "long",
	day: "numeric",
});

async function loadFeedSources() {
	try {
		feedSources = await listSubscriptionsClient();
	} catch (e) {
		console.error("Failed to load feed sources:", e);
	}
}

onMount(() => {
	requestAnimationFrame(() => {
		revealed = true;
	});
	loadFeedSources();
});

$effect(() => {
	if (!connectionRecovery) return;
	const unsubscribe = connectionRecovery.subscribe((info) => {
		console.info("[Feeds] Connection recovery triggered:", info.reason);
		loadFeedSources();
		feedGridApi?.refresh();
	});
	return unsubscribe;
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
	<title>Feeds - Alt</title>
</svelte:head>

{#if isDesktop}
	<div class="wire-page" class:revealed>
		<header class="wire-header">
			<span class="wire-date">{dateStr}</span>
			<h1 class="wire-title">Feeds</h1>
			<div class="wire-rule" aria-hidden="true"></div>
		</header>

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
		/>

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
				<button
					onclick={handleMarkAsReadInModal}
					disabled={isMarkingAsRead || isProcessingMarkAsRead}
					class="mark-read-btn"
				>
					{isMarkingAsRead ? "Marking\u2026" : "Mark as Read"}
				</button>
			{/snippet}
		</FeedDetailModal>
	</div>
{:else}
	<div style="background: var(--app-bg);" class="h-[100dvh] overflow-hidden flex flex-col">
		<header class="mobile-wire-header">
			<span class="wire-date">{dateStr}</span>
			<h1 class="wire-title-mobile">Feeds</h1>
			<div class="wire-rule" aria-hidden="true"></div>
		</header>
		<MobileFeedExcludeFilter
			sources={feedSources}
			excludedFeedLinkIds={mobileExcludedFeedLinkIds}
			onExclude={(ids: string[]) => (mobileExcludedFeedLinkIds = ids)}
			onClearExclusion={() => (mobileExcludedFeedLinkIds = [])}
		/>
		<div class="flex-1 min-h-0 flex flex-col">
			<FeedsClient
				initialFeeds={data.initialFeeds || []}
				excludeFeedLinkIds={mobileExcludedFeedLinkIds}
			/>
		</div>
	</div>
{/if}

<style>
	.wire-page {
		opacity: 0;
		transform: translateY(6px);
		transition:
			opacity 0.4s ease,
			transform 0.4s ease;
	}

	.wire-page.revealed {
		opacity: 1;
		transform: translateY(0);
	}

	.wire-header {
		padding: 1.5rem 0 0;
	}

	.mobile-wire-header {
		padding: 1rem 1.25rem 0;
	}

	.wire-date {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-ash);
		letter-spacing: 0.06em;
	}

	.wire-title {
		font-family: var(--font-display);
		font-size: 1.6rem;
		font-weight: 800;
		color: var(--alt-charcoal);
		letter-spacing: -0.01em;
		margin: 0.15rem 0 0;
		line-height: 1.2;
	}

	.wire-title-mobile {
		font-family: var(--font-display);
		font-size: 1.3rem;
		font-weight: 700;
		color: var(--alt-charcoal);
		margin: 0.1rem 0 0;
		line-height: 1.2;
	}

	.wire-rule {
		height: 1px;
		background: var(--surface-border);
		margin-top: 0.75rem;
	}

	.mark-read-btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		padding: 0.4rem 1rem;
		min-height: 2.25rem;
		font-family: var(--font-body);
		font-size: 0.75rem;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--alt-charcoal);
		background: transparent;
		border: 1.5px solid var(--alt-charcoal);
		cursor: pointer;
		transition:
			background 0.15s,
			color 0.15s;
	}

	.mark-read-btn:hover:not(:disabled) {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	.mark-read-btn:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}

	@media (prefers-reduced-motion: reduce) {
		.wire-page {
			opacity: 1;
			transform: none;
			transition: none;
		}
	}
</style>
