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
import {
	createClientTransport,
	streamSummarizeWithAbortAdapter,
} from "$lib/connect";
import type { RenderFeed } from "$lib/schema/feed";
import {
	type ArticleContentPhase,
	CONTENT_PENDING_LABEL,
	CONTENT_RETRYING_LABEL,
	CONTENT_UNAVAILABLE_LABEL,
	EMPTY_CONTENT_ERROR,
	foregroundRetryDelayMs,
	READ_ORIGINAL_LABEL,
	TRY_AGAIN_LABEL,
} from "$lib/utils/articleContentState";
import {
	articleContentErrorMessage,
	isTransientError,
} from "$lib/utils/errorClassification";
import { sanitizeHtml } from "$lib/utils/sanitizeHtml";
import { simulateTypewriterEffect } from "$lib/utils/streamingRenderer";

interface Props {
	feed: RenderFeed;
	statusMessage: string | null;
	onDismiss: (direction: number) => Promise<void> | void;
	getCachedContent?: (feedUrl: string) => string | null;
	getCachedArticleId?: (feedUrl: string) => string | null;
	/**
	 * Fetch the body through the shared prefetcher, which dedupes against an
	 * in-flight prefetch for the same URL and honors the host cooldown.
	 * Resolves null when the body is unavailable right now (retryable).
	 */
	requestContent?: (feedUrl: string) => Promise<string | null>;
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
	requestContent,
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
// Three honest states, not two booleans. `pending` / `retrying` are explicitly
// not `failed`: the background fetch used to swallow its rejection entirely
// (console.error and nothing else), so a reader looking at the RSS description
// had no way to tell a failure from a body that had simply not arrived yet.
let contentPhase = $state<ArticleContentPhase>("idle");
let contentError = $state<string | null>(null);
// True only while a load the READER asked for is running. The background
// fetch deliberately does not put the footer button into its loading state —
// locking the button during a fetch nobody asked for is what keeps the reader
// out of the panel where the pending state and the remedies live.
let isUserFetching = $state(false);
// The one automatic re-attempt this card is allowed per failure.
let foregroundRetrySpent = false;
let contentRetryTimer: ReturnType<typeof setTimeout> | null = null;
// Monotonic token: a load applies its result only while it is the newest one.
let contentToken = 0;

let summaryAbortController = $state<AbortController | null>(null);
let contentAbortController: AbortController | null = null;
let favoriteErrorTimer: ReturnType<typeof setTimeout> | null = null;
let summaryRetryTimer: ReturnType<typeof setTimeout> | null = null;

let isFavoriting = $state(false);
let isFavorited = $state(false);
let favoriteError = $state<string | null>(null);

// Retry counter (summary stream only; the article body has its own policy)
let summaryRetryCount = $state(0);

const debug = import.meta.env.DEV;

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
const isContentPending = $derived(
	contentPhase === "pending" || contentPhase === "retrying",
);
const pendingLabel = $derived(
	contentPhase === "retrying" ? CONTENT_RETRYING_LABEL : CONTENT_PENDING_LABEL,
);
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
	if (isUserFetching) return "loading" as const;
	if (contentPhase === "failed") {
		// A failure the reader watched happen gets the verb: the panel is open,
		// the notice naming the problem is in it, and pressing this really does
		// re-fetch (`handleRefetchContent`).
		//
		// A background fetch they never asked for gets the noun instead. On a
		// collapsed card nothing says what was attempted, so a bare "Try again"
		// asks the reader to repeat an attempt they never saw, and it does it by
		// spending the only control that says the card has an article to open —
		// while the press still runs `handleToggleContent`, i.e. opens the panel.
		// "Article unavailable" states the verdict the background fetch reached
		// AND stays the way in; the remedies live behind it, next to the notice.
		return isContentExpanded ? ("error" as const) : ("unavailable" as const);
	}
	return "idle" as const;
});

/** Both terminal labels carry the failed styling; only one carries the verb. */
const articleButtonFailed = $derived(
	articleButtonState === "error" || articleButtonState === "unavailable",
);

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
		contentPhase = "ready";
	}

	// Use normalizedUrl for cache access (consistent with articlePrefetcher)
	const cached = getCachedContent?.(feed.normalizedUrl);
	if (cached) {
		fullContent = cached;
		contentPhase = "ready";
		// Also check for cached articleId and notify parent
		const cachedArticleId = getCachedArticleId?.(feed.normalizedUrl);
		if (cachedArticleId && onArticleIdResolved) {
			onArticleIdResolved(feed.link, cachedArticleId);
		}
	} else if (!fullContent) {
		// Background fetch. It goes through the same loader as every tap, so a
		// failure here lands in the same `failed` state with the same wording
		// instead of being logged and forgotten.
		void loadContent({ viaPrefetcher: true });
	}

	return () => {
		contentToken++;
		contentAbortController?.abort();
		contentAbortController = null;
		if (contentRetryTimer) clearTimeout(contentRetryTimer);
		if (favoriteErrorTimer) clearTimeout(favoriteErrorTimer);
		if (summaryRetryTimer) clearTimeout(summaryRetryTimer);
		summaryAbortController?.abort();
	};
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

/**
 * One request for the body.
 *
 * The background path prefers the shared prefetcher — a card mounting
 * mid-prefetch joins that request instead of firing a second, unserialized one
 * at the same host. A force-refresh always goes direct: the point of it is to
 * bypass what the prefetcher has cached.
 */
async function requestBody(opts: {
	viaPrefetcher?: boolean;
	forceRefresh?: boolean;
}): Promise<{ content: string; articleId: string }> {
	if (opts.viaPrefetcher && requestContent && !opts.forceRefresh) {
		const content = await requestContent(feed.normalizedUrl);
		return {
			content: content ?? "",
			articleId: getCachedArticleId?.(feed.normalizedUrl) ?? "",
		};
	}

	contentAbortController?.abort();
	contentAbortController = new AbortController();
	const res = await getFeedContentOnTheFlyClient(feed.normalizedUrl, {
		forceRefresh: opts.forceRefresh,
		signal: contentAbortController.signal,
	});
	return { content: res.content ?? "", articleId: res.article_id ?? "" };
}

/**
 * Record a terminal failure. The wording comes from articleContentErrorMessage
 * and nowhere else — ADR-000959 §6 forbids putting the upstream `message`
 * ("Service temporarily unavailable due to circuit breaker") on the reading
 * surface.
 */
function failContent(err: unknown, token: number) {
	if (token !== contentToken) return;
	console.warn("[SwipeFeedCard] article content unavailable:", err);
	contentError = articleContentErrorMessage(err);
	contentPhase = "failed";
}

function applyBody(
	body: { content: string; articleId: string },
	token: number,
): boolean {
	if (token !== contentToken) return false;
	if (!body.content) {
		// `content: ""` is a state, not a falsy no-op (ADR-000581). The
		// background path used to drop it silently, which left the card
		// looking like it was still loading forever.
		failContent(EMPTY_CONTENT_ERROR, token);
		return false;
	}
	fullContent = body.content;
	contentPhase = "ready";
	contentError = null;
	if (body.articleId && onArticleIdResolved) {
		onArticleIdResolved(feed.link, body.articleId);
	}
	return true;
}

/**
 * Load the body, with at most ONE automatic re-attempt.
 *
 * The re-attempt is admitted for exactly one condition — see
 * foregroundRetryDelayMs — and announces itself while it waits instead of
 * hiding behind a frozen frame of the failed state.
 */
async function loadContent(
	opts: {
		viaPrefetcher?: boolean;
		forceRefresh?: boolean;
		userInitiated?: boolean;
	} = {},
): Promise<boolean> {
	const token = ++contentToken;
	contentPhase = "pending";
	contentError = null;
	if (opts.userInitiated) isUserFetching = true;

	try {
		try {
			return applyBody(await requestBody(opts), token);
		} catch (err) {
			if (token !== contentToken) return false;
			if (isAbortReason(err)) return false;

			const delayMs = foregroundRetrySpent ? null : foregroundRetryDelayMs(err);
			if (delayMs === null) {
				failContent(err, token);
				return false;
			}

			foregroundRetrySpent = true;
			contentPhase = "retrying";
			await new Promise<void>((resolve) => {
				if (contentRetryTimer) clearTimeout(contentRetryTimer);
				contentRetryTimer = setTimeout(() => {
					contentRetryTimer = null;
					resolve();
				}, delayMs);
			});
			if (token !== contentToken) return false;

			contentPhase = "pending";
			try {
				return applyBody(await requestBody(opts), token);
			} catch (retryErr) {
				if (token !== contentToken || isAbortReason(retryErr)) return false;
				failContent(retryErr, token);
				return false;
			}
		}
	} finally {
		if (token === contentToken && opts.userInitiated) {
			isUserFetching = false;
		}
	}
}

function isAbortReason(err: unknown): boolean {
	if (!(err instanceof Error)) return false;
	return (
		err.name === "AbortError" ||
		err.message.includes("abort") ||
		err.message.includes("cancel")
	);
}

async function handleRefetchContent() {
	fullContent = null;
	aiSummary = null;
	summaryError = null;
	// A reader asking again is a fresh budget: the automatic attempt is spent
	// per failure, not once per card.
	foregroundRetrySpent = false;
	isContentExpanded = true;
	await loadContent({ userInitiated: true, forceRefresh: true });
}

async function handleToggleContent() {
	if (isContentExpanded && contentPhase === "ready") {
		isContentExpanded = false;
		return;
	}

	// Open first, then fetch. Awaiting the body before expanding meant the
	// panel's pending state could not be reached on a first tap: the panel
	// arrived already in its final state, so a failure read as a card that had
	// been broken all along. ADR-000963 §7 left this card's expansion order
	// out of scope and recorded the mismatch with VisualPreviewCard as a
	// tradeoff; this closes it.
	isContentExpanded = true;

	if (fullContent) return;
	// A load is already running — join it rather than firing a second request
	// at the same host. The panel shows its pending state either way.
	if (isContentPending) return;

	if (contentPhase === "failed") foregroundRetrySpent = false;
	await loadContent({ userInitiated: true });
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
				if (debug && chunkCount <= 5) {
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
				if (debug) {
					console.log("[StreamSummarize] Final chunk decoded", {
						chunkCount: chunkCount + 1,
						totalLength,
					});
				}
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
				if (summaryRetryTimer) clearTimeout(summaryRetryTimer);
				summaryRetryTimer = setTimeout(() => {
					summaryRetryTimer = null;
					isSummarizing = false;
					handleGenerateAISummary();
				}, 500);
				return;
			}

			if (debug)
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
		if (favoriteErrorTimer) clearTimeout(favoriteErrorTimer);
		favoriteErrorTimer = setTimeout(() => {
			favoriteErrorTimer = null;
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
      <button
        type="button"
        class="keep-stamp"
        class:keep-stamp--stamped={isFavorited}
        class:keep-stamp--error={favoriteError}
        onclick={handleFavorite}
        disabled={isFavoriting || isFavorited}
        aria-pressed={isFavorited}
        aria-label={isFavorited ? "Favorited" : isFavoriting ? "Saving favorite" : favoriteError ? "Favorite failed, tap to retry" : "Favorite"}
        data-testid="keep-stamp"
      >
        {#if isFavoriting}
          <div class="loading-dot-sm" aria-hidden="true"></div>
        {:else}
          <Star size={18} fill={isFavorited ? "currentColor" : "none"} />
        {/if}
      </button>
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
          {#if isContentPending}
            <!-- A request in flight is not a verdict. It says what it is doing
                 and keeps the RSS body underneath, so the panel is never a
                 blank wait and never an error that has not happened yet. -->
            <div class="loading-state" data-testid="article-content-pending">
              <div class="loading-dot" aria-hidden="true"></div>
              <span class="loading-label">{pendingLabel}</span>
            </div>
            {#if hasDescription}
              <div
                class="summary-prose article-prose"
                data-testid="article-fallback-summary"
              >
                {feed.description}
              </div>
            {/if}
          {:else if contentPhase === "failed"}
            <div data-testid="article-content-failed">
              <p
                class="fallback-notice"
                role="alert"
                data-testid="source-unavailable-notice"
              >
                {contentError} Showing summary.
              </p>
              <!-- Naming the problem without offering a way out is the defect
                   (NN/g heuristic 9). Both remedies ship with the message. -->
              <div class="remedy-row">
                <button
                  type="button"
                  class="retry-btn"
                  onclick={handleRefetchContent}
                  disabled={isUserFetching}
                  data-testid="retry-content"
                >
                  {TRY_AGAIN_LABEL}
                </button>
                <a
                  class="remedy-link"
                  href={feed.link}
                  target="_blank"
                  rel="noopener noreferrer"
                  data-testid="read-original-link"
                >
                  {READ_ORIGINAL_LABEL}
                </a>
              </div>
            </div>
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

    <!-- Footer: reading actions only (keep judgment lives on the stamp) -->
    <footer class="card-footer" data-testid="action-footer">
      <div class="flex gap-3 w-full">
        <button
          type="button"
          data-testid="article-action"
          onclick={isContentExpanded ? handleRefetchContent : handleToggleContent}
          class="action-btn {articleButtonFailed ? 'action-btn--error' : ''} {isContentExpanded ? 'action-btn--active' : ''}"
          disabled={isUserFetching}
          class:action-btn--active={isContentExpanded && !articleButtonFailed}
        >
          {#if articleButtonState === 'loading'}
            <div class="loading-dot-sm" aria-hidden="true"></div>
            Loading...
          {:else if articleButtonState === 'error'}
            <RefreshCw size={14} />
            {TRY_AGAIN_LABEL}
          {:else if articleButtonState === 'unavailable'}
            <BookOpen size={14} />
            {CONTENT_UNAVAILABLE_LABEL}
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
          data-testid="summary-action"
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
    height: 100%;
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
    /* Reserve the top-right corner for the keep stamp */
    padding-right: 3.75rem;
  }

  /* ── Keep stamp ──
     Press-mark chip flush to the card's top-right corner: opaque paper
     ground, sharp edges. Stamped = inked (inverted). 48px, not the 44px
     floor: edge-flush targets double miss rates (Henze 2011). */
  .keep-stamp {
    position: absolute;
    top: 0;
    right: 0;
    z-index: 2;
    width: 48px;
    height: 48px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: var(--surface-bg);
    border: none;
    border-left: 1.5px solid var(--alt-charcoal);
    border-bottom: 1.5px solid var(--alt-charcoal);
    color: var(--alt-charcoal);
    cursor: pointer;
    touch-action: manipulation;
    transition: background 0.15s, color 0.15s;
  }

  .keep-stamp:active:not(:disabled) {
    background: var(--alt-charcoal);
    color: var(--surface-bg);
  }

  .keep-stamp--stamped {
    background: var(--alt-charcoal);
    color: var(--surface-bg);
  }

  .keep-stamp--stamped:disabled {
    opacity: 1;
    cursor: default;
  }

  .keep-stamp--error {
    border-color: var(--alt-terracotta);
    color: var(--alt-terracotta);
  }

  .keep-stamp:disabled:not(.keep-stamp--stamped) {
    opacity: 0.4;
    cursor: not-allowed;
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

  .fallback-notice {
    font-family: var(--font-mono);
    font-size: 0.65rem;
    letter-spacing: 0.06em;
    color: var(--alt-ash);
    margin: 0 0 0.5rem;
    padding: 0;
  }

  /* ── Remedies for a terminal content failure ── */
  .remedy-row {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    flex-wrap: wrap;
    margin: 0 0 0.75rem;
  }

  .retry-btn {
    font-family: var(--font-body);
    font-size: 0.7rem;
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--alt-charcoal);
    background: transparent;
    border: 1.5px solid var(--alt-charcoal);
    padding: 0.4rem 1rem;
    min-height: 44px;
    cursor: pointer;
    transition: background 0.15s, color 0.15s;
  }

  .retry-btn:active:not(:disabled) {
    background: var(--alt-charcoal);
    color: var(--surface-bg);
  }

  .retry-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .remedy-link {
    font-family: var(--font-body);
    font-size: 0.7rem;
    font-weight: 600;
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--alt-charcoal);
    text-decoration: underline;
    text-underline-offset: 0.2em;
    min-height: 44px;
    display: inline-flex;
    align-items: center;
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
    min-height: 48px;
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
