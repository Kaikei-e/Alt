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
import { Button } from "$lib/components/ui/button";
import type { ConnectFeedSource } from "$lib/connect/feeds";
import type { RenderFeed, SanitizedFeed } from "$lib/schema/feed";
import { toRenderFeed } from "$lib/schema/feed";
import { articlePrefetcher } from "$lib/utils/articlePrefetcher";
import { canonicalize } from "$lib/utils/feed";
import FloatingMenu from "./FloatingMenu.svelte";
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

// State - initialize directly from props (not via $effect to avoid circular deps)
let feeds = $state<RenderFeed[]>([...(initialFeeds ?? [])]);
let cursor = $state<string | null>(initialNextCursor ?? null);
// When SSR fails, initialFeeds is empty and initialNextCursor is null.
// In that case, assume hasMore=true so loadMore() fires as client-side fallback.
let hasMore = $state(initialFeeds.length > 0 ? !!initialNextCursor : true);
let isLoading = $state(false);
let error = $state<string | null>(null);
let readFeeds = $state<Set<string>>(new Set());
let activeIndex = $state(0);
let isInitialLoading = $state((initialFeeds ?? []).length === 0);
let liveRegionMessage = $state("");

// Filter & sort state
let excludedSourceId = $state<string | null>(null);
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

// Wire articleId callback to trigger batch image prefetch in visual-preview mode.
// When prefetchContent resolves an article_id, batch-prefetch its OGP proxy image.
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
				console.warn("[SwipeFeedScreen] Batch image prefetch for new articleId failed:", err);
			});
	});
	return () => articlePrefetcher.setOnArticleIdCached(null);
});

// Re-evaluate OGP image when activeFeed changes or cache updates
// Use SSR-provided initialOgImageUrl for first card before prefetcher cache is populated
const currentOgImage = $derived.by(() => {
	void ogImageVersion;
	if (!activeFeed) return null;
	const cached = articlePrefetcher.getCachedOgImage(activeFeed.normalizedUrl);
	if (cached) return cached;
	// Use pre-fetched OGP proxy URL from feed collection (no extra HTTP needed)
	if (activeFeed.ogImageProxyUrl) return activeFeed.ogImageProxyUrl;
	// SSR fallback: use server-provided URL for the first card
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
		// 最初のフィード読み込み時は、feedとarticleを並列で取得
		if (isFirstLoad) {
			const feedsPromise = getFeedsWithCursorClient(
				cursor ?? undefined,
				PAGE_SIZE,
				excludedSourceId ?? undefined,
			);

			// feed取得を開始（article取得は後で開始）
			const res = await feedsPromise;
			const newFeeds = res.data.map((f: SanitizedFeed) => toRenderFeed(f));

			// Filter out read feeds
			const filtered = newFeeds.filter(
				(f) => !readFeeds.has(canonicalize(f.link)),
			);

			// feedが取得できたらすぐに表示
			feeds = [...feeds, ...filtered];
			cursor = res.next_cursor;
			hasMore = res.next_cursor !== null;
			isInitialLoading = false;

			// 最初のフィードのarticleをバックグラウンドで取得
			if (filtered.length > 0) {
				const firstFeed = filtered[0];
				// Use normalizedUrl for consistent cache key
				const cacheKey = firstFeed.normalizedUrl;

				// キャッシュに既に存在するかチェック
				if (!articlePrefetcher.getCachedContent(cacheKey)) {
					// バックグラウンドでarticle取得を開始（normalizedUrl使用）
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
			// 通常の読み込み（2回目以降）
			const res = await getFeedsWithCursorClient(
				cursor ?? undefined,
				PAGE_SIZE,
				excludedSourceId ?? undefined,
			);
			const newFeeds = res.data.map((f: SanitizedFeed) => toRenderFeed(f));

			// Filter out read feeds
			const filtered = newFeeds.filter(
				(f) => !readFeeds.has(canonicalize(f.link)),
			);

			feeds = [...feeds, ...filtered];
			cursor = res.next_cursor;
			hasMore = res.next_cursor !== null;

			// Batch prefetch OGP images for Visual Preview mode
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

/**
 * Trigger batch image prefetch for visual preview mode.
 * Collects articleIds from prefetcher cache and calls BatchPrefetchImages.
 */
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
				// Find the feed that matches this articleId and update ogImage cache
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

function handleExclude(id: string) {
	excludedSourceId = id;
	resetAndReload();
}

function handleClearExclusion() {
	excludedSourceId = null;
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

	// Optimistic update
	readFeeds.add(currentLink);
	readFeeds = new Set(readFeeds); // Trigger reactivity

	liveRegionMessage = "Feed marked as read";
	setTimeout(() => {
		liveRegionMessage = "";
	}, 1000);

	articlePrefetcher.markAsDismissed(currentLink);

	// Move to next
	activeIndex++;

	// Server update - always try to mark as read
	// Backend uses feed_id (not article_id), so this works even for 404 articles
	try {
		await updateFeedReadStatusClient(currentLink);
	} catch (err) {
		// Log but don't block - feed might not exist in DB yet
		console.warn("Failed to mark as read:", currentLink, err);
	}
}

function getCachedContent(url: string) {
	return articlePrefetcher.getCachedContent(url);
}

function getCachedArticleId(url: string) {
	return articlePrefetcher.getCachedArticleId(url);
}

/**
 * Handle articleId resolution when content fetch creates an article.
 * Updates the feed data so UI reflects that the article is now saved.
 */
function handleArticleIdResolved(feedLink: string, articleId: string) {
	feeds = feeds.map((f) => (f.link === feedLink ? { ...f, articleId } : f));
}
</script>

<div
  class="min-h-[100dvh] relative flex flex-col items-center justify-center overflow-hidden bg-[var(--app-bg)]"
>
  <!-- Live Region -->
  <div
    aria-live="polite"
    aria-atomic="true"
    class="absolute left-[-10000px] w-px h-px overflow-hidden"
  >
    {liveRegionMessage}
  </div>

  {#if isInitialLoading}
    <div class="flex flex-col items-center justify-center gap-4">
      <div
        class="w-12 h-12 border-4 border-[var(--alt-primary)] border-t-transparent rounded-full animate-spin"
      ></div>
      <p class="text-[var(--alt-text-secondary)]">Loading feeds...</p>
    </div>
  {:else if error && feeds.length === 0}
    <div class="flex flex-col items-center justify-center p-6 text-center">
      <p class="text-[var(--destructive)] font-semibold mb-2">
        Error loading feeds
      </p>
      <p class="text-sm text-[var(--alt-text-secondary)] mb-4">{error}</p>
      <Button onclick={() => void loadMore()}>Retry</Button>
    </div>
  {:else if activeFeed}
    <div class="relative w-full max-w-[30rem] h-[95dvh] px-2 sm:px-4 overflow-hidden">
      <!-- Next card (background) -->
      {#if nextFeed}
        <div
          class="absolute w-full h-[95dvh] bg-[var(--alt-glass)] border-2 border-[var(--alt-glass-border)] rounded-2xl p-4 opacity-50 pointer-events-none"
          aria-hidden="true"
          style="max-width: calc(100% - 1rem)"
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
    <div class="flex flex-col items-center justify-center p-6 text-center">
      <p class="text-[var(--alt-text-secondary)] mb-4">No more feeds</p>
      <Button onclick={() => window.location.reload()}>Refresh</Button>
    </div>
  {/if}

  <SwipeLoadingOverlay isVisible={isLoading} />
  <SwipeFilterSortSheet
    sources={feedSources}
    {excludedSourceId}
    {sortOrder}
    onExclude={handleExclude}
    onClearExclusion={handleClearExclusion}
    onSortChange={(order) => { sortOrder = order; }}
  />
  <FloatingMenu />
</div>
