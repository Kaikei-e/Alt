<script lang="ts">
import {
	BookOpen,
	Loader,
	Sparkles,
	SquareArrowOutUpRight,
	Star,
} from "@lucide/svelte";
import { onMount, tick } from "svelte";
import { Spring } from "svelte/motion";
import { fade } from "svelte/transition";
import { type SwipeDirection, swipe } from "$lib/actions/swipe";
import {
	getFeedContentOnTheFlyClient,
	registerFavoriteFeedClient,
	summarizeArticleClient,
} from "$lib/api/client";
import { Button } from "$lib/components/ui/button";
import {
	createClientTransport,
	streamSummarizeWithAbortAdapter,
} from "$lib/connect";
import type { RenderFeed } from "$lib/schema/feed";
import { simulateTypewriterEffect } from "$lib/utils/streamingRenderer";

interface Props {
	feed: RenderFeed;
	statusMessage: string | null;
	onDismiss: (direction: number) => Promise<void> | void;
	thumbnailUrl: string | null;
	getCachedContent?: (feedUrl: string) => string | null;
	getCachedArticleId?: (feedUrl: string) => string | null;
	isBusy?: boolean;
	initialArticleContent?: string | null;
	onArticleIdResolved?: (feedLink: string, articleId: string) => void;
}

const {
	feed,
	statusMessage,
	onDismiss,
	thumbnailUrl,
	getCachedContent,
	getCachedArticleId,
	isBusy = false,
	initialArticleContent,
	onArticleIdResolved,
}: Props = $props();

// State
let isAISummaryRequested = $state(false);
let aiSummary = $state<string | null>(null);
let summaryError = $state<string | null>(null);
let isSummarizing = $state(false);

let isContentExpanded = $state(false);
let fullContent = $state<string | null>(null);
let isLoadingContent = $state(false);
let contentError = $state<string | null>(null);

let summaryAbortController = $state<AbortController | null>(null);

let isFavoriting = $state(false);
let isFavorited = $state(false);
let favoriteError = $state<string | null>(null);

// Thumbnail state
let imgLoaded = $state(false);
let imgError = $state(false);

// Swipe state with Spring
const SWIPE_THRESHOLD = 60;
let x = new Spring(0, { stiffness: 0.18, damping: 0.85 });
let isDragging = $state(false);
let hasSwiped = $state(false);
let swipeElement: HTMLDivElement | null = $state(null);
let scrollAreaRef: HTMLDivElement | null = $state(null);

// Derived styles
const cardStyle = $derived.by(() => {
	const translate = x.current;
	const opacity = Math.max(0.4, 1 - Math.abs(translate) / 500);

	return [
		`transform: translate3d(${translate}px, 0, 0)`,
		`opacity: ${opacity}`,
	].join("; ");
});

// Derived
const sanitizedFullContent = $derived(fullContent);
const hasDescription = $derived(Boolean(feed.description));
const publishedLabel = $derived.by(() => {
	if (feed.created_at) {
		try {
			return new Date(feed.created_at).toLocaleString();
		} catch {
			// Fallback
		}
	}
	if (!feed.published) return null;
	try {
		return new Date(feed.published).toLocaleString();
	} catch {
		return feed.published;
	}
});

// Auto-fetch content
onMount(() => {
	if (initialArticleContent) {
		fullContent = initialArticleContent;
	}

	const cached = getCachedContent?.(feed.normalizedUrl);
	if (cached) {
		fullContent = cached;
		const cachedArticleId = getCachedArticleId?.(feed.normalizedUrl);
		if (cachedArticleId && onArticleIdResolved) {
			onArticleIdResolved(feed.link, cachedArticleId);
		}
	} else if (!fullContent) {
		getFeedContentOnTheFlyClient(feed.normalizedUrl)
			.then((res) => {
				if (res.content) {
					fullContent = res.content;
				}
				if (res.article_id && onArticleIdResolved) {
					onArticleIdResolved(feed.link, res.article_id);
				}
			})
			.catch((err) => {
				console.error("[VisualPreviewCard] Error auto-fetching content:", err);
			});
	}
});

// Set up swipe event listeners reactively
$effect(() => {
	if (!swipeElement) return;

	const swipeHandler = (event: Event) => {
		if (hasSwiped) return;
		handleSwipe(event as CustomEvent<{ direction: SwipeDirection }>);
	};

	const swipeMoveHandler = (event: Event) => {
		const moveEvent = event as CustomEvent<{
			deltaX: number;
			deltaY: number;
		}>;
		const { deltaX, deltaY } = moveEvent.detail;

		if (Math.abs(deltaX) > Math.abs(deltaY)) {
			isDragging = true;
			x.set(deltaX, { instant: true });
		}
	};

	const swipeEndHandler = (_event: Event) => {
		x.target = 0;
		isDragging = false;
	};

	swipeElement.addEventListener("swipe", swipeHandler);
	swipeElement.addEventListener("swipe:move", swipeMoveHandler);
	swipeElement.addEventListener("swipe:end", swipeEndHandler);

	return () => {
		swipeElement?.removeEventListener("swipe", swipeHandler);
		swipeElement?.removeEventListener("swipe:move", swipeMoveHandler);
		swipeElement?.removeEventListener("swipe:end", swipeEndHandler);
	};
});

// Abort in-flight summary stream when component is destroyed
$effect(() => {
	return () => {
		summaryAbortController?.abort();
	};
});

async function handleToggleContent() {
	if (!isContentExpanded && !fullContent) {
		const cached = getCachedContent?.(feed.normalizedUrl);
		if (cached) {
			fullContent = cached;
			isContentExpanded = true;
			return;
		}

		isLoadingContent = true;
		contentError = null;

		try {
			const res = await getFeedContentOnTheFlyClient(feed.normalizedUrl);
			if (res.content) {
				fullContent = res.content;
			} else {
				contentError = "Could not fetch article content";
			}
		} catch (err) {
			console.error("Error fetching content:", err);
			contentError = "Could not fetch article content";
		} finally {
			isLoadingContent = false;
		}
	}
	isContentExpanded = !isContentExpanded;
}

function handleGenerateAISummary() {
	isAISummaryRequested = true;
	isSummarizing = true;
	summaryError = null;
	aiSummary = "";

	summaryAbortController?.abort();

	const transport = createClientTransport();
	summaryAbortController = streamSummarizeWithAbortAdapter(
		transport,
		{
			feedUrl: feed.link,
			title: feed.title,
		},
		(chunk) => {
			aiSummary = (aiSummary || "") + chunk;
		},
		{
			tick,
			typewriter: true,
			typewriterDelay: 10,
			onChunk: (
				chunkCount,
				_chunkSize,
				_decodedLength,
				_totalLength,
				_preview,
			) => {
				if (chunkCount === 1) {
					isSummarizing = false;
				}
			},
			onComplete: (_totalLength, _chunkCount) => {},
		},
		(_result) => {
			summaryAbortController = null;
		},
		async (err) => {
			summaryAbortController = null;

			const errorMessage = err instanceof Error ? err.message : String(err);

			if (errorMessage.includes("abort") || errorMessage.includes("cancel")) {
				return;
			}

			const isAuthError =
				errorMessage.includes("403") ||
				errorMessage.includes("401") ||
				errorMessage.includes("Forbidden") ||
				errorMessage.includes("Authentication") ||
				errorMessage.includes("unauthenticated");

			if (isAuthError) {
				summaryError =
					"Authentication failed. Please refresh the page and try again.";
				isSummarizing = false;
				return;
			}

			if (aiSummary && aiSummary.length > 0) {
				summaryError = "Stream interrupted. Summary may be incomplete.";
				isSummarizing = false;
				return;
			}

			try {
				const res = await summarizeArticleClient(feed.link);
				if (res.success && res.summary) {
					isSummarizing = false;
					const typewriter = simulateTypewriterEffect(
						(char) => {
							aiSummary = (aiSummary || "") + char;
						},
						{ tick, delay: 10 },
					);
					await typewriter.add(res.summary);
				} else {
					isSummarizing = false;
					summaryError = "Failed to generate the summary";
				}
			} catch (legacyErr) {
				isSummarizing = false;
				summaryError = "Failed to generate the summary. Please try again.";
			}
		},
	);
}

async function handleFavorite() {
	if (isFavoriting || isFavorited) return;
	isFavoriting = true;
	favoriteError = null;
	try {
		await registerFavoriteFeedClient(feed.link);
		isFavorited = true;
	} catch (err) {
		console.error("[VisualPreviewCard] Failed to favorite feed:", err);
		favoriteError = "Failed";
		setTimeout(() => {
			favoriteError = null;
		}, 3000);
	} finally {
		isFavoriting = false;
	}
}

async function handleSwipe(event: CustomEvent<{ direction: SwipeDirection }>) {
	const dir = event.detail.direction;
	if (dir !== "left" && dir !== "right") return;

	hasSwiped = true;
	isDragging = false;

	const width = swipeElement?.clientWidth ?? window.innerWidth;
	const target = dir === "left" ? -width : width;

	await x.set(target, { preserveMomentum: 120 });
	await onDismiss(dir === "left" ? -1 : 1);

	hasSwiped = false;
	await x.set(0, { instant: true });
}

function handleImgLoad() {
	imgLoaded = true;
}

function handleImgError() {
	imgError = true;
}
</script>

<div
  bind:this={swipeElement}
  class="absolute w-full h-[95dvh] bg-[var(--alt-glass)] text-[var(--alt-text-primary)] border-2 border-[var(--alt-glass-border)] shadow-[0_12px_40px_rgba(0,0,0,0.3),0_0_0_1px_rgba(255,255,255,0.1)] rounded-2xl backdrop-blur-[20px] select-none overflow-hidden"
  use:swipe={{ threshold: SWIPE_THRESHOLD, restraint: 120, allowedTime: 500 }}
  aria-busy={isBusy}
  data-testid="visual-preview-card"
  style={`${cardStyle}; touch-action: none; max-width: calc(100% - 1rem);`}
>
  <div class="flex flex-col gap-0 h-full">
    <!-- Thumbnail Area (~35% height) -->
    <div class="relative w-full" style="height: 35%; min-height: 150px;">
      {#if thumbnailUrl && !imgError}
        <img
          src={thumbnailUrl}
          alt=""
          loading="lazy"
          decoding="async"
          data-testid="thumbnail-image"
          class="absolute inset-0 w-full h-full object-cover transition-opacity duration-300 {imgLoaded ? 'opacity-100' : 'opacity-0'}"
          onload={handleImgLoad}
          onerror={handleImgError}
        />
        <!-- Shimmer placeholder while loading -->
        {#if !imgLoaded}
          <div class="absolute inset-0 shimmer-bg" data-testid="thumbnail-shimmer"></div>
        {/if}
      {:else}
        <!-- No-image fallback: decorative gradient -->
        <div
          class="absolute inset-0 fallback-gradient"
          data-testid="thumbnail-fallback"
        ></div>
      {/if}
      <!-- Gradient overlay from bottom -->
      <div class="absolute inset-0 pointer-events-none" style="background: linear-gradient(to top, var(--alt-glass) 0%, transparent 50%);"></div>
    </div>

    <!-- Content Area -->
    <div class="flex flex-col flex-1 min-h-0 px-4 -mt-4 relative z-[1]">
      <!-- Header info -->
      <div class="mb-2">
        <p
          class="text-sm mb-1 uppercase tracking-[0.08em] font-semibold"
          style="color: black;"
        >
          Swipe to mark as read
        </p>
        <div class="flex items-center gap-2">
          <a
            href={feed.link}
            target="_blank"
            rel="noopener noreferrer"
            aria-label="Open article in new tab"
            class="flex items-center gap-2 text-[var(--alt-text-primary)] min-w-0 hover:opacity-80 transition-opacity"
          >
            <div class="shrink-0">
              <SquareArrowOutUpRight
                class="text-[var(--alt-primary)]"
                size={18}
              />
            </div>
            <h2
              class="text-lg font-bold flex-1 break-words whitespace-normal min-w-0 line-clamp-2"
            >
              {feed.title}
            </h2>
          </a>
        </div>
        {#if publishedLabel}
          <p class="text-[var(--alt-text-secondary)] text-xs mt-1">
            {publishedLabel}
          </p>
        {/if}
      </div>

      <!-- Scrollable content area -->
      <div
        bind:this={scrollAreaRef}
        style="touch-action: pan-y; overflow-x: hidden;"
        class="flex-1 overflow-y-auto overflow-x-hidden bg-transparent scroll-smooth overscroll-contain scrollbar-thin select-none"
        data-testid="unified-scroll-area"
      >
        {#if hasDescription && !isAISummaryRequested}
          <div class="mb-3 overflow-x-hidden" transition:fade>
            <div
              class="text-sm text-[var(--alt-text-primary)] leading-[1.6] break-words overflow-wrap-anywhere line-clamp-3"
            >
              {@html feed.description}
            </div>
          </div>
        {/if}

        {#if isAISummaryRequested}
          <div
            class="px-3 pt-2 pb-3 border-t mb-3 overflow-x-hidden"
            data-testid="ai-summary-section"
            transition:fade
          >
            <p
              class="text-xs text-[var(--alt-text-secondary)] font-semibold mb-1 uppercase tracking-[0.18em]"
            >
              {isSummarizing ? "SUMMARY" : "AI SUMMARY"}
            </p>
            {#if isSummarizing}
              <div class="flex flex-col items-center gap-2 py-3">
                <Loader
                  class="animate-spin text-[var(--alt-primary)]"
                  size={18}
                />
                <span class="text-[var(--alt-text-secondary)] text-xs"
                  >Now summarizing ....</span
                >
              </div>
            {:else if summaryError}
              <p
                class="text-[var(--alt-text-secondary)] text-xs text-center py-3"
              >
                {summaryError}
              </p>
            {:else if aiSummary}
              <p
                class="text-sm text-[var(--alt-text-primary)] leading-relaxed whitespace-pre-wrap break-words overflow-wrap-anywhere"
              >
                {aiSummary}
              </p>
            {/if}
          </div>
        {/if}

        {#if isContentExpanded}
          <div
            class="mb-3 p-3 bg-[rgba(255,255,255,0.03)] rounded-xl border border-[var(--alt-glass-border)] overflow-x-hidden"
            data-testid="content-section"
            transition:fade
          >
            <p
              class="text-xs text-[var(--alt-text-secondary)] font-bold mb-1 uppercase tracking-widest"
            >
              Full Article
            </p>
            {#if isLoadingContent}
              <div class="flex justify-center py-3 gap-2">
                <Loader
                  class="animate-spin text-[var(--alt-primary)]"
                  size={18}
                />
                <span class="text-[var(--alt-text-secondary)] text-xs"
                  >Loading article content...</span
                >
              </div>
            {:else if contentError}
              <p class="text-[var(--alt-text-secondary)] text-xs text-center">
                {contentError}
              </p>
            {:else if sanitizedFullContent}
              <div
                class="text-sm text-[var(--alt-text-primary)] leading-[1.7] prose prose-invert max-w-none break-words overflow-wrap-anywhere overflow-x-hidden"
              >
                {@html sanitizedFullContent}
              </div>
            {/if}
          </div>
        {/if}
      </div>
    </div>

    <!-- Footer -->
    <div
      class="relative z-[2] bg-[rgba(0,0,0,0.25)] backdrop-blur-[20px] border-t border-[var(--alt-glass-border)] px-3 py-3 rounded-b-2xl shadow-[0_-4px_20px_rgba(0,0,0,0.3)]"
      data-testid="action-footer"
    >
      <div class="flex gap-2 w-full justify-between">
        <Button
          onclick={handleToggleContent}
          size="sm"
          class="flex-1 rounded-xl font-bold text-white hover:brightness-110 active:translate-y-0 transition-all duration-200 shadow-lg {isContentExpanded
            ? 'bg-[slate-200] shadow-[var(--alt-secondary)]/50'
            : 'bg-[slate-200] shadow-[var(--alt-primary)]/50'}"
          disabled={isLoadingContent}
        >
          <BookOpen class="mr-2 h-4 w-4" />
          {isLoadingContent
            ? "Loading..."
            : isContentExpanded
              ? "Hide"
              : "Article"}
        </Button>
        <Button
          onclick={handleFavorite}
          size="sm"
          class="rounded-xl font-bold text-white hover:brightness-110 active:translate-y-0 transition-all duration-200 shadow-lg {isFavorited
            ? 'bg-[slate-200] shadow-[var(--alt-secondary)]/50'
            : favoriteError
              ? 'bg-red-500/80 shadow-red-500/50'
              : 'bg-[slate-200] shadow-[var(--alt-primary)]/50'}"
          disabled={isFavoriting || isFavorited}
          aria-label={isFavorited ? "Favorited" : isFavoriting ? "Saving favorite" : favoriteError ? "Favorite failed, tap to retry" : "Favorite"}
        >
          {#if isFavoriting}
            <Loader class="h-5 w-5 animate-spin" />
          {:else}
            <Star class="h-5 w-5" fill={isFavorited ? "currentColor" : "none"} stroke={favoriteError ? "red" : "currentColor"} />
          {/if}
        </Button>
        <Button
          onclick={handleGenerateAISummary}
          size="sm"
          class="flex-1 rounded-xl font-bold text-white hover:brightness-110 active:translate-y-0 transition-all duration-200 shadow-lg {isAISummaryRequested
            ? 'bg-[slate-200] shadow-[var(--alt-secondary)]/50'
            : 'bg-[slate-200] shadow-[var(--alt-primary)]/50'}"
          disabled={isSummarizing}
        >
          <Sparkles class="mr-2 h-4 w-4" />
          {isSummarizing
            ? "Summarizing..."
            : isAISummaryRequested
              ? "Summary"
              : "Summary"}
        </Button>
      </div>
    </div>
  </div>
</div>

<style>
  .scrollbar-thin::-webkit-scrollbar {
    width: 4px;
  }
  .scrollbar-thin::-webkit-scrollbar-track {
    background: transparent;
    border-radius: 2px;
  }
  .scrollbar-thin::-webkit-scrollbar-thumb {
    background: rgba(255, 255, 255, 0.2);
    border-radius: 2px;
  }
  .scrollbar-thin::-webkit-scrollbar-thumb:hover {
    background: rgba(255, 255, 255, 0.3);
  }

  [data-testid="unified-scroll-area"],
  [data-testid="unified-scroll-area"] * {
    -webkit-user-select: none;
    -moz-user-select: none;
    -ms-user-select: none;
    user-select: none;
  }

  .shimmer-bg {
    background: linear-gradient(
      90deg,
      rgba(255, 255, 255, 0.05) 25%,
      rgba(255, 255, 255, 0.12) 50%,
      rgba(255, 255, 255, 0.05) 75%
    );
    background-size: 200% 100%;
    animation: shimmer 1.5s infinite;
  }

  @keyframes shimmer {
    0% {
      background-position: 200% 0;
    }
    100% {
      background-position: -200% 0;
    }
  }

  .fallback-gradient {
    background: linear-gradient(
      135deg,
      rgba(var(--alt-primary-rgb, 99, 102, 241), 0.15) 0%,
      rgba(var(--alt-secondary-rgb, 168, 85, 247), 0.10) 50%,
      rgba(var(--alt-primary-rgb, 99, 102, 241), 0.05) 100%
    );
  }
</style>
