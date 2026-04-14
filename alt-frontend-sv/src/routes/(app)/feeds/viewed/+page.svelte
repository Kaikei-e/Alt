<script lang="ts">
import { onMount } from "svelte";
import { useViewport } from "$lib/stores/viewport.svelte";

import { getReadFeedsWithCursorClient } from "$lib/api/client/feeds";
import type { RenderFeed } from "$lib/schema/feed";
import FeedGrid from "$lib/components/desktop/feeds/FeedGrid.svelte";
import FeedDetailModal from "$lib/components/desktop/feeds/FeedDetailModal.svelte";
import type { FeedGridApi } from "$lib/components/desktop/feeds/feed-grid-types";

import ViewedFeedsClient from "$lib/components/mobile/ViewedFeedsClient.svelte";

const { isDesktop } = useViewport();

const dateStr = new Date().toLocaleDateString("en-US", {
	weekday: "long",
	year: "numeric",
	month: "long",
	day: "numeric",
});

let revealed = $state(false);

onMount(() => {
	requestAnimationFrame(() => {
		revealed = true;
	});
});

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

function handleFeedGridReady(api: FeedGridApi) {
	feedGridApi = api;
}
</script>

<svelte:head>
	<title>The Morgue Desk - Alt</title>
</svelte:head>

{#if isDesktop}
	<div class="morgue-page" class:revealed data-role="morgue-desk-page">
		<header class="morgue-header">
			<span class="morgue-date">{dateStr}</span>
			<h1 class="morgue-title">The Morgue Desk</h1>
			<div class="morgue-rule" aria-hidden="true"></div>
		</header>

		<FeedGrid
			onSelectFeed={handleSelectFeed}
			onReady={handleFeedGridReady}
			fetchFn={getReadFeedsWithCursorClient}
			emptyText="Nothing filed yet"
			loadingText="Retrieving filed clippings"
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
		/>
	</div>
{:else}
	<div style="background: var(--app-bg);" class="h-[100dvh] overflow-hidden flex flex-col">
		<header class="mobile-morgue-header">
			<span class="morgue-date">{dateStr}</span>
			<h1 class="morgue-title-mobile">The Morgue Desk</h1>
			<div class="morgue-rule" aria-hidden="true"></div>
		</header>
		<div class="flex-1 min-h-0 flex flex-col">
			<ViewedFeedsClient />
		</div>
	</div>
{/if}

<style>
	.morgue-page {
		opacity: 0;
		transform: translateY(6px);
		transition:
			opacity 0.4s ease,
			transform 0.4s ease;
	}

	.morgue-page.revealed {
		opacity: 1;
		transform: translateY(0);
	}

	.morgue-header {
		padding: 1.5rem 0 0;
	}

	.mobile-morgue-header {
		padding: 1rem 1.25rem 0;
	}

	.morgue-date {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-ash);
		letter-spacing: 0.06em;
	}

	.morgue-title {
		font-family: var(--font-display);
		font-size: 1.6rem;
		font-weight: 800;
		color: var(--alt-charcoal);
		letter-spacing: -0.01em;
		margin: 0.15rem 0 0;
		line-height: 1.2;
	}

	.morgue-title-mobile {
		font-family: var(--font-display);
		font-size: 1.3rem;
		font-weight: 700;
		color: var(--alt-charcoal);
		margin: 0.1rem 0 0;
		line-height: 1.2;
	}

	.morgue-rule {
		height: 1px;
		background: var(--surface-border);
		margin-top: 0.75rem;
	}

	@media (prefers-reduced-motion: reduce) {
		.morgue-page {
			opacity: 1;
			transform: none;
			transition: none;
		}
	}
</style>
