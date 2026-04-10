<script lang="ts">
import {
	BookOpen,
	RefreshCw,
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
import type { RenderFeed } from "$lib/schema/feed";
import { sanitizeHtml } from "$lib/utils/sanitizeHtml";
import { simulateTypewriterEffect } from "$lib/utils/streamingRenderer";
import { isTransientError } from "$lib/utils/errorClassification";
import {
	createClientTransport,
	streamSummarizeWithAbortAdapter,
} from "$lib/connect";

interface Props {
	feed: RenderFeed;
	statusMessage: string | null;
	onDismiss: (direction: number) => Promise<void> | void;
	getCachedContent?: (feedUrl: string) => string | null;
	getCachedArticleId?: (feedUrl: string) => string | null;
	isBusy?: boolean;
	initialArticleContent?: string | null;
	/** Callback when articleId is resolved (e.g., after fetching content creates an article) */
	onArticleIdResolved?: (feedLink: string, articleId: string) => void;
}

const {
	feed,
	statusMessage,
	onDismiss,
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

// Retry counters
let contentRetryCount = $state(0);
let summaryRetryCount = $state(0);

// Swipe state with Spring
const SWIPE_THRESHOLD = 60;
const HORIZONTAL_SWIPE_THRESHOLD = 10;
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

// Derived button states
const articleButtonState = $derived.by(() => {
	if (isLoadingContent) return "loading" as const;
	if (contentError) return "error" as const;
	return "idle" as const;
});

const summaryButtonState = $derived.by(() => {
	if (isSummarizing) return "loading" as const;
	if (summaryError && !aiSummary) return "error" as const;
	return "idle" as const;
});

// Auto-fetch content
onMount(() => {
	// Initialize with prop value if available
	if (initialArticleContent) {
		fullContent = initialArticleContent;
	}

	// Use normalizedUrl for cache access (consistent with articlePrefetcher)
	const cached = getCachedContent?.(feed.normalizedUrl);
	if (cached) {
		fullContent = cached;
		// Also check for cached articleId and notify parent
		const cachedArticleId = getCachedArticleId?.(feed.normalizedUrl);
		if (cachedArticleId && onArticleIdResolved) {
			onArticleIdResolved(feed.link, cachedArticleId);
		}
	} else if (!fullContent) {
		// Background fetch using normalizedUrl
		getFeedContentOnTheFlyClient(feed.normalizedUrl)
			.then((res) => {
				if (res.content) {
					fullContent = res.content;
				}
				// Notify parent if articleId was resolved (article created during fetch)
				if (res.article_id && onArticleIdResolved) {
					onArticleIdResolved(feed.link, res.article_id);
				}
			})
			.catch((err) => {
				console.error("[SwipeFeedCard] Error auto-fetching content:", err);
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

async function fetchArticleContent(forceRefresh = false): Promise<boolean> {
	try {
		const res = await getFeedContentOnTheFlyClient(feed.normalizedUrl, {
			forceRefresh,
		});
		if (res.content) {
			fullContent = res.content;
			if (res.article_id && onArticleIdResolved) {
				onArticleIdResolved(feed.link, res.article_id);
			}
			return true;
		}
		contentError = "Could not fetch article content";
		return false;
	} catch (err) {
		if (isTransientError(err) && contentRetryCount < 1) {
			contentRetryCount++;
			await new Promise((resolve) => setTimeout(resolve, 500));
			try {
				const res = await getFeedContentOnTheFlyClient(feed.normalizedUrl);
				if (res.content) {
					fullContent = res.content;
					if (res.article_id && onArticleIdResolved) {
						onArticleIdResolved(feed.link, res.article_id);
					}
					return true;
				}
				contentError = "Could not fetch article content";
				return false;
			} catch {
				contentError = "Could not fetch article content";
				return false;
			}
		}
		contentError = "Could not fetch article content";
		return false;
	}
}

async function handleRefetchContent() {
	fullContent = null;
	aiSummary = null;
	summaryError = null;
	contentError = null;
	isLoadingContent = true;
	const success = await fetchArticleContent(true);
	isLoadingContent = false;
	if (success) {
		isContentExpanded = true;
	}
}

async function handleToggleContent() {
	if (contentError && !isLoadingContent) {
		isLoadingContent = true;
		contentError = null;
		const success = await fetchArticleContent();
		isLoadingContent = false;
		if (success) {
			isContentExpanded = true;
		}
		return;
	}

	if (!isContentExpanded && !fullContent) {
		const cached = getCachedContent?.(feed.normalizedUrl);
		if (cached) {
			fullContent = cached;
			isContentExpanded = true;
			return;
		}

		isLoadingContent = true;
		contentError = null;
		await fetchArticleContent();
		isLoadingContent = false;
	}
	isContentExpanded = !isContentExpanded;
}

function handleGenerateAISummary(forceRefresh = false) {
	if (summaryError && !aiSummary) {
		summaryError = null;
	}

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
			forceRefresh,
		},
		(chunk) => {
			if (feed.link !== targetFeedLink) return;
			aiSummary = (aiSummary || "") + chunk;
		},
		{
			tick,
			typewriter: true,
			typewriterDelay: 10,
			onChunk: (chunkCount, chunkSize, decodedLength, totalLength, preview) => {
				if (chunkCount === 1) {
					isSummarizing = false;
				}
				if (chunkCount <= 5) {
					console.log("[StreamSummarize] Chunk received and rendered", {
						chunkCount,
						chunkSize,
						decodedLength,
						totalLength,
						preview,
					});
				}
			},
			onComplete: (totalLength, chunkCount) => {
				console.log("[StreamSummarize] Final chunk decoded", {
					chunkCount: chunkCount + 1,
					totalLength,
				});
			},
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

			console.error("[StreamSummarize] Error streaming summary:", {
				error: errorMessage,
				isAuthError,
				hasPartialData: !!aiSummary && aiSummary.length > 0,
			});

			if (isAuthError) {
				summaryError =
					"Authentication failed. Please refresh the page and try again.";
				isSummarizing = false;
				return;
			}

			if (aiSummary && aiSummary.length > 0) {
				console.warn(
					"[StreamSummarize] Using partial summary due to stream error",
				);
				summaryError = "Stream interrupted. Summary may be incomplete.";
				isSummarizing = false;
				return;
			}

			if (isTransientError(err) && summaryRetryCount < 1) {
				summaryRetryCount++;
				setTimeout(() => {
					isSummarizing = false;
					handleGenerateAISummary();
				}, 500);
				return;
			}

			console.log("[StreamSummarize] Falling back to legacy endpoint");
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
				console.error(
					"[StreamSummarize] Legacy endpoint also failed:",
					legacyErr,
				);
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
		console.error("[SwipeFeedCard] Failed to favorite feed:", err);
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
</script>

<div
  bind:this={swipeElement}
  class="swipe-card"
  use:swipe={{ threshold: SWIPE_THRESHOLD, restraint: 120, allowedTime: 500 }}
  aria-busy={isBusy}
  data-testid="swipe-card"
  style="{cardStyle}; touch-action: none;"
>
  <div class="card-inner">
    <!-- Header -->
    <header class="card-header">
      <p class="card-label">Swipe to mark as read</p>
      <div class="flex items-center gap-2">
        <a
          href={feed.link}
          target="_blank"
          rel="noopener noreferrer"
          aria-label="Open article in new tab"
          class="card-title-link"
        >
          <div class="flex-shrink-0">
            <SquareArrowOutUpRight
              class="title-icon"
              size={18}
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
    </header>

    <!-- Scroll Area -->
    <div
      bind:this={scrollAreaRef}
      style="touch-action: pan-y; overflow-x: hidden;"
      class="scroll-area"
      data-testid="unified-scroll-area"
    >
      {#if hasDescription && !isAISummaryRequested}
        <div class="content-block" transition:fade>
          <p class="section-label">Summary</p>
          <div class="summary-prose">
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
          {:else if summaryError && !aiSummary}
            <div class="error-box" role="alert">
              {summaryError}
            </div>
          {:else if aiSummary}
            <p class="summary-prose ai-summary-text">
              {aiSummary}
            </p>
            {#if summaryError}
              <p class="error-hint" role="alert">{summaryError}</p>
            {/if}
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
            <div class="error-box" role="alert">
              {contentError}
            </div>
          {:else if sanitizedFullContent}
            <div class="article-prose">
              {@html sanitizedFullContent}
            </div>
          {/if}
        </div>
      {/if}
    </div>

    <!-- Footer -->
    <footer class="card-footer" data-testid="action-footer">
      <div class="flex gap-2 w-full">
        <button
          type="button"
          onclick={isContentExpanded ? handleRefetchContent : handleToggleContent}
          class="action-btn {articleButtonState === 'error' ? 'action-btn--error' : ''} {isContentExpanded ? 'action-btn--active' : ''}"
          disabled={isLoadingContent}
          class:action-btn--active={isContentExpanded && articleButtonState !== 'error'}
        >
          {#if articleButtonState === 'loading'}
            <div class="loading-dot-sm" aria-hidden="true"></div>
            Loading...
          {:else if articleButtonState === 'error'}
            <RefreshCw size={14} />
            Try again
          {:else if isContentExpanded}
            <RefreshCw size={14} />
            Re-fetch
          {:else}
            <BookOpen size={14} />
            Article
          {/if}
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
          onclick={() => handleGenerateAISummary(!!aiSummary)}
          class="action-btn {summaryButtonState === 'error' ? 'action-btn--error' : ''} {isAISummaryRequested && summaryButtonState !== 'error' ? 'action-btn--active' : ''}"
          disabled={isSummarizing}
        >
          {#if summaryButtonState === 'loading'}
            <Sparkles size={14} />
            Summarizing...
          {:else if summaryButtonState === 'error'}
            <RefreshCw size={14} />
            Try again
          {:else if aiSummary}
            <RefreshCw size={14} />
            Re-summarize
          {:else}
            <Sparkles size={14} />
            Summary
          {/if}
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
  }

  .card-inner {
    display: flex;
    flex-direction: column;
    height: 100%;
  }

  /* ── Header ── */
  .card-header {
    position: relative;
    z-index: 2;
    border-bottom: 1px solid var(--surface-border);
    padding: 0.75rem;
  }

  .card-label {
    font-family: var(--font-body);
    font-size: 0.65rem;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--alt-ash);
    margin: 0 0 0.5rem;
  }

  .card-title-link {
    display: flex;
    align-items: flex-start;
    gap: 0.5rem;
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
    font-size: 1.05rem;
    font-weight: 600;
    color: var(--alt-primary);
    line-height: 1.3;
    margin: 0;
    word-break: break-word;
    white-space: normal;
    min-width: 0;
  }

  .card-title-link:hover .card-title {
    text-decoration: underline;
    text-underline-offset: 2px;
  }

  .card-dateline {
    font-family: var(--font-mono);
    font-size: 0.65rem;
    color: var(--alt-ash);
    margin: 0.35rem 0 0;
  }

  /* ── Scroll area ── */
  .scroll-area {
    flex: 1;
    overflow-y: auto;
    overflow-x: hidden;
    padding: 0.75rem;
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
    margin-bottom: 1rem;
  }

  .ai-summary-block {
    border-top: 1px solid var(--surface-border);
    padding-top: 0.75rem;
  }

  .article-block {
    border-top: 1px solid var(--surface-border);
    padding-top: 0.75rem;
  }

  .section-label {
    font-family: var(--font-body);
    font-size: 0.65rem;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--alt-ash);
    margin: 0 0 0.5rem;
  }

  .summary-prose {
    font-family: var(--font-body);
    font-size: 0.9rem;
    line-height: 1.65;
    color: var(--alt-charcoal);
    word-break: break-word;
    overflow-wrap: anywhere;
  }

  .ai-summary-text {
    white-space: pre-wrap;
  }

  .article-prose {
    font-family: var(--font-body);
    font-size: 0.9rem;
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
    gap: 0.5rem;
    padding: 1rem 0;
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
    font-size: 0.82rem;
    font-style: italic;
    color: var(--alt-ash);
  }

  /* ── Error ── */
  .error-box {
    border: 1px solid var(--alt-terracotta);
    padding: 0.75rem;
    color: var(--alt-terracotta);
    font-family: var(--font-body);
    font-size: 0.82rem;
    text-align: center;
  }

  .error-hint {
    font-size: 0.7rem;
    color: var(--alt-terracotta);
    margin: 0.35rem 0 0;
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

  @media (prefers-reduced-motion: reduce) {
    .loading-dot,
    .loading-dot-sm {
      animation: none;
      opacity: 0.6;
    }
  }
</style>
