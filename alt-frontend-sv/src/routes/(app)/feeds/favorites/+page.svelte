<script lang="ts">
import { onMount } from "svelte";
import { useViewport } from "$lib/stores/viewport.svelte";

import FeedDetailModal from "$lib/components/desktop/feeds/FeedDetailModal.svelte";
import FeedGrid from "$lib/components/desktop/feeds/FeedGrid.svelte";
import type { FeedGridApi } from "$lib/components/desktop/feeds/feed-grid-types";

import ClippingsEntry from "$lib/components/mobile/ClippingsEntry.svelte";

import { getFavoriteFeedsWithCursorClient } from "$lib/api/client/feeds";
import { removeFavoriteFeedClient } from "$lib/api/client";
import { infiniteScroll } from "$lib/actions/infinite-scroll";

import type { RenderFeed } from "$lib/schema/feed";

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

// --- Mobile state ---
let mobileFeeds = $state<RenderFeed[]>([]);
let mobileIsLoading = $state(true);
let mobileIsFetchingNext = $state(false);
let mobileError = $state<Error | null>(null);
let mobileNextCursor = $state<string | undefined>(undefined);
let mobileHasMore = $state(true);

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

async function handleRemoveFavorite(feedUrl: string) {
	try {
		await removeFavoriteFeedClient(feedUrl);
		mobileFeeds = mobileFeeds.filter((f) => f.normalizedUrl !== feedUrl);
	} catch (err) {
		console.error("Failed to remove favorite:", err);
	}
}

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
	<title>The Clippings File - Alt</title>
</svelte:head>

{#if isDesktop}
	<div class="clippings-page" class:revealed data-role="clippings-file-page">
		<header class="clippings-header">
			<span class="clippings-date">{dateStr}</span>
			<h1 class="clippings-title">The Clippings File</h1>
			<p class="clippings-subtitle">Your curated collection</p>
			<div class="clippings-rule" aria-hidden="true"></div>
		</header>

		<FeedGrid
			onSelectFeed={handleSelectFeed}
			onReady={handleFeedGridReady}
			fetchFn={getFavoriteFeedsWithCursorClient}
			emptyText="No clippings yet"
			loadingText="Retrieving your clippings"
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
	<div
		class="h-screen overflow-hidden flex flex-col"
		style="background: var(--app-bg);"
		data-role="clippings-file-page"
	>
		<header class="mobile-clippings-header">
			<span class="clippings-date">{dateStr}</span>
			<h1 class="clippings-title-mobile">The Clippings File</h1>
			<p class="clippings-subtitle-mobile">Your curated collection</p>
			<div class="clippings-rule" aria-hidden="true"></div>
		</header>

		{#if mobileIsLoading}
			<div class="loading-state">
				<span class="loading-pulse"></span>
				<span class="loading-text">Retrieving your clippings&hellip;</span>
			</div>
		{:else if mobileError}
			<div class="error-stripe" role="alert">
				<p class="error-stripe-title">Error loading clippings</p>
				<p>{mobileError.message}</p>
			</div>
		{:else if mobileFeeds.length === 0}
			<div class="empty-state">
				<div class="empty-ornament" aria-hidden="true">&#9670;</div>
				<h2 class="empty-heading">No Clippings Yet</h2>
				<p class="empty-body">
					Star articles from the wire to add them to your clippings file.
				</p>
			</div>
		{:else}
			<div
				class="flex-1 min-h-0 overflow-y-auto"
				style="padding: 0 1.25rem 1.25rem;"
			>
				<div class="clippings-list" data-role="clippings-feed-list">
					{#each mobileFeeds as feed, index (feed.id)}
						<div class="clipping-item" style="--stagger: {index};">
							<ClippingsEntry
								{feed}
								onRemove={handleRemoveFavorite}
							/>
						</div>
					{/each}
				</div>

				<div
					use:infiniteScroll={{
						callback: loadMoreMobile,
						disabled: mobileIsFetchingNext || !mobileHasMore,
						threshold: 0.1,
						rootMargin: "0px 0px 200px 0px",
					}}
					class="load-more"
				>
					{#if mobileIsFetchingNext}
						<div class="loading-state loading-state--compact">
							<span class="loading-pulse"></span>
							<span class="loading-text">Loading more&hellip;</span>
						</div>
					{:else if mobileHasMore}
						<p class="scroll-hint">Scroll for more</p>
					{:else}
						<p class="scroll-hint">End of clippings</p>
					{/if}
				</div>
			</div>
		{/if}
	</div>
{/if}

<style>
	.clippings-page {
		opacity: 0;
		transform: translateY(6px);
		transition:
			opacity 0.4s ease,
			transform 0.4s ease;
	}

	.clippings-page.revealed {
		opacity: 1;
		transform: translateY(0);
	}

	.clippings-header {
		padding: 1.5rem 0 0;
	}

	.mobile-clippings-header {
		padding: 1rem 1.25rem 0;
	}

	.clippings-date {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-ash);
		letter-spacing: 0.06em;
	}

	.clippings-title {
		font-family: var(--font-display);
		font-size: 1.6rem;
		font-weight: 800;
		color: var(--alt-charcoal);
		letter-spacing: -0.01em;
		margin: 0.15rem 0 0;
		line-height: 1.2;
	}

	.clippings-title-mobile {
		font-family: var(--font-display);
		font-size: 1.3rem;
		font-weight: 700;
		color: var(--alt-charcoal);
		margin: 0.1rem 0 0;
		line-height: 1.2;
	}

	.clippings-subtitle {
		font-family: var(--font-body);
		font-size: 0.85rem;
		font-style: italic;
		color: var(--alt-slate);
		margin: 0.2rem 0 0;
	}

	.clippings-subtitle-mobile {
		font-family: var(--font-body);
		font-size: 0.8rem;
		font-style: italic;
		color: var(--alt-slate);
		margin: 0.1rem 0 0;
	}

	.clippings-rule {
		height: 1px;
		background: var(--surface-border);
		margin-top: 0.75rem;
	}

	.clippings-list {
		display: flex;
		flex-direction: column;
	}

	.clipping-item {
		opacity: 0;
		animation: entry-in 0.3s ease forwards;
		animation-delay: calc(var(--stagger) * 40ms);
	}

	.load-more {
		padding: 1.5rem 0;
	}

	.loading-state {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 2rem 0;
		justify-content: center;
	}

	.loading-state--compact {
		padding: 1rem 0;
	}

	.loading-pulse {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--alt-ash);
		animation: pulse 1.2s ease-in-out infinite;
	}

	.loading-text {
		font-family: var(--font-body);
		font-size: 0.85rem;
		font-style: italic;
		color: var(--alt-ash);
	}

	.error-stripe {
		padding: 0.75rem 1rem;
		margin: 0 1.25rem;
		border-left: 3px solid var(--alt-terracotta);
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-terracotta);
	}

	.error-stripe-title {
		font-weight: 600;
		margin: 0 0 0.25rem;
	}

	.error-stripe p {
		margin: 0;
	}

	.empty-state {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		min-height: 70vh;
		padding: 1.5rem;
		text-align: center;
	}

	.empty-ornament {
		font-size: 1.5rem;
		color: var(--surface-border);
		margin-bottom: 1rem;
	}

	.empty-heading {
		font-family: var(--font-display);
		font-size: 1.4rem;
		font-weight: 700;
		color: var(--alt-charcoal);
		margin: 0 0 0.5rem;
	}

	.empty-body {
		font-family: var(--font-body);
		font-size: 0.9rem;
		line-height: 1.6;
		color: var(--alt-slate);
		max-width: 320px;
		margin: 0;
	}

	.scroll-hint {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-ash);
		text-align: center;
		margin: 0;
	}

	@keyframes pulse {
		0%, 100% { opacity: 0.3; }
		50% { opacity: 1; }
	}

	@keyframes entry-in {
		to { opacity: 1; }
	}

	@media (prefers-reduced-motion: reduce) {
		.clippings-page {
			opacity: 1;
			transform: none;
			transition: none;
		}
		.clipping-item {
			animation: none;
			opacity: 1;
		}
		.loading-pulse {
			animation: none;
			opacity: 1;
		}
	}
</style>
