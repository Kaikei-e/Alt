<script lang="ts">
import {
	BookOpen,
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
import {
	createClientTransport,
	streamSummarizeWithAbortAdapter,
} from "$lib/connect";
import type { RenderFeed } from "$lib/schema/feed";
import { sanitizeHtml } from "$lib/utils/sanitizeHtml";
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
	isLcp?: boolean;
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
	isLcp = false,
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

// Reset error/loaded state when thumbnailUrl changes
$effect(() => {
	void thumbnailUrl;
	imgError = false;
	imgLoaded = false;
});

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
const sanitizedFullContent = $derived(
	fullContent ? sanitizeHtml(fullContent) : null,
);
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
				console.warn("[VisualPreviewCard] Error auto-fetching content:", err);
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
				contentError = "Source content unavailable.";
			}
		} catch (err) {
			console.warn("Error fetching content:", err);
			contentError = "Source content unavailable.";
		} finally {
			isLoadingContent = false;
		}
	}
	isContentExpanded = !isContentExpanded;
}

function handleGenerateAISummary() {
	const targetFeedLink = feed.link;

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
			if (feed.link !== targetFeedLink) return;
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
  class="swipe-card"
  use:swipe={{ threshold: SWIPE_THRESHOLD, restraint: 120, allowedTime: 500 }}
  aria-busy={isBusy}
  data-testid="visual-preview-card"
  style="{cardStyle}; touch-action: none;"
>
  <div class="card-inner">
    <!-- Thumbnail Area -->
    <div class="thumbnail-area">
      {#if thumbnailUrl && !imgError}
        <img
          src={thumbnailUrl}
          alt=""
          loading={isLcp ? "eager" : "lazy"}
          fetchpriority={isLcp ? "high" : undefined}
          decoding="async"
          data-testid="thumbnail-image"
          class="thumbnail-img"
          class:thumbnail-img--loaded={imgLoaded}
          onload={handleImgLoad}
          onerror={handleImgError}
        />
        {#if !imgLoaded}
          <div class="thumbnail-shimmer" data-testid="thumbnail-shimmer"></div>
        {/if}
      {:else}
        <div class="thumbnail-fallback" data-testid="thumbnail-fallback">
          <span class="thumbnail-fallback-text">No preview</span>
        </div>
      {/if}
      <div class="thumbnail-overlay"></div>
    </div>

    <!-- Content Area -->
    <div class="content-area">
      <!-- Header -->
      <div class="card-header-compact">
        <p class="card-label">Swipe to mark as read</p>
        <div class="flex items-center gap-2">
          <a
            href={feed.link}
            target="_blank"
            rel="noopener noreferrer"
            aria-label="Open article"
            class="card-title-link"
          >
            <div class="flex-shrink-0">
              <SquareArrowOutUpRight
                class="title-icon"
                size={16}
              />
            </div>
            <h2 class="card-title">
              {feed.title}
            </h2>
          </a>
        </div>
        {#if publishedLabel}
          <p class="card-dateline">{publishedLabel}</p>
        {/if}
      </div>

      <!-- Scrollable content -->
      <div
        bind:this={scrollAreaRef}
        style="touch-action: pan-y; overflow-x: hidden;"
        class="scroll-area"
        data-testid="unified-scroll-area"
      >
        {#if hasDescription && !isAISummaryRequested}
          <div class="content-block">
            <div class="summary-prose summary-prose--clamp">
              {feed.description}
            </div>
          </div>
        {/if}

        {#if isAISummaryRequested}
          <div
            class="content-block ai-summary-block"
            data-testid="ai-summary-section"
            transition:fade
          >
            <p class="section-label">
              {isSummarizing ? "Summary" : "AI Summary"}
            </p>
            {#if isSummarizing}
              <div class="loading-state">
                <div class="loading-dot" aria-hidden="true"></div>
                <span class="loading-label">Summarizing...</span>
              </div>
            {:else if summaryError}
              <p class="error-hint">{summaryError}</p>
            {:else if aiSummary}
              <p class="summary-prose ai-summary-text">
                {aiSummary}
              </p>
            {/if}
          </div>
        {/if}

        {#if isContentExpanded}
          <div
            class="content-block article-block"
            data-testid="content-section"
            transition:fade
          >
            <p class="section-label">Full Article</p>
            {#if isLoadingContent}
              <div class="loading-state">
                <div class="loading-dot" aria-hidden="true"></div>
                <span class="loading-label">Loading article...</span>
              </div>
            {:else if contentError}
              <p class="fallback-notice" data-testid="source-unavailable-notice">
                {contentError} Showing summary.
              </p>
              {#if hasDescription}
                <div
                  class="summary-prose article-prose"
                  data-testid="article-fallback-summary"
                >
                  {feed.description}
                </div>
              {/if}
            {:else if sanitizedFullContent}
              <div class="article-prose">
                {@html sanitizedFullContent}
              </div>
            {/if}
          </div>
        {/if}
      </div>
    </div>

    <!-- Footer -->
    <footer class="card-footer" data-testid="action-footer">
      <div class="flex gap-2 w-full">
        <button
          type="button"
          onclick={handleToggleContent}
          class="action-btn {isContentExpanded ? 'action-btn--active' : ''}"
          disabled={isLoadingContent}
        >
          <BookOpen size={14} />
          {isLoadingContent
            ? "Loading..."
            : isContentExpanded
              ? "Hide"
              : "Article"}
        </button>
        <button
          type="button"
          onclick={handleFavorite}
          class="action-btn action-btn--icon {isFavorited ? 'action-btn--active' : ''} {favoriteError ? 'action-btn--error' : ''}"
          disabled={isFavoriting || isFavorited}
          aria-label={isFavorited ? "Favorited" : isFavoriting ? "Saving favorite" : favoriteError ? "Favorite failed, tap to retry" : "Favorite"}
        >
          {#if isFavoriting}
            <div class="loading-dot-sm" aria-hidden="true"></div>
          {:else}
            <Star size={16} fill={isFavorited ? "currentColor" : "none"} stroke={favoriteError ? "var(--alt-terracotta)" : "currentColor"} />
          {/if}
        </button>
        <button
          type="button"
          onclick={handleGenerateAISummary}
          class="action-btn {isAISummaryRequested ? 'action-btn--active' : ''}"
          disabled={isSummarizing}
        >
          <Sparkles size={14} />
          {isSummarizing
            ? "Summarizing..."
            : isAISummaryRequested
              ? "Summary"
              : "Summary"}
        </button>
      </div>
    </footer>
  </div>
</div>

<style>
  .swipe-card {
    position: absolute;
    width: 100%;
    height: 95dvh;
    max-width: calc(100% - 1rem);
    background: var(--surface-bg);
    border: 1px solid var(--surface-border);
    user-select: none;
    overflow: hidden;
  }

  .card-inner {
    display: flex;
    flex-direction: column;
    height: 100%;
  }

  /* ── Thumbnail ── */
  .thumbnail-area {
    position: relative;
    width: 100%;
    height: 35%;
    min-height: 150px;
    overflow: hidden;
  }

  .thumbnail-img {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    opacity: 0;
    transition: opacity 0.3s;
  }

  .thumbnail-img--loaded {
    opacity: 1;
  }

  .thumbnail-shimmer {
    position: absolute;
    inset: 0;
    background: var(--surface-2);
    animation: shimmer-pulse 1.5s ease-in-out infinite;
  }

  .thumbnail-fallback {
    position: absolute;
    inset: 0;
    background: var(--surface-2);
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .thumbnail-fallback-text {
    font-family: var(--font-mono);
    font-size: 0.65rem;
    color: var(--alt-ash);
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }

  .thumbnail-overlay {
    position: absolute;
    inset: 0;
    pointer-events: none;
    background: linear-gradient(to top, var(--surface-bg) 0%, transparent 50%);
  }

  /* ── Content area ── */
  .content-area {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    padding: 0 0.75rem;
    margin-top: -1rem;
    position: relative;
    z-index: 1;
  }

  .card-header-compact {
    margin-bottom: 0.5rem;
  }

  .card-label {
    font-family: var(--font-body);
    font-size: 0.65rem;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--alt-ash);
    margin: 0 0 0.35rem;
  }

  .card-title-link {
    display: flex;
    align-items: flex-start;
    gap: 0.4rem;
    text-decoration: none;
    min-width: 0;
  }

  .card-title-link :global(.title-icon) {
    color: var(--alt-primary);
    flex-shrink: 0;
    margin-top: 0.15rem;
  }

  .card-title {
    font-family: var(--font-display);
    font-size: 1rem;
    font-weight: 600;
    color: var(--alt-primary);
    line-height: 1.3;
    margin: 0;
    word-break: break-word;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .card-title-link:hover .card-title {
    text-decoration: underline;
    text-underline-offset: 2px;
  }

  .card-dateline {
    font-family: var(--font-mono);
    font-size: 0.65rem;
    color: var(--alt-ash);
    margin: 0.25rem 0 0;
  }

  /* ── Scroll area ── */
  .scroll-area {
    flex: 1;
    overflow-y: auto;
    overflow-x: hidden;
    background: transparent;
    scroll-behavior: smooth;
    overscroll-behavior: contain;
    user-select: none;
  }

  .scroll-area::-webkit-scrollbar { width: 3px; }
  .scroll-area::-webkit-scrollbar-track { background: transparent; }
  .scroll-area::-webkit-scrollbar-thumb { background: var(--surface-border); }

  .scroll-area,
  .scroll-area :global(*) {
    -webkit-user-select: none;
    -moz-user-select: none;
    -ms-user-select: none;
    user-select: none;
  }

  /* ── Content blocks ── */
  .content-block {
    margin-bottom: 0.75rem;
  }

  .ai-summary-block {
    border-top: 1px solid var(--surface-border);
    padding-top: 0.5rem;
  }

  .article-block {
    border-top: 1px solid var(--surface-border);
    padding-top: 0.5rem;
  }

  .section-label {
    font-family: var(--font-body);
    font-size: 0.65rem;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--alt-ash);
    margin: 0 0 0.35rem;
  }

  .summary-prose {
    font-family: var(--font-body);
    font-size: 0.85rem;
    line-height: 1.6;
    color: var(--alt-charcoal);
    word-break: break-word;
    overflow-wrap: anywhere;
  }

  .summary-prose--clamp {
    display: -webkit-box;
    -webkit-line-clamp: 3;
    line-clamp: 3;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .ai-summary-text {
    white-space: pre-wrap;
  }

  .article-prose {
    font-family: var(--font-body);
    font-size: 0.85rem;
    line-height: 1.65;
    color: var(--alt-charcoal);
    max-width: 65ch;
    word-break: break-word;
    overflow-wrap: anywhere;
  }

  .article-prose :global(a) {
    color: var(--alt-primary);
    text-decoration: underline;
    text-underline-offset: 2px;
  }

  .article-prose :global(img) {
    max-width: 100%;
    height: auto;
  }

  /* ── Loading ── */
  .loading-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.4rem;
    padding: 0.75rem 0;
  }

  .loading-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--alt-ash);
    animation: pulse 1.2s ease-in-out infinite;
  }

  .loading-dot-sm {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: currentColor;
    animation: pulse 1.2s ease-in-out infinite;
    flex-shrink: 0;
  }

  .loading-label {
    font-family: var(--font-body);
    font-size: 0.75rem;
    font-style: italic;
    color: var(--alt-ash);
  }

  .error-hint {
    font-size: 0.75rem;
    color: var(--alt-terracotta);
    text-align: center;
    padding: 0.5rem 0;
    margin: 0;
  }

  .fallback-notice {
    font-family: var(--font-mono);
    font-size: 0.65rem;
    letter-spacing: 0.06em;
    color: var(--alt-ash);
    margin: 0 0 0.5rem;
    padding: 0;
  }

  /* ── Footer ── */
  .card-footer {
    position: relative;
    z-index: 2;
    border-top: 1px solid var(--surface-border);
    padding: 0.75rem;
    padding-bottom: calc(0.75rem + env(safe-area-inset-bottom, 0px));
  }

  /* ── Action buttons ── */
  .action-btn {
    font-family: var(--font-body);
    font-size: 0.75rem;
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--alt-charcoal);
    background: transparent;
    border: 1.5px solid var(--alt-charcoal);
    padding: 0.5rem 0.75rem;
    min-height: 44px;
    flex: 1;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 0.35rem;
    cursor: pointer;
    transition: background 0.15s, color 0.15s;
  }

  .action-btn:active:not(:disabled) {
    background: var(--alt-charcoal);
    color: var(--surface-bg);
  }

  .action-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .action-btn--icon {
    flex: 0 0 auto;
    padding: 0.5rem;
    width: 44px;
  }

  .action-btn--error {
    border-color: var(--alt-terracotta);
    color: var(--alt-terracotta);
  }

  .action-btn--active {
    background: var(--alt-charcoal);
    color: var(--surface-bg);
  }

  /* ── Animations ── */
  @keyframes pulse {
    0%, 100% { opacity: 0.3; }
    50% { opacity: 1; }
  }

  @keyframes shimmer-pulse {
    0%, 100% { opacity: 0.5; }
    50% { opacity: 1; }
  }

  @media (prefers-reduced-motion: reduce) {
    .loading-dot,
    .loading-dot-sm {
      animation: none;
      opacity: 0.6;
    }
    .thumbnail-shimmer {
      animation: none;
      opacity: 0.7;
    }
  }
</style>
