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

let {
	feedURL,
	feedTitle,
	initialData,
	open = $bindable(false),
	onOpenChange,
	showButton = true,
}: Props = $props();

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

// Bounded auto-retry policy for the on-open content fetch.
//
// Transient-only with a hard attempt ceiling. The shape follows
// $lib/utils/loadProxyImage — a permanent failure is terminal on the first
// attempt, a transient one buys a finite number of retries from a backoff
// table — and the budget is deliberately identical to the one
// SwipeFeedCard.svelte already applies to getFeedContentOnTheFlyClient
// (`contentRetryCount < 1` + a 500ms delay): the two mobile content paths hit
// the same endpoints, so they must not retry at different rates.
const CONTENT_RETRY_BACKOFFS_MS: readonly number[] = [500];
const MAX_CONTENT_ATTEMPTS = CONTENT_RETRY_BACKOFFS_MS.length + 1;

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

// Attempts actually spent by the current/last content fetch. Drives the
// terminal message, so it must be the real count and never the ceiling: the
// budget is abandoned early on a permanent failure.
let contentAttemptCount = $state(0);
let summaryRetryCount = $state(0);
// Terminal state for the content fetch: the attempt budget is spent (or the
// failure was permanent) and nothing will be retried until the user asks.
let contentFetchExhausted = $state(false);
// Monotonic token identifying the newest content fetch. A fetch only applies
// its result while it is still the newest one — the stale-response guard for
// swipe-to-next-article, where a backed-off retry can outlive its article.
let contentFetchToken = 0;
// Tracker for the feedURL the current state belongs to. Plain `let`, not
// `$state`: it is only ever read from inside untrack(), and making it
// reactive would put it back in the auto-fetch effect's dependency set.
let previousFeedUrl = untrack(() => feedURL);

// What the terminal content-failure stripe announces. It reports the attempts
// actually spent, never MAX_CONTENT_ATTEMPTS: a permanent failure (or an
// empty-but-successful response) is terminal after a single attempt, so
// hard-coding the ceiling would state something untrue to every user — and,
// since the stripe is role="alert", read it out to screen readers.
const contentFailureMessage = $derived(
	`Stopped after ${contentAttemptCount} ${
		contentAttemptCount === 1 ? "attempt" : "attempts"
	}.`,
);

// Derived button state for summary
const summaryButtonState = $derived.by(() => {
	if (isSummarizing) return "loading" as const;
	if (summaryError && !summary) return "error" as const;
	if (summary) return "success" as const;
	return "idle" as const;
});

// Create unique test ID based on feedURL. encodeURIComponent first: btoa only
// accepts Latin1 and throws InvalidCharacterError on non-ASCII URLs (e.g. Japanese).
const uniqueId = $derived(
	feedURL ? btoa(encodeURIComponent(feedURL)).slice(0, 8) : "default",
);

// Reset per-article state on a feedURL change (handling swipes), then
// auto-fetch when the sheet is open (e.g., from ViewedFeedCard).
//
// The tracked dependencies are deliberately ONLY `open` and `feedURL` — the
// two things that should ever start a fetch. Every value the fetch itself
// writes (isLoading / articleSummary / feedDetails / contentFetchExhausted)
// is read under untrack(). $effect dependency tracking crosses the call
// stack, so while those reads were tracked, `isLoading = false` in
// fetchData()'s finally block re-armed the very effect that had called it
// whenever the fetch came back with nothing to show: guard passes → fetch →
// fails → guard passes again, an unbounded refetch loop measured at ~4,000
// console.error/sec that ran until the tab hit its memory ceiling.
// See .claude/skills/bp-svelte principle 9.
//
// The reset lives here rather than in its own effect so the ordering is
// explicit: an untracked auto-fetch cannot be re-run by a reset that happens
// to be scheduled after it.
$effect(() => {
	const url = feedURL;
	const isOpen = open;

	untrack(() => {
		if (url !== previousFeedUrl) {
			resetForNewArticle();
			previousFeedUrl = url;
		}

		// Only trigger when:
		// 1. Modal is open
		// 2. No initial data provided
		// 3. Data not already loaded
		// 4. Not currently loading
		// 5. The bounded retry budget has not already been spent
		if (
			isOpen &&
			url &&
			!initialData &&
			!articleSummary &&
			!feedDetails &&
			!isLoading &&
			!contentFetchExhausted
		) {
			void fetchData();
		}
	});
});

// Notify the optional callback prop whenever `open` changes, whether the
// change came from the parent (via bind:open) or from internal handlers.
$effect(() => {
	onOpenChange?.(open);
});

// Handle escape key to close modal
$effect(() => {
	if (!browser || !open) return;

	const handleEscape = (event: KeyboardEvent) => {
		if (event.key === "Escape" && open) {
			handleHideDetails();
		}
	};

	document.addEventListener("keydown", handleEscape);

	return () => {
		document.removeEventListener("keydown", handleEscape);
	};
});

// Cleanup on destroy
$effect(() => {
	return () => {
		if (abortController) {
			abortController.abort();
		}
		// Invalidate any in-flight content fetch. Without this a retry parked
		// in sleep() outlives the component: it wakes after unmount, spends the
		// rest of its budget on requests nobody is waiting for, and writes
		// $state on a destroyed instance.
		contentFetchToken++;
	};
});

// Reset all article-specific state. Called from the effect above when the
// feedURL changes (handling swipes).
const resetForNewArticle = () => {
	// Abort any ongoing summarization
	if (abortController) {
		abortController.abort();
		abortController = null;
	}

	// Invalidate any in-flight content fetch so a late attempt — or one
	// sleeping between bounded retries — cannot write the previous article's
	// content or error into this one.
	contentFetchToken++;

	summary = null;
	summaryError = null;
	isSummarizing = false;
	articleSummary = null;
	feedDetails = null;
	isFavoriting = false;
	isLoading = false;
	error = null;
	contentAttemptCount = 0;
	summaryRetryCount = 0;
	contentFetchExhausted = false;
};

const handleHideDetails = () => {
	open = false;
	if (abortController) {
		abortController.abort();
		abortController = null;
	}
};

// A single attempt at both endpoints. Reports what came back and whether any
// failure looked transient, so the caller can decide whether it is worth
// spending an attempt from the budget.
const attemptFetch = async (url: string) => {
	let sawTransient = false;

	// Fetch both summary and content independently
	const summaryPromise = getArticleSummaryClient(url).catch((err: unknown) => {
		sawTransient = sawTransient || isTransientError(err);
		console.error("Error fetching article summary:", err);
		return null;
	});

	const detailsPromise = getFeedContentOnTheFlyClient(url).catch(
		(err: unknown) => {
			sawTransient = sawTransient || isTransientError(err);
			console.error("Error fetching article content:", err);
			return null;
		},
	);

	const [summaryResult, detailsResult] = await Promise.all([
		summaryPromise,
		detailsPromise,
	]);

	return {
		summaryResult,
		detailsResult,
		// Check if summary has valid content
		hasValidSummary: Boolean(
			summaryResult?.matched_articles &&
				summaryResult.matched_articles.length > 0,
		),
		// Check if details has valid content
		hasValidDetails: Boolean(
			detailsResult?.content && detailsResult.content.trim() !== "",
		),
		sawTransient,
	};
};

// Reusable function to fetch article data, with a bounded retry.
//
// At most MAX_CONTENT_ATTEMPTS attempts, and only a transient failure buys a
// retry at all; anything else is terminal immediately. When the budget runs
// out the component parks in `contentFetchExhausted` and waits for the user
// instead of retrying on its own — nothing in here may leave a state that
// re-arms the auto-fetch effect.
const fetchData = async () => {
	if (!feedURL) {
		error = "No feed URL available";
		return;
	}
	// A fetch is already in flight; joining it would double the request rate.
	if (isLoading) return;

	const url = feedURL;
	const token = ++contentFetchToken;

	isLoading = true;
	error = null;
	contentAttemptCount = 0;
	contentFetchExhausted = false;

	try {
		// Counted loop, not `for (;;)`: the ceiling is structural, so the bound
		// holds even if the backoff table is later edited into something that
		// never runs out.
		for (let attemptNo = 1; attemptNo <= MAX_CONTENT_ATTEMPTS; attemptNo++) {
			const attempt = await attemptFetch(url);

			// Stale-response guard: a newer fetch (or a feedURL change) took over
			// while this attempt was in flight. Drop the result on the floor.
			if (token !== contentFetchToken) return;

			contentAttemptCount = attemptNo;

			if (attempt.hasValidSummary) {
				articleSummary = attempt.summaryResult;
			}
			if (attempt.hasValidDetails) {
				feedDetails = attempt.detailsResult;
			}
			if (attempt.hasValidSummary || attempt.hasValidDetails) return;

			// Neither API call came back with valid content. Retry only a
			// transient failure, and only while the budget has an entry left.
			const backoffMs = attempt.sawTransient
				? CONTENT_RETRY_BACKOFFS_MS[attemptNo - 1]
				: undefined;

			// Permanent failure, or the budget is spent: stop here, having made
			// attemptNo attempts — which is what the user is told.
			if (backoffMs === undefined) break;

			await sleep(backoffMs);
			if (token !== contentFetchToken) return;
		}

		error = "Unable to fetch article content";
		contentFetchExhausted = true;
	} catch (err) {
		console.error("Unexpected error:", err);
		if (token === contentFetchToken) {
			// A throw before the in-flight attempt could record itself (e.g. the
			// client throws synchronously) still cost the user one attempt —
			// announcing "0 attempts" would be as untrue as announcing the
			// ceiling.
			contentAttemptCount = Math.max(contentAttemptCount, 1);
			error = "Unexpected error occurred";
			contentFetchExhausted = true;
		}
	} finally {
		if (token === contentFetchToken) {
			isLoading = false;
		}
	}
};

// User-initiated retry after the automatic budget is spent. This is the only
// way back into fetchData once `contentFetchExhausted` is set.
const handleRetryContent = () => {
	void fetchData();
};

const handleShowDetails = () => {
	// Open first, fetch behind the sheet's own loading state. Awaiting the
	// fetch here would hold the sheet closed for the whole bounded retry
	// sequence — up to 500ms of backoff plus two round trips — with nothing on
	// screen but a disabled button.
	open = true;

	// If we already have initial data there is nothing to fetch.
	if (initialData) return;

	// Kick the fetch off synchronously so `isLoading` is already set by the
	// time the auto-fetch effect runs; its guard then makes that a no-op
	// rather than a second request.
	void fetchData();
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
					if (import.meta.env.DEV) {
						console.log("[StreamSummarize] Stream aborted by user");
					}
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
				if (import.meta.env.DEV) {
					console.log("[StreamSummarize] Falling back to legacy endpoint");
				}
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

{#if showButton && !open}
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
	bind:open
	onOpenChange={(next: boolean) => {
		if (!next) handleHideDetails();
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

			{#if contentFetchExhausted}
				<div class="error-stripe content-failure" role="alert">
					<span>{contentFailureMessage}</span>
					<button
						class="action-btn"
						data-testid="retry-content-{uniqueId}"
						onclick={handleRetryContent}
						disabled={isLoading}
					>
						Reload article
					</button>
				</div>
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

	.content-failure {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
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
