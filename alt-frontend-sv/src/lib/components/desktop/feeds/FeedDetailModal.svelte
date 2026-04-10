<script lang="ts">
import { ChevronLeft, ChevronRight, Volume2, Square } from "@lucide/svelte";
import { getFeedContentOnTheFlyClient } from "$lib/api/client/articles";
import RenderFeedDetails from "$lib/components/mobile/RenderFeedDetails.svelte";
import { Dialog as DialogPrimitive } from "bits-ui";
import {
	createClientTransport,
	streamSummarizeWithAbortAdapter,
} from "$lib/connect";
import type { Snippet } from "svelte";
import type { RenderFeed } from "$lib/schema/feed";
import { articlePrefetcher } from "$lib/utils/articlePrefetcher";
import { isTransientError } from "$lib/utils/errorClassification";
import { processArticleFetchResponse } from "./FeedDetailModal.logic";
import { useTtsPlayback } from "$lib/hooks/useTtsPlayback.svelte";

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
	footerActions?: Snippet;
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
	footerActions,
}: Props = $props();

// TTS playback
const tts = useTtsPlayback();

function handleTtsClick() {
	if (tts.isPlaying || tts.isLoading) {
		tts.stop();
	} else if (summary) {
		tts.play(summary, { speed: 1.25 });
	}
}

// Content fetching state
let isFetchingContent = $state(false);
let articleContent = $state<string | null>(null);
let articleID = $state<string | null>(null);
let contentError = $state<string | null>(null);

// AI summary state
let isSummarizing = $state(false);
let summary = $state<string | null>(null);
let summaryError = $state<string | null>(null);
let abortController = $state<AbortController | null>(null);

// Content fetch abort controller
let contentAbortController = $state<AbortController | null>(null);

// Retry counters
let contentRetryCount = $state(0);
let summaryRetryCount = $state(0);

// Derived button states
const articleButtonState = $derived.by(() => {
	if (isFetchingContent) return "loading" as const;
	if (articleContent) return "success" as const;
	if (contentError) return "error" as const;
	return "idle" as const;
});

const summaryButtonState = $derived.by(() => {
	if (isSummarizing) return "loading" as const;
	if (summaryError) return "error" as const;
	if (summary) return "success" as const;
	return "idle" as const;
});

// Track previous feed URL to detect actual feed changes
let previousFeedUrl = $state<string | null>(null);

// Cleanup on modal close
$effect(() => {
	if (!open) {
		// Stop TTS playback
		tts.stop();
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
		isFetchingContent = false;
		isSummarizing = false;
		contentError = null;
		summaryError = null;
		contentRetryCount = 0;
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

	// Stop TTS playback
	tts.stop();
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
	isFetchingContent = false;
	isSummarizing = false;
	contentError = null;
	summaryError = null;
	contentRetryCount = 0;
	summaryRetryCount = 0;
});

// Auto-fetch article content when modal opens
$effect(() => {
	if (!open || !feed) return;
	if (!feed.normalizedUrl) {
		if (!contentError) {
			contentError = "Article URL is not available";
		}
		return;
	}
	if (!articleContent && !isFetchingContent && !contentError) {
		handleFetchFullArticle();
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
	await handleFetchFullArticle(true);
}

async function handleFetchFullArticle(forceRefresh = false) {
	if (isFetchingContent) return;
	if (!feed?.normalizedUrl) {
		contentError = "Article URL is not available";
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
			return;
		}
	}

	// Cancel previous content fetch request
	if (contentAbortController) {
		contentAbortController.abort();
	}
	contentAbortController = new AbortController();

	isFetchingContent = true;
	contentError = null;

	try {
		// Use normalizedUrl for API call (consistent with prefetcher)
		const response = await getFeedContentOnTheFlyClient(targetFeedUrl, {
			signal: contentAbortController.signal,
			forceRefresh,
		});

		// Defensive validation: discard stale response if feed changed
		if (feed.normalizedUrl !== targetFeedUrl) return;

		const result = processArticleFetchResponse(response);
		articleContent = result.articleContent;
		articleID = result.articleID;
		if (result.contentError) {
			contentError = result.contentError;
		}
	} catch (err) {
		// Ignore AbortError and ConnectError wrapping abort (user cancelled)
		if (err instanceof Error) {
			if (err.name === "AbortError") return;
			if (err.message.includes("abort") || err.message.includes("cancel"))
				return;
		}

		if (feed.normalizedUrl !== targetFeedUrl) return;

		// Auto-retry for transient errors (1 attempt only)
		if (isTransientError(err) && contentRetryCount < 1) {
			contentRetryCount++;
			try {
				await new Promise((resolve) => setTimeout(resolve, 500));
				if (feed.normalizedUrl !== targetFeedUrl) return;

				contentAbortController = new AbortController();
				const response = await getFeedContentOnTheFlyClient(targetFeedUrl, {
					signal: contentAbortController.signal,
				});

				if (feed.normalizedUrl !== targetFeedUrl) return;

				const retryResult = processArticleFetchResponse(response);
				articleContent = retryResult.articleContent;
				articleID = retryResult.articleID;
				if (retryResult.contentError) {
					contentError = retryResult.contentError;
				}
				return;
			} catch (retryErr) {
				if (retryErr instanceof Error) {
					if (retryErr.name === "AbortError") return;
					if (
						retryErr.message.includes("abort") ||
						retryErr.message.includes("cancel")
					)
						return;
				}
				if (feed.normalizedUrl === targetFeedUrl) {
					contentError =
						retryErr instanceof Error
							? retryErr.message
							: "Failed to fetch article";
				}
				return;
			}
		}

		contentError =
			err instanceof Error ? err.message : "Failed to fetch article";
	} finally {
		isFetchingContent = false;
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
			{}, // No typewriter effect for desktop
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
			class="modal-content fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 w-[75vw] sm:max-w-[1800px] h-[75vh] overflow-hidden flex flex-col z-50"
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
					{#if feed.excerpt}
						<section class="content-section">
							<h3 class="section-label">EXCERPT</h3>
							<p class="section-prose">{feed.excerpt}</p>
						</section>
					{/if}

					{#if articleContent}
						<section class="content-section">
							<h3 class="section-label">FULL ARTICLE</h3>
							<RenderFeedDetails
								feedDetails={articleContent ? { content: articleContent, article_id: articleID ?? "", og_image_url: "", og_image_proxy_url: "" } : null}
								error={contentError}
							/>
						</section>
					{:else if contentError}
						<div class="error-stripe" role="alert">
							<p>{contentError}</p>
						</div>
					{/if}

					{#if summary}
						<section class="content-section">
							<div class="flex items-center justify-between">
								<h3 class="section-label">AI SUMMARY</h3>
								{#if !isSummarizing}
									<button
										onclick={handleTtsClick}
										class="tts-button"
										aria-label={tts.isPlaying ? "Stop reading" : tts.isLoading ? "Cancel loading" : "Read aloud"}
										title={tts.isPlaying ? "Stop reading" : tts.isLoading ? "Cancel loading" : "Read aloud"}
									>
										{#if tts.isLoading}
											<span class="loading-pulse"></span>
										{:else if tts.isPlaying}
											<Square class="h-4 w-4" />
										{:else}
											<Volume2 class="h-4 w-4" />
										{/if}
									</button>
								{/if}
							</div>
							<div class="section-prose">{summary}</div>
							{#if tts.error}
								<p class="tts-error">{tts.error}</p>
							{/if}
						</section>
					{:else if summaryError}
						<div class="error-stripe" role="alert">
							<p>{summaryError}</p>
						</div>
					{/if}
				</div>

				<div class="modal-footer">
					<div class="flex gap-3 flex-1 min-w-0">
						<button
							onclick={articleButtonState === 'success' ? handleRefetchArticle : () => handleFetchFullArticle()}
							disabled={articleButtonState === 'loading'}
							class="action-btn"
							class:action-btn--error={articleButtonState === 'error'}
						>
							{#if articleButtonState === 'loading'}
								<span class="loading-pulse"></span>
								<span>Loading&hellip;</span>
							{:else if articleButtonState === 'success'}
								<span>Re-fetch Article</span>
							{:else if articleButtonState === 'error'}
								<span>Try Again</span>
							{:else}
								<span>Full Article</span>
							{/if}
						</button>

						<button
							onclick={() => handleSummarize(summaryButtonState === 'success')}
							disabled={summaryButtonState === 'loading' || (!articleContent && summaryButtonState !== 'error' && summaryButtonState !== 'success')}
							class="action-btn action-btn--primary"
							class:action-btn--error={summaryButtonState === 'error'}
						>
							{#if summaryButtonState === 'loading'}
								<span class="loading-pulse"></span>
								<span>Summarizing&hellip;</span>
							{:else if summaryButtonState === 'error'}
								<span>Try Again</span>
							{:else if summaryButtonState === 'success'}
								<span>Re-summarize</span>
							{:else}
								<span>Summarize</span>
							{/if}
						</button>
					</div>

					<div class="flex gap-3 flex-shrink-0">
						{#if footerActions}
							{@render footerActions()}
						{/if}

						<DialogPrimitive.Close class="action-btn">
							Close
						</DialogPrimitive.Close>
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
		padding: 1.5rem 4.5rem;
		border-bottom: 1px solid var(--surface-border);
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
		padding: 1.5rem 4.5rem;
		background: var(--surface-2);
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

	.tts-button {
		padding: 0.4rem;
		background: transparent;
		border: none;
		color: var(--alt-ash);
		cursor: pointer;
		transition: color 0.15s;
	}

	.tts-button:hover {
		color: var(--alt-charcoal);
	}

	.tts-error {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-terracotta);
		margin-top: 0.4rem;
	}

	.modal-footer {
		flex-shrink: 0;
		display: flex;
		flex-wrap: wrap;
		gap: 0.75rem;
		align-items: center;
		padding: 0.75rem 4.5rem;
		border-top: 1px solid var(--surface-border);
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

	.action-btn--error {
		color: var(--alt-terracotta);
		border-color: var(--alt-terracotta);
		background: transparent;
	}

	.action-btn--error:hover:not(:disabled) {
		background: var(--alt-terracotta);
		color: var(--surface-bg);
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
