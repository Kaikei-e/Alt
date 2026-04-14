<script lang="ts">
import { tick, untrack } from "svelte";
import { fade } from "svelte/transition";
import { browser } from "$app/environment";
import {
	type FeedContentOnTheFlyResponse,
	type FetchArticleSummaryResponse,
	getArticleSummaryClient,
	getFeedContentOnTheFlyClient,
	registerFavoriteFeedClient,
	summarizeArticleClient,
} from "$lib/api/client";
import * as Sheet from "$lib/components/ui/sheet";
import {
	createClientTransport,
	streamSummarizeWithAbortAdapter,
} from "$lib/connect";
import { isTransientError } from "$lib/utils/errorClassification";
import RenderFeedDetails from "./RenderFeedDetails.svelte";

interface Props {
	feedURL?: string;
	feedTitle?: string;
	initialData?: FetchArticleSummaryResponse | FeedContentOnTheFlyResponse;
	open?: boolean;
	onOpenChange?: (open: boolean) => void;
	showButton?: boolean;
}

const {
	feedURL,
	feedTitle,
	initialData,
	open: openProp,
	onOpenChange,
	showButton = true,
}: Props = $props();

// Initialize with default, sync with prop in $effect
let isOpen = $state(false);
let isLoading = $state(false);
let isFavoriting = $state(false);
let error = $state<string | null>(null);
let isBookmarked = $state(false);
let summary = $state<string | null>(null);
let summaryError = $state<string | null>(null);
let isSummarizing = $state(false);
let abortController = $state<AbortController | null>(null);
// Initialize state from props (props are immutable, so this is safe)
let articleSummary = $state<FetchArticleSummaryResponse | null>(
	(() => {
		if (initialData && "matched_articles" in initialData) {
			return initialData as FetchArticleSummaryResponse;
		}
		return null;
	})(),
);
let feedDetails = $state<FeedContentOnTheFlyResponse | null>(
	(() => {
		if (initialData && "content" in initialData) {
			return initialData as FeedContentOnTheFlyResponse;
		}
		return null;
	})(),
);

// Retry counters
let contentRetryCount = $state(0);
let summaryRetryCount = $state(0);

// Derived button state for summary
const summaryButtonState = $derived.by(() => {
	if (isSummarizing) return "loading" as const;
	if (summaryError && !summary) return "error" as const;
	if (summary) return "success" as const;
	return "idle" as const;
});

// Create unique test ID based on feedURL (capture initial value)
const uniqueId = $derived(feedURL ? btoa(feedURL).slice(0, 8) : "default");

// Sync with external open prop - use $effect to track prop changes
$effect(() => {
	// Access openProp inside $effect to track changes
	if (openProp !== undefined && openProp !== isOpen) {
		isOpen = openProp;
	}
});

// Auto-fetch data when opened externally (e.g., from ViewedFeedCard)
$effect(() => {
	// Only trigger when:
	// 1. Modal is opening (isOpen becomes true)
	// 2. No initial data provided
	// 3. Data not already loaded
	// 4. Not currently loading
	if (
		isOpen &&
		!initialData &&
		!articleSummary &&
		!feedDetails &&
		!isLoading &&
		feedURL
	) {
		// Fetch data when opened externally
		fetchData();
	}
});

// Sync internal state to external
$effect(() => {
	if (onOpenChange && isOpen !== (openProp ?? false)) {
		onOpenChange(isOpen);
	}
});

// Handle escape key to close modal
$effect(() => {
	if (!browser || !isOpen) return;

	const handleEscape = (event: KeyboardEvent) => {
		if (event.key === "Escape" && isOpen) {
			handleHideDetails();
		}
	};

	document.addEventListener("keydown", handleEscape);

	return () => {
		document.removeEventListener("keydown", handleEscape);
	};
});

// Cleanup abort controller on destroy
$effect(() => {
	return () => {
		if (abortController) {
			abortController.abort();
		}
	};
});

// Reset state when feedURL changes (handling swipes)
let previousFeedUrl = $state(untrack(() => feedURL));
$effect(() => {
	if (feedURL !== previousFeedUrl) {
		// Abort any ongoing summarization
		if (abortController) {
			abortController.abort();
			abortController = null;
		}

		// Reset all article-specific state
		summary = null;
		summaryError = null;
		isSummarizing = false;
		articleSummary = null;
		feedDetails = null;
		isFavoriting = false;
		contentRetryCount = 0;
		summaryRetryCount = 0;

		// Update tracker
		previousFeedUrl = feedURL;
	}
});

const handleHideDetails = () => {
	isOpen = false;
	if (onOpenChange) {
		onOpenChange(false);
	}
	if (abortController) {
		abortController.abort();
		abortController = null;
	}
};

// Reusable function to fetch article data
const fetchData = async () => {
	if (!feedURL) {
		error = "No feed URL available";
		return;
	}

	isLoading = true;
	error = null;

	// Fetch both summary and content independently
	const summaryPromise = getArticleSummaryClient(feedURL).catch((err) => {
		console.error("Error fetching article summary:", err);
		return null;
	});

	const detailsPromise = getFeedContentOnTheFlyClient(feedURL).catch((err) => {
		console.error("Error fetching article content:", err);
		return null;
	});

	try {
		const [summaryResult, detailsResult] = await Promise.all([
			summaryPromise,
			detailsPromise,
		]);

		// Check if summary has valid content
		const hasValidSummary =
			summaryResult?.matched_articles &&
			summaryResult.matched_articles.length > 0;
		// Check if details has valid content
		const hasValidDetails =
			detailsResult?.content && detailsResult.content.trim() !== "";

		if (hasValidSummary) {
			articleSummary = summaryResult;
		}

		if (hasValidDetails) {
			feedDetails = detailsResult;
		}

		// If neither API call succeeded with valid content, show error
		if (!hasValidSummary && !hasValidDetails) {
			error = "Unable to fetch article content";
		}
	} catch (err) {
		console.error("Unexpected error:", err);
		error = "Unexpected error occurred";
	} finally {
		isLoading = false;
	}
};

const handleShowDetails = async () => {
	// If we already have initial data, just open the modal
	if (initialData) {
		isOpen = true;
		return;
	}

	await fetchData();
	isOpen = true;
	if (onOpenChange) {
		onOpenChange(true);
	}
};

async function handleSummarize(forceRefresh = false) {
	if (!feedURL || isSummarizing) return;

	if (abortController) {
		abortController.abort();
	}

	isSummarizing = true;
	summaryError = null;
	summary = ""; // Reset summary

	try {
		const transport = createClientTransport();
		abortController = streamSummarizeWithAbortAdapter(
			transport,
			{
				feedUrl: feedURL,
				articleId: articleSummary?.matched_articles?.[0]?.source_id,
				title: feedTitle,
				forceRefresh,
			},
			(chunk) => {
				summary = (summary || "") + chunk;
			},
			{
				tick,
				typewriter: true,
				typewriterDelay: 10, // 10ms delay ~100 chars/sec for responsive reading
				onChunk: (chunkCount) => {
					// Hide "Summarizing..." when first chunk arrives
					if (chunkCount === 1) {
						isSummarizing = false;
						// Scroll to summary section after first chunk
						setTimeout(() => {
							const summaryEl = document.getElementById("summary-section");
							summaryEl?.scrollIntoView({ behavior: "smooth", block: "start" });
						}, 100);
					}
				},
			},
			(_result) => {
				// onComplete
				isSummarizing = false;
				abortController = null;
			},
			async (error) => {
				// onError
				if (error.name === "AbortError") {
					console.log("[StreamSummarize] Stream aborted by user");
					return;
				}

				const errorMessage = error.message;
				const isAuthError =
					errorMessage.includes("403") ||
					errorMessage.includes("401") ||
					errorMessage.includes("Forbidden") ||
					errorMessage.includes("Authentication") ||
					errorMessage.includes("unauthenticated");

				console.error("[StreamSummarize] Error streaming summary:", {
					error: errorMessage,
					isAuthError,
					hasPartialData: !!summary && summary.length > 0,
				});

				// Don't retry on authentication errors - user needs to re-authenticate
				if (isAuthError) {
					summaryError =
						"Authentication failed. Please refresh the page and try again.";
					isSummarizing = false;
					abortController = null;
					return;
				}

				// If we have partial data, don't fallback - show what we have
				if (summary && summary.length > 0) {
					console.warn(
						"[StreamSummarize] Using partial summary due to stream error",
					);
					summaryError = "Stream interrupted. Summary may be incomplete.";
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

				// Fallback to legacy endpoint only if no data was received
				console.log("[StreamSummarize] Falling back to legacy endpoint");
				try {
					const result = await summarizeArticleClient(feedURL);
					const trimmedSummary = result.summary?.trim();

					if (trimmedSummary) {
						summary = trimmedSummary;
						summaryError = null;
					} else {
						summaryError = "Failed to get summary. Please try again.";
					}
				} catch (fallbackErr) {
					console.error(
						"[StreamSummarize] Legacy endpoint also failed:",
						fallbackErr,
					);
					summaryError = "Failed to summarize article. Please try again.";
				}
				isSummarizing = false;
				abortController = null;
			},
		);
	} catch (e) {
		// Synchronous error during setup
		console.error("[StreamSummarize] Setup error:", e);
		summaryError = "Failed to start summarization. Please try again.";
		isSummarizing = false;
		abortController = null;
	}
}
</script>

{#if showButton && !isOpen}
	<button
		class="show-details-btn"
		onclick={handleShowDetails}
		data-testid="show-details-button-{uniqueId}"
		disabled={isLoading}
	>
		{isLoading ? "Loading\u2026" : "Show Details"}
	</button>
{/if}

<Sheet.Root
	bind:open={isOpen}
	onOpenChange={(open: boolean) => {
		if (!open) handleHideDetails();
	}}
>
	<Sheet.Content
		side="bottom"
		class="sheet-content max-w-[500px] h-[85dvh] flex flex-col overflow-hidden p-0"
	>
		<Sheet.Header class="sheet-header">
			<Sheet.Title class="sheet-title">
				{feedTitle || "Article Details"}
			</Sheet.Title>
		</Sheet.Header>

		<div class="sheet-body scrollable-content" id="summary-content">
			{#if feedDetails || articleSummary}
				<RenderFeedDetails
					feedDetails={feedDetails ?? articleSummary}
					isLoading={false}
					error={null}
				/>
			{:else}
				<RenderFeedDetails
					feedDetails={articleSummary || feedDetails}
					{isLoading}
					{error}
				/>
			{/if}

			{#if summary}
				<div
					id="summary-section"
					class="summary-section"
					transition:fade={{ duration: 200 }}
				>
					<h3 class="section-label">AI SUMMARY</h3>
					<p class="summary-prose">{summary}</p>
				</div>
			{/if}

			{#if summaryError}
				<div class="error-stripe" role="alert">
					{summaryError}
				</div>
			{/if}
		</div>

		<Sheet.Footer class="sheet-footer">
			<button
				class="action-btn"
				class:action-btn--success={isBookmarked}
				onclick={async () => {
					if (!feedURL || isBookmarked) return;
					isFavoriting = true;
					try {
						await registerFavoriteFeedClient(feedURL);
						isBookmarked = true;
					} catch (e) {
						console.error("Failed to favorite feed", e);
					} finally {
						isFavoriting = false;
					}
				}}
				disabled={isFavoriting || isBookmarked}
			>
				{isFavoriting ? "Saving\u2026" : isBookmarked ? "Favorited" : "Favorite"}
			</button>

			<button
				class="action-btn action-btn--primary"
				class:action-btn--error={summaryButtonState === 'error'}
				onclick={() => handleSummarize(summaryButtonState === 'success')}
				disabled={isSummarizing}
			>
				{#if summaryButtonState === 'loading'}
					<span class="loading-pulse"></span>
					Summarizing
				{:else if summaryButtonState === 'error'}
					Try Again
				{:else if summaryButtonState === 'success'}
					Re-summarize
				{:else}
					Summarize
				{/if}
			</button>
		</Sheet.Footer>
	</Sheet.Content>
</Sheet.Root>

<style>
	.show-details-btn {
		font-family: var(--font-body);
		font-size: 0.75rem;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--alt-charcoal);
		background: transparent;
		border: 1.5px solid var(--alt-charcoal);
		padding: 0.4rem 0.75rem;
		min-height: 44px;
		cursor: pointer;
		transition: background 0.15s, color 0.15s;
	}

	.show-details-btn:active {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	.show-details-btn:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}

	:global(.sheet-content) {
		background: var(--surface-bg) !important;
		border: 1px solid var(--surface-border) !important;
		border-radius: 0 !important;
		box-shadow: none !important;
	}

	:global(.sheet-header) {
		padding: 1rem !important;
		padding-inline-end: 2.5rem !important;
		background: var(--surface-bg) !important;
		border-bottom: 1px solid var(--surface-border) !important;
		flex-shrink: 0;
	}

	:global(.sheet-title) {
		font-family: var(--font-display) !important;
		font-size: 1.1rem !important;
		font-weight: 700 !important;
		color: var(--alt-charcoal) !important;
		line-height: 1.3 !important;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow-wrap: break-word;
	}

	.sheet-body {
		flex: 1;
		min-height: 0;
		overflow-y: auto;
		padding: 1rem;
		background: var(--surface-2);
	}

	.summary-section {
		margin-top: 1.5rem;
		padding: 1rem;
		border: 1px solid var(--surface-border);
		background: var(--surface-bg);
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

	.summary-prose {
		font-family: var(--font-body);
		font-size: 0.9rem;
		line-height: 1.7;
		color: var(--alt-charcoal);
		white-space: pre-wrap;
		margin: 0;
	}

	.error-stripe {
		margin-top: 1rem;
		padding: 0.75rem 1rem;
		border-left: 3px solid var(--alt-terracotta);
		font-family: var(--font-body);
		font-size: 0.82rem;
		color: var(--alt-terracotta);
	}

	:global(.sheet-footer) {
		padding: 0.75rem 1rem !important;
		background: var(--surface-bg) !important;
		border-top: 1px solid var(--surface-border) !important;
		flex-shrink: 0;
		display: flex !important;
		flex-direction: row !important;
		justify-content: flex-end !important;
		gap: 0.75rem !important;
	}

	.action-btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 0.4rem;
		padding: 0.4rem 0.75rem;
		min-height: 44px;
		min-width: 100px;
		font-family: var(--font-body);
		font-size: 0.75rem;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--alt-charcoal);
		background: transparent;
		border: 1.5px solid var(--alt-charcoal);
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

	.action-btn--primary {
		background: var(--alt-primary);
		color: var(--surface-bg);
		border-color: var(--alt-primary);
		min-width: 120px;
	}

	.action-btn--primary:active:not(:disabled) {
		background: var(--alt-charcoal);
		border-color: var(--alt-charcoal);
	}

	.action-btn--error {
		color: var(--alt-terracotta);
		border-color: var(--alt-terracotta);
		background: transparent;
	}

	.action-btn--success {
		color: var(--alt-sage);
		border-color: var(--alt-sage);
	}

	.loading-pulse {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: currentColor;
		animation: pulse 1.2s ease-in-out infinite;
	}

	/* Scrollable content styling */
	:global(.scrollable-content) {
		-webkit-font-smoothing: antialiased;
		-moz-osx-font-smoothing: grayscale;
		text-rendering: optimizeLegibility;
	}

	:global(.scrollable-content p) {
		line-height: 1.7;
		margin-bottom: 1em;
	}

	:global(.scrollable-content h1),
	:global(.scrollable-content h2),
	:global(.scrollable-content h3) {
		font-weight: 700;
		color: var(--alt-charcoal);
		margin-top: 1.5em;
		margin-bottom: 0.5em;
	}

	:global(.scrollable-content::-webkit-scrollbar) {
		width: 6px;
	}

	:global(.scrollable-content::-webkit-scrollbar-track) {
		background: var(--surface-2);
	}

	:global(.scrollable-content::-webkit-scrollbar-thumb) {
		background: var(--alt-ash);
	}

	@keyframes pulse {
		0%, 100% { opacity: 0.3; }
		50% { opacity: 1; }
	}

	@media (prefers-reduced-motion: reduce) {
		.loading-pulse {
			animation: none;
			opacity: 1;
		}
	}
</style>
