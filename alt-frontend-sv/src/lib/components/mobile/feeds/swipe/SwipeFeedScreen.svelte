<script lang="ts">
import { onMount } from "svelte";
import { fade } from "svelte/transition";
import { browser } from "$app/environment";
import {
	getFeedsWithCursorClient,
	getReadFeedsWithCursorClient,
	updateFeedReadStatusClient,
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

	try {
		const res = await getFeedsWithCursorClient(cursor ?? undefined, PAGE_SIZE);
		const newFeeds = res.data.map((f: SanitizedFeed) => toRenderFeed(f));

		// Filter out read feeds
		const filtered = newFeeds.filter(
			(f) => !readFeeds.has(canonicalize(f.link)),
		);

		feeds = [...feeds, ...filtered];
		cursor = res.next_cursor;
		hasMore = res.next_cursor !== null;
	} catch (err) {
		console.error("Error loading feeds:", err);
		error = "Failed to load feeds";
	} finally {
		isLoading = false;
		isInitialLoading = false;
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
    <div class="relative w-full max-w-[30rem] h-[95dvh] px-4">
      <!-- Next card (background) -->
      {#if nextFeed}
        <div
          class="absolute inset-0 px-4 scale-95 opacity-50 pointer-events-none"
          aria-hidden="true"
        >
          <div
            class="w-full h-full bg-[var(--alt-glass)] border-2 border-[var(--alt-glass-border)] rounded-2xl"
          ></div>
        </div>
      {/if}

      <!-- Active card -->
      {#key activeFeed.id}
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
