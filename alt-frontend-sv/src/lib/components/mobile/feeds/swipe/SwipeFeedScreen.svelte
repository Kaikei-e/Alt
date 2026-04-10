<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import {
	batchPrefetchImagesClient,
	getFeedContentOnTheFlyClient,
	getFeedsWithCursorClient,
	getReadFeedsWithCursorClient,
	listSubscriptionsClient,
	updateFeedReadStatusClient,
} from "$lib/api/client";
import type { ConnectFeedSource } from "$lib/connect/feeds";
import type { RenderFeed, SanitizedFeed } from "$lib/schema/feed";
import { toRenderFeed } from "$lib/schema/feed";
import { articlePrefetcher } from "$lib/utils/articlePrefetcher";
import { canonicalize } from "$lib/utils/feed";
import SwipeFeedCard from "./SwipeFeedCard.svelte";
import VisualPreviewCard from "./VisualPreviewCard.svelte";
import SwipeFilterSortSheet from "./SwipeFilterSortSheet.svelte";
import SwipeLoadingOverlay from "./SwipeLoadingOverlay.svelte";

interface Props {
	initialFeeds?: RenderFeed[];
	initialNextCursor?: string | null;
	initialArticleContent?: string | null;
	initialOgImageUrl?: string | null;
	mode?: "default" | "visual-preview";
}

const {
	initialFeeds = [],
	initialNextCursor,
	initialArticleContent,
	initialOgImageUrl,
	mode = "default",
}: Props = $props();

const isVisualPreview = $derived(mode === "visual-preview");
const prefetchAhead = $derived(isVisualPreview ? 10 : 2);

const PAGE_SIZE = 20;

// State
// svelte-ignore state_referenced_locally
let feeds = $state<RenderFeed[]>([...(initialFeeds ?? [])]);
// svelte-ignore state_referenced_locally
let cursor = $state<string | null>(initialNextCursor ?? null);
// svelte-ignore state_referenced_locally
let hasMore = $state(initialFeeds.length > 0 ? !!initialNextCursor : true);
let isLoading = $state(false);
let error = $state<string | null>(null);
let readFeeds = $state<Set<string>>(new Set());
let activeIndex = $state(0);
// svelte-ignore state_referenced_locally
let isInitialLoading = $state((initialFeeds ?? []).length === 0);
let liveRegionMessage = $state("");

// Filter & sort state
let excludedFeedLinkIds = $state<string[]>([]);
let feedSources = $state<ConnectFeedSource[]>([]);
let sortOrder = $state<"newest" | "oldest">("newest");

// Reactive OGP image update version counter
let ogImageVersion = $state(0);

// Derived
const activeFeed = $derived(feeds[activeIndex]);
const nextFeed = $derived(feeds[activeIndex + 1]);

// Initialize read feeds
onMount(() => {
	if (!browser) return;

	const initializeReadFeeds = async () => {
		try {
			const res = await getReadFeedsWithCursorClient(undefined, 32);
			const links = new Set<string>();
			if (res?.data) {
				res.data.forEach((feed: SanitizedFeed) => {
					links.add(canonicalize(feed.link));
				});
			}
			readFeeds = links;
		} catch (err) {
			console.warn("Failed to initialize read feeds:", err);
		}
	};

	if ("requestIdleCallback" in window) {
		window.requestIdleCallback(() => void initializeReadFeeds());
	} else {
		setTimeout(() => void initializeReadFeeds(), 100);
	}
});

// Initial load if needed
onMount(() => {
	if (feeds.length === 0 && hasMore) {
		void loadMore();
	} else {
		isInitialLoading = false;
	}
});

// Load feed sources for filter
onMount(async () => {
	try {
		feedSources = await listSubscriptionsClient();
	} catch (e) {
		console.warn("Failed to load feed sources:", e);
	}
});

// Wire OGP image callback for reactivity
onMount(() => {
	articlePrefetcher.setOnOgImageFetched(() => {
		ogImageVersion++;
	});
	return () => articlePrefetcher.setOnOgImageFetched(null);
});

// Wire articleId callback to trigger batch image prefetch in visual-preview mode
onMount(() => {
	if (!isVisualPreview) return;

	articlePrefetcher.setOnArticleIdCached((feedUrl, articleId) => {
		batchPrefetchImagesClient([articleId])
			.then((results) => {
				for (const info of results) {
					if (info.articleId === articleId && info.proxyUrl) {
						articlePrefetcher.seedCache(
							feedUrl,
							articlePrefetcher.getCachedContent(feedUrl) || "",
							articleId,
							null,
							info.proxyUrl,
						);
					}
				}
			})
			.catch((err) => {
				console.warn(
					"[SwipeFeedScreen] Batch image prefetch for new articleId failed:",
					err,
				);
			});
	});
	return () => articlePrefetcher.setOnArticleIdCached(null);
});

// Re-evaluate OGP image when activeFeed changes or cache updates
const currentOgImage = $derived.by(() => {
	void ogImageVersion;
	if (!activeFeed) return null;
	const cached = articlePrefetcher.getCachedOgImage(activeFeed.normalizedUrl);
	if (cached) return cached;
	if (activeFeed.ogImageProxyUrl) return activeFeed.ogImageProxyUrl;
	if (activeIndex === 0 && initialOgImageUrl) return initialOgImageUrl;
	return null;
});

// Prefetch next feeds
$effect(() => {
	if (feeds.length > 0) {
		articlePrefetcher.triggerPrefetch(feeds, activeIndex, prefetchAhead);
	}
});

// Load more when running low
$effect(() => {
	if (
		!isLoading &&
		hasMore &&
		feeds.length - activeIndex < 5 &&
		feeds.length > 0
	) {
		void loadMore();
	}
});

async function loadMore() {
	if (isLoading || !hasMore) return;
	isLoading = true;
	error = null;

	const isFirstLoad = feeds.length === 0;

	try {
		if (isFirstLoad) {
			const feedsPromise = getFeedsWithCursorClient(
				cursor ?? undefined,
				PAGE_SIZE,
				excludedFeedLinkIds.length > 0 ? excludedFeedLinkIds : undefined,
			);

			const res = await feedsPromise;
			const newFeeds = res.data.map((f: SanitizedFeed) => toRenderFeed(f));

			const filtered = newFeeds.filter(
				(f) => !readFeeds.has(canonicalize(f.link)),
			);

			feeds = [...feeds, ...filtered];
			cursor = res.next_cursor;
			hasMore = res.next_cursor !== null;
			isInitialLoading = false;

			if (filtered.length > 0) {
				const firstFeed = filtered[0];
				const cacheKey = firstFeed.normalizedUrl;

				if (!articlePrefetcher.getCachedContent(cacheKey)) {
					getFeedContentOnTheFlyClient(cacheKey)
						.then((articleRes) => {
							if (articleRes.content) {
								articlePrefetcher.seedCache(
									cacheKey,
									articleRes.content,
									articleRes.article_id || "",
									articleRes.og_image_url || null,
								);
							}
						})
						.catch((err) => {
							console.warn(
								`[SwipeFeedScreen] Failed to fetch article content for first feed: ${cacheKey}`,
								err,
							);
						});
				}
			}
		} else {
			const res = await getFeedsWithCursorClient(
				cursor ?? undefined,
				PAGE_SIZE,
				excludedFeedLinkIds.length > 0 ? excludedFeedLinkIds : undefined,
			);
			const newFeeds = res.data.map((f: SanitizedFeed) => toRenderFeed(f));

			const filtered = newFeeds.filter(
				(f) => !readFeeds.has(canonicalize(f.link)),
			);

			feeds = [...feeds, ...filtered];
			cursor = res.next_cursor;
			hasMore = res.next_cursor !== null;

			if (isVisualPreview && filtered.length > 0) {
				triggerBatchImagePrefetch(filtered);
			}
		}
	} catch (err) {
		console.warn("Error loading feeds:", err);
		error = "Failed to load feeds";
	} finally {
		isLoading = false;
		if (!isFirstLoad) {
			isInitialLoading = false;
		}
	}
}

function triggerBatchImagePrefetch(newFeeds: RenderFeed[]) {
	const articleIds: string[] = [];
	for (const feed of newFeeds.slice(0, 10)) {
		if (feed.articleId) {
			articleIds.push(feed.articleId);
		}
	}
	if (articleIds.length === 0) return;

	batchPrefetchImagesClient(articleIds)
		.then((results) => {
			for (const info of results) {
				for (const feed of newFeeds) {
					if (feed.articleId === info.articleId && info.proxyUrl) {
						articlePrefetcher.seedCache(
							feed.normalizedUrl,
							articlePrefetcher.getCachedContent(feed.normalizedUrl) || "",
							info.articleId,
							null,
							info.proxyUrl,
						);
						break;
					}
				}
			}
		})
		.catch((err) => {
			console.warn("[SwipeFeedScreen] Batch image prefetch failed:", err);
		});
}

function handleExclude(ids: string[]) {
	excludedFeedLinkIds = ids;
	resetAndReload();
}

function handleClearExclusion() {
	excludedFeedLinkIds = [];
	resetAndReload();
}

function resetAndReload() {
	feeds = [];
	activeIndex = 0;
	cursor = null;
	hasMore = true;
	isInitialLoading = true;
	void loadMore();
}

async function handleDismiss(_direction: number) {
	if (!activeFeed) return;

	const currentLink = canonicalize(activeFeed.link);

	readFeeds.add(currentLink);
	readFeeds = new Set(readFeeds);

	liveRegionMessage = "Feed marked as read";
	setTimeout(() => {
		liveRegionMessage = "";
	}, 1000);

	articlePrefetcher.markAsDismissed(currentLink);

	activeIndex++;

	try {
		await updateFeedReadStatusClient(currentLink);
	} catch (err) {
		console.warn("Failed to mark as read:", currentLink, err);
	}
}

function getCachedContent(url: string) {
	return articlePrefetcher.getCachedContent(url);
}

function getCachedArticleId(url: string) {
	return articlePrefetcher.getCachedArticleId(url);
}

function handleArticleIdResolved(feedLink: string, articleId: string) {
	feeds = feeds.map((f) => (f.link === feedLink ? { ...f, articleId } : f));
}
</script>

<div class="swipe-screen">
  <!-- Live Region -->
  <div
    aria-live="polite"
    aria-atomic="true"
    class="sr-only"
  >
    {liveRegionMessage}
  </div>

  {#if isInitialLoading}
    <div class="initial-loading">
      <div class="loading-dot" aria-hidden="true"></div>
      <p class="loading-text">Loading feeds...</p>
    </div>
  {:else if error && feeds.length === 0}
    <div class="empty-state">
      <p class="error-title">Error loading feeds</p>
      <p class="error-message">{error}</p>
      <button
        type="button"
        class="action-btn"
        onclick={() => void loadMore()}
      >
        Retry
      </button>
    </div>
  {:else if activeFeed}
    <div class="card-container">
      <!-- Next card (background) -->
      {#if nextFeed}
        <div
          class="background-card"
          aria-hidden="true"
        ></div>
      {/if}

      <!-- Active card -->
      {#key activeFeed.id}
        {#if isVisualPreview}
          <VisualPreviewCard
            feed={activeFeed}
            statusMessage={liveRegionMessage}
            onDismiss={handleDismiss}
            thumbnailUrl={currentOgImage}
            {getCachedContent}
            {getCachedArticleId}
            isBusy={isLoading}
            initialArticleContent={activeIndex === 0
              ? initialArticleContent
              : undefined}
            onArticleIdResolved={handleArticleIdResolved}
            isLcp={activeIndex === 0}
          />
        {:else}
          <SwipeFeedCard
            feed={activeFeed}
            statusMessage={liveRegionMessage}
            onDismiss={handleDismiss}
            {getCachedContent}
            {getCachedArticleId}
            isBusy={isLoading}
            initialArticleContent={activeIndex === 0
              ? initialArticleContent
              : undefined}
            onArticleIdResolved={handleArticleIdResolved}
          />
        {/if}
      {/key}
    </div>
  {:else}
    <div class="empty-state">
      <p class="empty-text">No more feeds</p>
      <button
        type="button"
        class="action-btn"
        onclick={() => window.location.reload()}
      >
        Refresh
      </button>
    </div>
  {/if}

  <SwipeLoadingOverlay isVisible={isLoading} />
  <SwipeFilterSortSheet
    sources={feedSources}
    {excludedFeedLinkIds}
    {sortOrder}
    onExclude={handleExclude}
    onClearExclusion={handleClearExclusion}
    onSortChange={(order) => { sortOrder = order; }}
  />
</div>

<style>
  .swipe-screen {
    min-height: 100dvh;
    position: relative;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    overflow: hidden;
    background: var(--surface-bg);
  }

  .sr-only {
    position: absolute;
    left: -10000px;
    width: 1px;
    height: 1px;
    overflow: hidden;
  }

  .card-container {
    position: relative;
    width: 100%;
    max-width: 30rem;
    height: 95dvh;
    padding: 0 0.5rem;
    overflow: hidden;
  }

  .background-card {
    position: absolute;
    width: 100%;
    height: 95dvh;
    max-width: calc(100% - 1rem);
    background: var(--surface-bg);
    border: 1px solid var(--surface-border);
    opacity: 0.35;
    pointer-events: none;
  }

  /* ── Loading ── */
  .initial-loading {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 0.75rem;
  }

  .loading-dot {
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
    color: var(--alt-slate);
    margin: 0;
  }

  /* ── Empty / Error ── */
  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 1.5rem;
    text-align: center;
    gap: 0.5rem;
  }

  .empty-text {
    font-family: var(--font-body);
    font-size: 0.9rem;
    color: var(--alt-slate);
    margin: 0 0 0.75rem;
  }

  .error-title {
    font-family: var(--font-body);
    font-size: 0.9rem;
    font-weight: 600;
    color: var(--alt-terracotta);
    margin: 0;
  }

  .error-message {
    font-family: var(--font-body);
    font-size: 0.82rem;
    color: var(--alt-slate);
    margin: 0 0 0.75rem;
  }

  .action-btn {
    font-family: var(--font-body);
    font-size: 0.75rem;
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--alt-charcoal);
    background: transparent;
    border: 1.5px solid var(--alt-charcoal);
    padding: 0.5rem 1.5rem;
    min-height: 44px;
    cursor: pointer;
    transition: background 0.15s, color 0.15s;
  }

  .action-btn:active {
    background: var(--alt-charcoal);
    color: var(--surface-bg);
  }

  @keyframes pulse {
    0%, 100% { opacity: 0.3; }
    50% { opacity: 1; }
  }

  @media (prefers-reduced-motion: reduce) {
    .loading-dot {
      animation: none;
      opacity: 0.6;
    }
  }
</style>
