<script lang="ts">
import { ChevronLeft, ChevronRight, X } from "@lucide/svelte";
import { Dialog as DialogPrimitive } from "bits-ui";
import { tick } from "svelte";
import { getFeedContentOnTheFlyClient } from "$lib/api/client/articles";
import RenderFeedDetails from "$lib/components/mobile/RenderFeedDetails.svelte";
import {
	createClientTransport,
	streamSummarizeWithAbortAdapter,
} from "$lib/connect";
import type { RenderFeed } from "$lib/schema/feed";
import {
	type ArticleContentPhase,
	CONTENT_PENDING_LABEL,
	CONTENT_RETRYING_LABEL,
	EMPTY_CONTENT_ERROR,
	foregroundRetryDelayMs,
	READ_ORIGINAL_LABEL,
	TRY_AGAIN_LABEL,
} from "$lib/utils/articleContentState";
import { articlePrefetcher } from "$lib/utils/articlePrefetcher";
import {
	articleContentErrorMessage,
	isTransientError,
} from "$lib/utils/errorClassification";
import {
	buildSummaryRendererOptions,
	processArticleFetchResponse,
} from "./FeedDetailModal.logic";

interface Props {
	open: boolean;
	feed: RenderFeed | null;
	onOpenChange: (open: boolean) => void;
	hasPrevious?: boolean;
	hasNext?: boolean;
	onPrevious?: () => void;
	onNext?: () => void;
	feeds?: RenderFeed[];
	currentIndex?: number;
	/**
	 * Rendered as a footer action when supplied. A typed callback rather than a
	 * snippet: every caller passed the same "Mark as Read" button, and a
	 * caller-owned button carries the *caller's* style scope, so it could never
	 * match this component's footer rules — it arrived unstyled and, on a phone
	 * rail, visibly foreign.
	 */
	onMarkAsRead?: () => void;
	isMarkingAsRead?: boolean;
}

let {
	open = $bindable(),
	feed,
	onOpenChange,
	hasPrevious = false,
	hasNext = false,
	onPrevious,
	onNext,
	feeds,
	currentIndex,
	onMarkAsRead,
	isMarkingAsRead = false,
}: Props = $props();

// Content fetching state.
//
// One explicit phase rather than a truth table of booleans. The auto-fetch
// effect below keys off it, and `idle` is the ONLY phase it acts on: that is
// what keeps ADR-000581's infinite loop closed now that an empty body is a
// state of its own rather than a falsy no-op.
let contentPhase = $state<ArticleContentPhase>("idle");
let articleContent = $state<string | null>(null);
let articleID = $state<string | null>(null);
let contentError = $state<string | null>(null);
// The one automatic re-attempt allowed per failure.
let foregroundRetrySpent = false;
let contentRetryTimer: ReturnType<typeof setTimeout> | null = null;

// AI summary state
let isSummarizing = $state(false);
let summary = $state<string | null>(null);
let summaryError = $state<string | null>(null);
let abortController = $state<AbortController | null>(null);

// Content fetch abort controller
let contentAbortController = $state<AbortController | null>(null);

// Retry counter (summary stream only; the article body has its own policy)
let summaryRetryCount = $state(0);

const isFetchingContent = $derived(
	contentPhase === "pending" || contentPhase === "retrying",
);
const pendingLabel = $derived(
	contentPhase === "retrying" ? CONTENT_RETRYING_LABEL : CONTENT_PENDING_LABEL,
);

// Derived button states
const articleButtonState = $derived.by(() => {
	if (isFetchingContent) return "loading" as const;
	if (articleContent) return "success" as const;
	if (contentPhase === "failed") return "error" as const;
	return "idle" as const;
});

const summaryButtonState = $derived.by(() => {
	if (isSummarizing) return "loading" as const;
	if (summaryError) return "error" as const;
	if (summary) return "success" as const;
	return "idle" as const;
});

/**
 * Footer labels in a long and a short form.
 *
 * On the phone rail a cell is about a third of 346px, and "Re-fetch Article"
 * wraps to two lines there. Both forms are rendered and CSS picks one, rather
 * than JS reading the viewport: a viewport-derived string renders one way on the
 * server and another in the browser, which is a hydration mismatch. Both spans
 * are aria-hidden and the button carries the long form as its accessible name,
 * so the name is the same at every width.
 */
const articleLabel = $derived.by(() => {
	switch (articleButtonState) {
		case "loading":
			return { long: "Loading…", short: "Loading" };
		case "success":
			return { long: "Re-fetch Article", short: "Re-fetch" };
		case "error":
			return { long: "Try Again", short: "Retry" };
		default:
			return { long: "Full Article", short: "Article" };
	}
});

const summaryLabel = $derived.by(() => {
	switch (summaryButtonState) {
		case "loading":
			return { long: "Summarizing…", short: "Summarizing" };
		case "success":
			return { long: "Re-summarize", short: "Re-run" };
		case "error":
			return { long: "Try Again", short: "Retry" };
		default:
			return { long: "Summarize", short: "Summary" };
	}
});

const markAsReadLabel = $derived(
	isMarkingAsRead
		? { long: "Marking…", short: "Marking" }
		: { long: "Mark as Read", short: "Mark Read" },
);

function requestClose() {
	open = false;
	onOpenChange(false);
}

// Track previous feed URL to detect actual feed changes
let previousFeedUrl = $state<string | null>(null);

// Cleanup on modal close
$effect(() => {
	if (!open) {
		// Cancel any ongoing content fetch request
		if (contentAbortController) {
			contentAbortController.abort();
			contentAbortController = null;
		}
		// Cancel any ongoing summary request
		if (abortController) {
			abortController.abort();
			abortController = null;
		}
		// Reset states
		articleContent = null;
		articleID = null;
		summary = null;
		contentPhase = "idle";
		isSummarizing = false;
		contentError = null;
		summaryError = null;
		foregroundRetrySpent = false;
		if (contentRetryTimer) clearTimeout(contentRetryTimer);
		summaryRetryCount = 0;
		previousFeedUrl = null;
	}
});

// Manual scroll lock: only set overflow hidden (no pointer-events manipulation)
$effect(() => {
	if (!open) return;
	const originalOverflow = document.body.style.overflow;
	document.body.style.overflow = "hidden";
	return () => {
		document.body.style.overflow = originalOverflow;
	};
});

// Reset content states when feed changes (for arrow navigation)
$effect(() => {
	const currentFeedUrl = feed?.normalizedUrl ?? null;

	// Only reset when feed actually changes
	if (currentFeedUrl === previousFeedUrl) return;

	previousFeedUrl = currentFeedUrl;

	// Cancel any ongoing content fetch request
	if (contentAbortController) {
		contentAbortController.abort();
		contentAbortController = null;
	}
	// Cancel any ongoing summary request
	if (abortController) {
		abortController.abort();
		abortController = null;
	}
	// Reset content states
	articleContent = null;
	articleID = null;
	summary = null;
	contentPhase = "idle";
	isSummarizing = false;
	contentError = null;
	summaryError = null;
	foregroundRetrySpent = false;
	if (contentRetryTimer) clearTimeout(contentRetryTimer);
	summaryRetryCount = 0;
});

// Auto-fetch article content when modal opens.
//
// `idle` is the only phase this may act on. Acting on `failed` is ADR-000581's
// loop; acting on `pending` doubles the request rate at the publisher.
$effect(() => {
	if (!open || !feed) return;
	if (!feed.normalizedUrl) {
		if (contentPhase !== "failed") {
			contentError = "Article URL is not available";
			contentPhase = "failed";
		}
		return;
	}
	if (contentPhase === "idle") {
		void handleFetchFullArticle();
	}
});

// Keyboard navigation
$effect(() => {
	if (!open) return;

	function handleKeyDown(event: KeyboardEvent) {
		if (event.key === "ArrowLeft" && hasPrevious) {
			event.preventDefault();
			onPrevious?.();
		} else if (event.key === "ArrowRight" && hasNext) {
			event.preventDefault();
			onNext?.();
		}
	}

	window.addEventListener("keydown", handleKeyDown);
	return () => window.removeEventListener("keydown", handleKeyDown);
});

// Prefetch next 2 articles when modal opens or feed changes
$effect(() => {
	if (open && feeds && currentIndex !== undefined && currentIndex >= 0) {
		articlePrefetcher.triggerPrefetch(feeds, currentIndex, 2);
	}
});

async function handleRefetchArticle() {
	// Clear existing content and summary, then re-fetch with force refresh
	articleContent = null;
	articleID = null;
	summary = null;
	summaryError = null;
	contentError = null;
	// A reader asking again is a fresh budget: the automatic attempt is spent
	// per failure, not once per modal.
	foregroundRetrySpent = false;
	await handleFetchFullArticle(true);
}

/**
 * Record a terminal failure. The wording comes from articleContentErrorMessage
 * and nowhere else — ADR-000959 §6 keeps the upstream `message` (the BFF's
 * "Service temporarily unavailable due to circuit breaker") off the reading
 * surface.
 */
function failContent(err: unknown, targetFeedUrl: string) {
	if (feed?.normalizedUrl !== targetFeedUrl) return;
	contentError = articleContentErrorMessage(err);
	contentPhase = "failed";
}

function applyResult(
	result: ReturnType<typeof processArticleFetchResponse>,
	targetFeedUrl: string,
) {
	if (feed?.normalizedUrl !== targetFeedUrl) return;
	if (result.contentError) {
		// processArticleFetchResponse still owns the "is this body empty"
		// decision — the guard ADR-000581 added — but not the wording. Empty
		// bodies now read the same here as on every other surface.
		failContent(EMPTY_CONTENT_ERROR, targetFeedUrl);
		return;
	}
	articleContent = result.articleContent;
	articleID = result.articleID;
	contentError = null;
	contentPhase = "ready";
}

function isAbortReason(err: unknown): boolean {
	if (!(err instanceof Error)) return false;
	return (
		err.name === "AbortError" ||
		err.message.includes("abort") ||
		err.message.includes("cancel")
	);
}

/** One request. Kept separate so the re-attempt reuses it verbatim. */
async function requestBody(targetFeedUrl: string, forceRefresh: boolean) {
	if (contentAbortController) contentAbortController.abort();
	contentAbortController = new AbortController();
	return await getFeedContentOnTheFlyClient(targetFeedUrl, {
		signal: contentAbortController.signal,
		forceRefresh,
	});
}

/**
 * Fetch the body, with at most ONE automatic re-attempt.
 *
 * The re-attempt replaces the old `isTransientError` + fixed 500ms retry. That
 * predicate reads the error's MESSAGE, so what it re-sent into depended on
 * prose the publisher and the gateway are free to change — and a message
 * carrying "503" would have re-sent into an open circuit breaker, which is the
 * outcome ADR-000959 was written to prevent. The replacement decides on the
 * Connect code plus the declared failure scope and honours the server's own
 * Retry-After.
 */
async function handleFetchFullArticle(forceRefresh = false) {
	if (isFetchingContent) return;
	if (!feed?.normalizedUrl) {
		contentError = "Article URL is not available";
		contentPhase = "failed";
		return;
	}

	const targetFeedUrl = feed.normalizedUrl; // Capture for stale response validation

	// Check prefetch cache first (using normalizedUrl for consistency), skip when force refreshing
	if (!forceRefresh) {
		const cachedContent = articlePrefetcher.getCachedContent(targetFeedUrl);
		const cachedArticleId = articlePrefetcher.getCachedArticleId(targetFeedUrl);

		if (cachedContent) {
			// Validate feed hasn't changed before applying cached content
			if (feed.normalizedUrl !== targetFeedUrl) return;
			articleContent = cachedContent;
			articleID = cachedArticleId;
			contentError = null;
			contentPhase = "ready";
			return;
		}
	}

	contentPhase = "pending";
	contentError = null;

	try {
		applyResult(
			processArticleFetchResponse(
				await requestBody(targetFeedUrl, forceRefresh),
			),
			targetFeedUrl,
		);
		return;
	} catch (err) {
		if (isAbortReason(err)) return;
		if (feed.normalizedUrl !== targetFeedUrl) return;

		const delayMs = foregroundRetrySpent ? null : foregroundRetryDelayMs(err);
		if (delayMs === null) {
			failContent(err, targetFeedUrl);
			return;
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
		if (feed.normalizedUrl !== targetFeedUrl) return;

		contentPhase = "pending";
		try {
			applyResult(
				processArticleFetchResponse(
					await requestBody(targetFeedUrl, forceRefresh),
				),
				targetFeedUrl,
			);
		} catch (retryErr) {
			if (isAbortReason(retryErr)) return;
			failContent(retryErr, targetFeedUrl);
		}
	} finally {
		contentAbortController = null;
	}
}

async function handleSummarize(forceRefresh = false) {
	if (!feed?.link || isSummarizing) return;

	// Capture current feed URL for stale response validation.
	// If the user navigates to another article while streaming,
	// feed.normalizedUrl will differ from targetFeedUrl and
	// callbacks will discard the stale data.
	const targetFeedUrl = feed.normalizedUrl;

	// Cancel previous request
	if (abortController) {
		abortController.abort();
	}

	isSummarizing = true;
	summaryError = null;
	summary = "";

	try {
		const transport = createClientTransport();
		abortController = streamSummarizeWithAbortAdapter(
			transport,
			{
				feedUrl: feed.link,
				articleId: articleID || undefined,
				title: feed.title,
				forceRefresh,
			},
			(chunk: string) => {
				// Discard stale chunks if feed changed during streaming
				if (feed.normalizedUrl !== targetFeedUrl) return;
				summary = (summary || "") + chunk;
			},
			buildSummaryRendererOptions({ tick }),
			(_result) => {
				// onComplete — discard if feed changed
				if (feed.normalizedUrl !== targetFeedUrl) return;
				isSummarizing = false;
				abortController = null;
			},
			(error) => {
				// Discard stale errors if feed changed
				if (feed.normalizedUrl !== targetFeedUrl) return;
				// onError — ignore abort/cancel errors (user navigation)
				if (error.name === "AbortError") {
					isSummarizing = false;
					abortController = null;
					return;
				}
				if (
					error.message?.includes("abort") ||
					error.message?.includes("cancel")
				) {
					isSummarizing = false;
					abortController = null;
					return;
				}

				// Auto-retry for transient errors (1 attempt only)
				if (isTransientError(error) && summaryRetryCount < 1) {
					summaryRetryCount++;
					abortController = null;
					setTimeout(() => {
						isSummarizing = false;
						handleSummarize();
					}, 500);
					return;
				}

				summaryError = error.message || "Failed to generate summary";
				isSummarizing = false;
				abortController = null;
			},
		);
	} catch (err) {
		// Discard stale errors if feed changed
		if (feed.normalizedUrl !== targetFeedUrl) return;
		// Ignore abort/cancel errors (user navigation)
		if (err instanceof Error) {
			if (err.name === "AbortError") return;
			if (err.message.includes("abort") || err.message.includes("cancel"))
				return;
		}
		summaryError =
			err instanceof Error ? err.message : "Failed to generate summary";
		isSummarizing = false;
		abortController = null;
	}
}
</script>

{#if open}
<DialogPrimitive.Root open={true} onOpenChange={(value) => { if (!value) { open = false; onOpenChange(false); } }}>
	<DialogPrimitive.Portal>
		<DialogPrimitive.Overlay class="fixed inset-0 z-50" style="background: rgba(0,0,0,0.5);" />
		<DialogPrimitive.Content
			preventScroll={false}
			class="modal-content fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 w-[94vw] h-[92vh] sm:w-[88vw] sm:max-w-[1920px] sm:h-[88vh] overflow-hidden flex flex-col z-50"
		>
			{#if hasPrevious}
				<button
					onclick={onPrevious}
					class="nav-arrow nav-arrow--left"
					aria-label="Previous feed"
				>
					<ChevronLeft class="h-6 w-6" />
				</button>
			{/if}
			{#if hasNext}
				<button
					onclick={onNext}
					class="nav-arrow nav-arrow--right"
					aria-label="Next feed"
				>
					<ChevronRight class="h-6 w-6" />
				</button>
			{/if}

			{#if feed}
				<!--
					Phone-only close. On a phone the footer has room for reading
					actions or for chrome, not both, and chrome is what moves: Close
					is how you leave the modal, not something you do to the article.
					Hidden above 640px, where the footer's Close still serves.
				-->
				<button
					type="button"
					class="modal-close"
					data-testid="modal-close"
					aria-label="Close"
					onclick={requestClose}
				>
					<X class="h-5 w-5" />
				</button>

				<div class="modal-header">
					{#if feed.link}
						<a
							href={feed.link}
							target="_blank"
							rel="noopener noreferrer"
							class="modal-title-link"
						>
							<h2 class="modal-title">{feed.title || "Untitled"}</h2>
						</a>
					{:else}
						<h2 class="modal-title">{feed.title || "Untitled"}</h2>
					{/if}

					<div class="modal-meta">
						{#if feed.author}
							<span>{feed.author}</span>
						{/if}
						{#if feed.publishedAtFormatted}
							{#if feed.author}<span class="modal-meta-sep">&middot;</span>{/if}
							<span>{feed.publishedAtFormatted}</span>
						{/if}
					</div>

					{#if feed.mergedTagsLabel}
						<span class="modal-tags">
							{feed.mergedTagsLabel.split(" / ").join(" \u00b7 ")}
						</span>
					{/if}
				</div>

				<div class="modal-body">
					<div class="modal-body-grid">
						<article class="modal-article">
							{#if articleContent}
								<section class="content-section">
									<h3 class="section-label">FULL ARTICLE</h3>
									<RenderFeedDetails
										feedDetails={articleContent ? { content: articleContent, article_id: articleID ?? "", og_image_url: "", og_image_proxy_url: "" } : null}
										error={null}
									/>
								</section>
							{:else if isFetchingContent}
								<!-- A request in flight is not a failure. It says which
								     attempt it is on, and the RSS body sits underneath so
								     the column is never a blank wait. -->
								<section class="content-section">
									<h3 class="section-label">FULL ARTICLE</h3>
									<p class="pending-notice" data-testid="article-content-pending">
										{pendingLabel}
									</p>
									{#if feed.description}
										<p
											class="section-prose"
											data-testid="article-fallback-summary"
										>{feed.description}</p>
									{/if}
								</section>
							{:else if contentPhase === "failed"}
								<section class="content-section">
									<h3 class="section-label">FULL ARTICLE</h3>
									<div class="error-stripe" role="alert" data-testid="article-content-failed">
										<p>{contentError}</p>
										<!-- Stating the problem without offering a way out is
										     the defect (NN/g heuristic 9). -->
										<div class="remedy-row">
											<button
												type="button"
												class="action-btn"
												onclick={handleRefetchArticle}
												disabled={isFetchingContent}
												data-testid="retry-content"
											>
												{TRY_AGAIN_LABEL}
											</button>
											{#if feed.link}
												<a
													class="remedy-link"
													href={feed.link}
													target="_blank"
													rel="noopener noreferrer"
													data-testid="read-original-link"
												>
													{READ_ORIGINAL_LABEL}
												</a>
											{/if}
										</div>
									</div>
									{#if feed.description}
										<p
											class="section-prose"
											data-testid="article-fallback-summary"
										>{feed.description}</p>
									{/if}
								</section>
							{/if}
						</article>

						<aside class="modal-rail" data-testid="modal-rail" aria-label="Article details">
							{#if feed.excerpt}
								<section class="rail-section">
									<h3 class="section-label">EXCERPT</h3>
									<p class="section-prose rail-prose">{feed.excerpt}</p>
								</section>
							{/if}

							{#if summary}
								<section class="rail-section">
									<h3 class="section-label">AI SUMMARY</h3>
									<div class="section-prose rail-prose">{summary}</div>
								</section>
							{:else if summaryError}
								<section class="rail-section rail-section--error" role="alert">
									<h3 class="section-label">AI SUMMARY</h3>
									<p class="section-prose rail-prose">{summaryError}</p>
								</section>
							{/if}
						</aside>
					</div>
				</div>

				<div class="modal-footer" data-testid="modal-footer">
					<div class="footer-group footer-group--reading">
						<button
							type="button"
							onclick={articleButtonState === 'success' ? handleRefetchArticle : () => handleFetchFullArticle()}
							disabled={articleButtonState === 'loading'}
							class="action-btn"
							class:action-btn--error={articleButtonState === 'error'}
							aria-label={articleLabel.long}
						>
							{#if articleButtonState === 'loading'}
								<span class="loading-pulse"></span>
							{/if}
							<span class="btn-label btn-label--long" aria-hidden="true">{articleLabel.long}</span>
							<span class="btn-label btn-label--short" aria-hidden="true">{articleLabel.short}</span>
						</button>

						<button
							type="button"
							onclick={() => handleSummarize(summaryButtonState === 'success')}
							disabled={summaryButtonState === 'loading' || (!articleContent && summaryButtonState !== 'error' && summaryButtonState !== 'success')}
							class="action-btn action-btn--primary"
							class:action-btn--error={summaryButtonState === 'error'}
							aria-label={summaryLabel.long}
						>
							{#if summaryButtonState === 'loading'}
								<span class="loading-pulse"></span>
							{/if}
							<span class="btn-label btn-label--long" aria-hidden="true">{summaryLabel.long}</span>
							<span class="btn-label btn-label--short" aria-hidden="true">{summaryLabel.short}</span>
						</button>
					</div>

					<div class="footer-group footer-group--chrome">
						{#if onMarkAsRead}
							<button
								type="button"
								class="action-btn"
								onclick={onMarkAsRead}
								disabled={isMarkingAsRead}
								aria-label={markAsReadLabel.long}
							>
								<span class="btn-label btn-label--long" aria-hidden="true">{markAsReadLabel.long}</span>
								<span class="btn-label btn-label--short" aria-hidden="true">{markAsReadLabel.short}</span>
							</button>
						{/if}

						<button
							type="button"
							class="action-btn action-btn--close"
							onclick={requestClose}
						>
							Close
						</button>
					</div>
				</div>
			{/if}
		</DialogPrimitive.Content>
	</DialogPrimitive.Portal>
</DialogPrimitive.Root>
{/if}

<style>
	:global(.modal-content) {
		background: var(--surface-bg);
		border: 1px solid var(--surface-border);
	}

	.nav-arrow {
		position: absolute;
		top: 50%;
		transform: translateY(-50%);
		padding: 0.5rem;
		background: var(--surface-bg);
		border: 1px solid var(--surface-border);
		color: var(--alt-charcoal);
		cursor: pointer;
		z-index: 10;
		transition: background 0.15s;
	}

	.nav-arrow:hover {
		background: var(--surface-hover);
	}

	.nav-arrow--left {
		left: 0.75rem;
	}

	.nav-arrow--right {
		right: 0.75rem;
	}

	.modal-header {
		padding: 1.5rem 3rem;
		border-bottom: 1px solid var(--surface-border);
	}

	/* Phone-only; the footer's Close serves at desktop width. */
	.modal-close {
		display: none;
	}

	.modal-title-link {
		text-decoration: none;
	}

	.modal-title-link:hover {
		text-decoration: underline;
		text-underline-offset: 2px;
	}

	.modal-title {
		font-family: var(--font-display);
		font-size: 1.4rem;
		font-weight: 700;
		color: var(--alt-charcoal);
		line-height: 1.3;
		margin: 0;
	}

	.modal-meta {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		margin-top: 0.4rem;
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-ash);
	}

	.modal-meta-sep {
		color: var(--surface-border);
	}

	.modal-tags {
		display: block;
		margin-top: 0.5rem;
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-ash);
		letter-spacing: 0.04em;
	}

	.modal-body {
		min-height: 0;
		flex: 1;
		overflow-y: auto;
		padding: 1.75rem 3rem;
		background: var(--surface-2);
		container-type: inline-size;
		container-name: modal-body;
	}

	.modal-body-grid {
		display: flex;
		flex-direction: column;
		gap: 1.5rem;
	}

	.modal-article {
		min-width: 0;
	}

	.modal-article > :global(.content-section) {
		width: 100%;
		padding: 1.5rem 2rem;
	}

	.modal-article :global(.article-content) {
		max-width: 100%;
	}

	.modal-rail {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.rail-section {
		padding: 1rem;
		background: var(--surface-bg);
		border: 1px solid var(--surface-border);
		border-top: 3px solid var(--alt-primary);
	}

	.rail-section--error {
		border-top-color: var(--alt-terracotta);
	}

	.rail-prose {
		font-size: 0.85rem;
	}

	@container modal-body (min-width: 960px) {
		.modal-body-grid {
			display: grid;
			grid-template-columns: minmax(0, 1fr) clamp(20rem, 22%, 26rem);
			column-gap: 2.5rem;
			align-items: start;
		}

		.modal-rail {
			position: sticky;
			top: 0;
			align-self: start;
			max-height: calc(88vh - 3rem);
			overflow-y: auto;
		}
	}

	.content-section {
		margin-bottom: 1.5rem;
		padding: 1rem;
		background: var(--surface-bg);
		border: 1px solid var(--surface-border);
	}

	.section-label {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		font-weight: 700;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--alt-ash);
		margin: 0 0 0.5rem;
	}

	.section-prose {
		font-family: var(--font-body);
		font-size: 0.9rem;
		line-height: 1.7;
		color: var(--alt-charcoal);
		white-space: pre-wrap;
		margin: 0;
	}

	.error-stripe {
		margin-bottom: 1.5rem;
		padding: 0.75rem 1rem;
		border-left: 3px solid var(--alt-terracotta);
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-terracotta);
	}

	.pending-notice {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		letter-spacing: 0.06em;
		color: var(--alt-ash);
		margin: 0 0 0.75rem;
	}

	.remedy-row {
		display: flex;
		align-items: center;
		gap: 1rem;
		flex-wrap: wrap;
		margin-top: 0.75rem;
	}

	.remedy-link {
		font-family: var(--font-body);
		font-size: 0.75rem;
		font-weight: 600;
		letter-spacing: 0.03em;
		color: var(--alt-charcoal);
		text-decoration: underline;
		text-underline-offset: 0.2em;
		display: inline-flex;
		align-items: center;
		min-height: 2rem;
	}

	.modal-footer {
		flex-shrink: 0;
		display: flex;
		flex-wrap: wrap;
		gap: 0.75rem;
		align-items: center;
		padding: 0.75rem 3rem;
		border-top: 1px solid var(--surface-border);
	}

	/* Scoped classes rather than the Tailwind utilities that used to sit here:
	   the phone rail needs to restyle these groups, and a utility class carries
	   no hook for that. */
	.footer-group {
		display: flex;
		gap: 0.75rem;
	}

	.footer-group--reading {
		flex: 1;
		min-width: 0;
	}

	.footer-group--chrome {
		flex-shrink: 0;
	}

	/* One of the two label forms is always display:none — see the labels comment
	   in the script block. */
	.btn-label--short {
		display: none;
	}

	.action-btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 0.4rem;
		padding: 0.4rem 1rem;
		min-height: 2.25rem;
		font-family: var(--font-body);
		font-size: 0.75rem;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--alt-charcoal);
		background: transparent;
		border: 1.5px solid var(--alt-charcoal);
		cursor: pointer;
		transition:
			background 0.15s,
			color 0.15s;
	}

	.action-btn:hover:not(:disabled) {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	.action-btn:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}

	.action-btn--primary {
		background: var(--alt-primary);
		color: var(--surface-bg);
		border-color: var(--alt-primary);
	}

	.action-btn--primary:hover:not(:disabled) {
		background: var(--alt-charcoal);
		border-color: var(--alt-charcoal);
	}

	/* Kept above the phone media query on purpose: these and the rail rules have
	   equal specificity, so source order is what decides, and the rail must win
	   below 640px. */
	.action-btn--error {
		color: var(--alt-terracotta);
		border-color: var(--alt-terracotta);
		background: transparent;
	}

	.action-btn--error:hover:not(:disabled) {
		background: var(--alt-terracotta);
		color: var(--surface-bg);
	}

	/*
	 * Phone width. This modal was only ever opened from the desktop grid, so its
	 * 3rem gutters were free; reached from the mobile visual-preview gallery they
	 * eat ~96px of a ~346px dialog and squeeze the article into a column too
	 * narrow to read.
	 *
	 * The prev/next arrows go with them. They are vertically centred over the
	 * body, so at this width they sit on top of the prose rather than beside it,
	 * and the gallery behind the modal is itself the faster way to reach the next
	 * article — closing and tapping costs one gesture more than the arrow did.
	 */
	@media (max-width: 640px) {
		.nav-arrow {
			display: none;
		}

		/* padding-right keeps the headline clear of the close stamp. */
		.modal-header {
			padding: 1rem;
			padding-right: 3.5rem;
		}

		.modal-title {
			font-size: 1.15rem;
		}

		.modal-body {
			padding: 1rem;
		}

		.modal-article > :global(.content-section) {
			padding: 1rem;
		}

		/*
		 * The footer becomes an editorial action rail.
		 *
		 * Alt-Paper sets type on paper: hairline rules, small-caps mono, ink
		 * instead of fills. Three bordered slabs — one of them a solid block of
		 * teal — is the language of a form, not of a page, and at this width they
		 * cost two rows and ~112px of a 700px-tall dialog.
		 *
		 * So: drop the boxes and let the rules do the work. The footer's own
		 * top rule plus one hairline between cells frames three equal columns,
		 * edge to edge, 48px tall — the same rule-and-column device the body's
		 * section labels already use. Primacy is marked the way this file marks
		 * it elsewhere (`.rail-section`): a 3px ink rule along the top of the
		 * cell, not a filled slab.
		 */
		.modal-footer {
			gap: 0;
			padding: 0;
			padding-bottom: env(safe-area-inset-bottom, 0px);
			flex-wrap: nowrap;
			align-items: stretch;
		}

		/* Three equal columns. The flex-grow weights are per *cell*, so the
		   two-cell reading group takes twice the one-cell chrome group and every
		   cell lands on a third — a measured column grid rather than cells sized
		   by how long their label happens to be. */
		.footer-group {
			gap: 0;
			min-width: 0;
		}

		.footer-group--reading {
			flex: 2 1 0;
		}

		.footer-group--chrome {
			flex: 1 1 0;
		}

		.action-btn {
			flex: 1 1 0;
			min-width: 0;
			min-height: 48px;
			padding: 0.5rem 0.25rem;
			border: none;
			border-left: 1px solid var(--surface-border);
			background: transparent;
			color: var(--alt-charcoal);
			font-family: var(--font-mono);
			font-size: 0.62rem;
			font-weight: 700;
			letter-spacing: 0.06em;
			white-space: nowrap;
		}

		/* The leftmost cell butts against the dialog edge; only the seams get a
		   rule. `.footer-group--reading` is always first, so its first child is
		   the rail's first cell. */
		.footer-group--reading .action-btn:first-child {
			border-left: none;
		}

		.action-btn:hover:not(:disabled) {
			background: transparent;
			color: var(--alt-charcoal);
		}

		/* Touch has no hover; the press is the only feedback, so it inverts. */
		.action-btn:active:not(:disabled) {
			background: var(--alt-charcoal);
			color: var(--surface-bg);
		}

		.action-btn--primary {
			background: transparent;
			color: var(--alt-primary);
			box-shadow: inset 0 3px 0 var(--alt-primary);
		}

		.action-btn--primary:hover:not(:disabled) {
			background: transparent;
			color: var(--alt-primary);
		}

		.action-btn--primary:active:not(:disabled) {
			background: var(--alt-primary);
			color: var(--surface-bg);
		}

		.action-btn--error {
			color: var(--alt-terracotta);
			box-shadow: inset 0 3px 0 var(--alt-terracotta);
		}

		/* Close lives in the header at this width. */
		.action-btn--close {
			display: none;
		}

		.btn-label--long {
			display: none;
		}

		.btn-label--short {
			display: inline;
		}

		/*
		 * Header close: a 48px target flush to the modal's top-right corner, in
		 * the same press-mark idiom as the swipe card's keep stamp — opaque paper
		 * ground, hairline edges, no radius.
		 */
		.modal-close {
			position: absolute;
			top: 0;
			right: 0;
			z-index: 10;
			display: inline-flex;
			align-items: center;
			justify-content: center;
			width: 48px;
			height: 48px;
			background: var(--surface-bg);
			border: none;
			border-left: 1px solid var(--surface-border);
			border-bottom: 1px solid var(--surface-border);
			color: var(--alt-charcoal);
			cursor: pointer;
			touch-action: manipulation;
		}

		.modal-close:active {
			background: var(--alt-charcoal);
			color: var(--surface-bg);
		}

	}

	.loading-pulse {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: currentColor;
		animation: pulse 1.2s ease-in-out infinite;
	}

	@keyframes pulse {
		0%,
		100% {
			opacity: 0.3;
		}
		50% {
			opacity: 1;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.loading-pulse {
			animation: none;
			opacity: 1;
		}
	}
</style>
