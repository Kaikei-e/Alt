<script lang="ts">
import { onMount } from "svelte";
import { fade, fly } from "svelte/transition";
import { browser } from "$app/environment";
import {
	getFeedsWithCursorClient,
	getReadFeedsWithCursorClient,
	updateFeedReadStatusClient,
	getFeedContentOnTheFlyClient,
} from "$lib/api/client";
import { Button } from "$lib/components/ui/button";
import type { RenderFeed, SanitizedFeed } from "$lib/schema/feed";
import { toRenderFeed } from "$lib/schema/feed";
import { articlePrefetcher } from "$lib/utils/articlePrefetcher";
import { canonicalize } from "$lib/utils/feed";
import FloatingMenu from "./FloatingMenu.svelte";
import SwipeFeedCard from "./SwipeFeedCard.svelte";
import SwipeLoadingOverlay from "./SwipeLoadingOverlay.svelte";

interface Props {
	initialFeeds?: RenderFeed[];
	initialNextCursor?: string | null;
	initialArticleContent?: string | null;
}

const {
	initialFeeds = [],
	initialNextCursor,
	initialArticleContent,
}: Props = $props();

const PAGE_SIZE = 20;

// State
let feeds = $state<RenderFeed[]>([]);
let cursor = $state<string | null>(null);
let hasMore = $state(false);
let isLoading = $state(false);
let error = $state<string | null>(null);
let readFeeds = $state<Set<string>>(new Set());
let activeIndex = $state(0);
let isInitialLoading = $state(true);
let liveRegionMessage = $state("");

// Initialize state from props
$effect(() => {
	feeds = [...(initialFeeds ?? [])];
	cursor = initialNextCursor ?? null;
	hasMore = !!initialNextCursor;
	isInitialLoading = (initialFeeds ?? []).length === 0;
});

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
			console.error("Failed to initialize read feeds:", err);
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

// Prefetch next feeds
$effect(() => {
	if (feeds.length > 0) {
		articlePrefetcher.triggerPrefetch(feeds, activeIndex);
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
				const feedUrl = firstFeed.link;

				// キャッシュに既に存在するかチェック
				if (!articlePrefetcher.getCachedContent(feedUrl)) {
					// バックグラウンドでarticle取得を開始
					getFeedContentOnTheFlyClient(feedUrl)
						.then((articleRes) => {
							if (articleRes.content) {
								// articlePrefetcherのキャッシュに直接保存
								// contentCacheはprivateなので、既存のprefetchContentロジックを参考に
								// 簡易的に直接キャッシュに設定（型安全性のため、anyを使用）
								const cache = (articlePrefetcher as any).contentCache as Map<
									string,
									string | "loading"
								>;
								cache.set(feedUrl, articleRes.content);

								// キャッシュサイズ制限のチェック
								if (cache.size > 5) {
									const entries = Array.from(cache.keys());
									const oldestKey = entries[0];
									cache.delete(oldestKey);
								}
							}
						})
						.catch((err) => {
							console.warn(
								`[SwipeFeedScreen] Failed to fetch article content for first feed: ${feedUrl}`,
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
			);
			const newFeeds = res.data.map((f: SanitizedFeed) => toRenderFeed(f));

			// Filter out read feeds
			const filtered = newFeeds.filter(
				(f) => !readFeeds.has(canonicalize(f.link)),
			);

			feeds = [...feeds, ...filtered];
			cursor = res.next_cursor;
			hasMore = res.next_cursor !== null;
		}
	} catch (err) {
		console.error("Error loading feeds:", err);
		error = "Failed to load feeds";
	} finally {
		isLoading = false;
		if (!isFirstLoad) {
			isInitialLoading = false;
		}
	}
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

	// Server update
	try {
		await updateFeedReadStatusClient(currentLink);
	} catch (err) {
		console.error("Failed to mark as read:", err);
		// Rollback if needed, but for now we keep moving forward
	}
}

function getCachedContent(url: string) {
	return articlePrefetcher.getCachedContent(url);
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
        <div
          in:fly={{ x: 0, y: 0, duration: 300 }}
          out:fly={{ x: 0, y: 0, duration: 300 }}
        >
          <SwipeFeedCard
            feed={activeFeed}
            statusMessage={liveRegionMessage}
            onDismiss={handleDismiss}
            {getCachedContent}
            isBusy={isLoading}
            initialArticleContent={activeIndex === 0
              ? initialArticleContent
              : undefined}
          />
        </div>
      {/key}
    </div>
  {:else}
    <div class="flex flex-col items-center justify-center p-6 text-center">
      <p class="text-[var(--alt-text-secondary)] mb-4">No more feeds</p>
      <Button onclick={() => window.location.reload()}>Refresh</Button>
    </div>
  {/if}

  <SwipeLoadingOverlay isVisible={isLoading} />
  <FloatingMenu />
</div>
