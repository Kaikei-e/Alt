<script lang="ts">
import type {
	FeedContentOnTheFlyResponse,
	FetchArticleSummaryResponse,
} from "$lib/api/client";
import {
	getArticleSummaryClient,
	getFeedContentOnTheFlyClient,
} from "$lib/api/client";
import RenderFeedDetails from "$lib/components/mobile/RenderFeedDetails.svelte";
import type { SearchFeedItem } from "$lib/schema/search";
import {
	CONTENT_PENDING_LABEL,
	EMPTY_CONTENT_ERROR,
	TRY_AGAIN_LABEL,
} from "$lib/utils/articleContentState";
import { articleContentErrorMessage } from "$lib/utils/errorClassification";

interface Props {
	result: SearchFeedItem;
}

const { result }: Props = $props();

let isExpanded = $state(false);
let summary = $state<FetchArticleSummaryResponse | null>(null);
let isLoadingSummary = $state(false);
let summaryError = $state<string | null>(null);
let isSummarizing = $state(false);
let isDescriptionExpanded = $state(false);
let isBodyExpanded = $state(false);
let body = $state<FeedContentOnTheFlyResponse | null>(null);
let isLoadingBody = $state(false);
let bodyError = $state<string | null>(null);

const descriptionText = $derived((result.description || "").trim());
const hasDescription = $derived(descriptionText.length > 0);
const shouldTruncateDescription = $derived(descriptionText.length > 200);
const displayDescription = $derived(
	isDescriptionExpanded
		? descriptionText
		: shouldTruncateDescription
			? `${descriptionText.slice(0, 200)}...`
			: descriptionText,
);

// The body is fetched on tap and never on render: a search page holds 20+ of
// these cards, and warming every one of them would put an on-the-fly fetch per
// result onto the publisher's host slots for bodies nobody asked to read.
const fetchBody = async () => {
	if (!result.link) return;
	if (isLoadingBody) return;

	isLoadingBody = true;
	bodyError = null;

	try {
		const response = await getFeedContentOnTheFlyClient(result.link);
		// `content: ""` is the server answering with nothing, not "not fetched
		// yet". Routing it through the shared formatter keeps this card from
		// rendering an empty panel where every other reading surface states the
		// failure.
		if (!response.content.trim()) {
			body = null;
			bodyError = articleContentErrorMessage(EMPTY_CONTENT_ERROR);
			return;
		}
		body = response;
	} catch (error) {
		console.error("Error fetching article content:", error);
		body = null;
		bodyError = articleContentErrorMessage(error);
	} finally {
		isLoadingBody = false;
	}
};

const handleToggleBody = async () => {
	if (isBodyExpanded) {
		isBodyExpanded = false;
		return;
	}
	isBodyExpanded = true;
	// A body already in hand is reused; only a previous failure re-fetches.
	if (!body) await fetchBody();
};

const handleToggleSummary = async () => {
	if (!isExpanded && !summary && result.link) {
		isLoadingSummary = true;
		summaryError = null;

		try {
			const summaryResponse = await getArticleSummaryClient(result.link);
			summary = summaryResponse;
		} catch (error) {
			console.error("Error fetching summary:", error);
			summaryError = "Failed to fetch summary";
		} finally {
			isLoadingSummary = false;
		}
	}
	isExpanded = !isExpanded;
};

const handleSummarizeNow = async () => {
	if (!result.link) return;

	isSummarizing = true;
	try {
		const summaryResponse = await getArticleSummaryClient(result.link);
		summary = summaryResponse;
		isExpanded = true;
	} catch (error) {
		console.error("Error generating summary:", error);
		summaryError = "Failed to fetch summary";
	} finally {
		isSummarizing = false;
	}
};

const authorName = $derived(
	result.author?.name || result.authors?.[0]?.name || "Unknown",
);
const publishedDate = $derived(
	result.published ? new Date(result.published).toLocaleDateString() : null,
);
</script>

<article class="archive-result" data-role="archive-result-item" data-testid="search-result-item">
	<a
		href={result.link || "#"}
		target="_blank"
		rel="noopener noreferrer"
		class="result-title"
	>
		{result.title}
	</a>

	{#if result.published}
		<span class="result-dateline">
			{authorName}{publishedDate ? ` \u00b7 ${publishedDate}` : ""}
		</span>
	{/if}

	{#if hasDescription}
		<p class="result-excerpt">{displayDescription}</p>
		{#if shouldTruncateDescription}
			<button
				type="button"
				onclick={() => { isDescriptionExpanded = !isDescriptionExpanded; }}
				class="result-toggle"
			>
				{isDescriptionExpanded ? "SHOW LESS" : "READ MORE"}
			</button>
		{/if}
	{/if}

	<div class="result-actions">
		{#if result.link}
			<button
				type="button"
				onclick={handleToggleBody}
				class="result-action-btn"
				data-role="toggle-details-btn"
			>
				{isBodyExpanded ? "HIDE DETAILS" : "DETAILS"}
			</button>
		{/if}
		<button
			type="button"
			onclick={handleToggleSummary}
			class="result-action-btn"
			data-role="toggle-summary-btn"
		>
			{isExpanded ? "HIDE SUMMARY" : "SHOW SUMMARY"}
		</button>
	</div>

	{#if isBodyExpanded}
		<div class="result-body" data-role="result-body">
			{#if isLoadingBody}
				<div class="result-summary-loading">
					<span class="loading-pulse"></span>
					<span class="result-loading-text">{CONTENT_PENDING_LABEL}</span>
				</div>
			{:else if bodyError}
				<div class="error-stripe">{bodyError}</div>
				<button type="button" onclick={fetchBody} class="result-action-btn">
					{TRY_AGAIN_LABEL}
				</button>
			{:else if body}
				<RenderFeedDetails feedDetails={body} />
			{/if}
		</div>
	{/if}

	{#if isExpanded}
		<div class="result-summary">
			{#if isLoadingSummary}
				<div class="result-summary-loading">
					<span class="loading-pulse"></span>
					<span class="result-loading-text">Loading summary...</span>
				</div>
			{:else if isSummarizing}
				<div class="result-summary-loading">
					<span class="loading-pulse"></span>
					<span class="result-loading-text">Generating summary...</span>
				</div>
			{:else if summaryError}
				<div class="error-stripe">{summaryError}</div>
				{#if summaryError === "Failed to fetch summary"}
					<button type="button" onclick={handleSummarizeNow} class="result-action-btn">
						SUMMARIZE NOW
					</button>
				{/if}
			{:else if summary?.matched_articles && summary.matched_articles.length > 0}
				<h4 class="result-summary-title">{summary.matched_articles[0]!.title}</h4>
				<p class="result-summary-prose">{summary.matched_articles[0]!.content}</p>
			{:else}
				<p class="result-summary-empty">No summary available</p>
				<button type="button" onclick={handleSummarizeNow} class="result-action-btn">
					SUMMARIZE NOW
				</button>
			{/if}
		</div>
	{/if}
</article>

<style>
	/* Feed descriptions arrive as raw `content:encoded` blobs, so a single
	   CDN image URL (query string included) is routinely one unbreakable
	   token far wider than a phone viewport. min-width: 0 lets the card
	   shrink below that intrinsic width and `overflow-wrap: anywhere` on the
	   text below splits the token at any glyph; without both, the URL runs
	   straight out of the card and gives the whole page a horizontal
	   scrollbar. */
	.archive-result {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
		min-width: 0;
		padding: 0.75rem;
		border: 1px solid var(--surface-border);
		background: var(--surface-bg);
		transition: background 0.15s;
	}

	.result-title {
		font-family: var(--font-display);
		font-size: 0.95rem;
		font-weight: 600;
		color: var(--alt-charcoal);
		text-decoration: none;
		line-height: 1.3;
		overflow-wrap: anywhere;
		word-break: break-word;
	}

	.result-title:hover {
		text-decoration: underline;
		text-underline-offset: 2px;
	}

	.result-dateline {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-ash);
		letter-spacing: 0.04em;
		overflow-wrap: anywhere;
		word-break: break-word;
	}

	.result-excerpt {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-slate);
		line-height: 1.5;
		margin: 0;
		overflow-wrap: anywhere;
		word-break: break-word;
	}

	.result-toggle {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-primary);
		background: transparent;
		border: none;
		cursor: pointer;
		padding: 0;
		letter-spacing: 0.04em;
		text-transform: uppercase;
	}

	.result-actions {
		display: flex;
		gap: 0.5rem;
		margin-top: 0.5rem;
		padding-top: 0.5rem;
		border-top: 1px solid var(--surface-border);
	}

	.result-actions .result-action-btn {
		flex: 1 1 0;
		min-width: 0;
	}

	.result-action-btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 0.4rem;
		width: 100%;
		min-height: 44px;
		padding: 0.5rem 1rem;
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

	.result-action-btn:hover {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	.result-summary {
		margin-top: 0.5rem;
	}

	/* The fetched body is rendered by RenderFeedDetails, which brings its own
	   `.article-content` prose rules. min-width: 0 keeps a wide token inside
	   it — a URL, a <pre> line — from widening the card. */
	.result-body {
		margin-top: 0.5rem;
		min-width: 0;
	}

	.result-summary-loading {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.75rem 0;
	}

	.result-loading-text {
		font-family: var(--font-body);
		font-size: 0.8rem;
		color: var(--alt-ash);
		font-style: italic;
	}

	.result-summary-title {
		font-family: var(--font-display);
		font-size: 0.85rem;
		font-weight: 600;
		color: var(--alt-charcoal);
		margin: 0 0 0.3rem;
		overflow-wrap: anywhere;
		word-break: break-word;
	}

	.result-summary-prose {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-charcoal);
		line-height: 1.7;
		white-space: pre-wrap;
		margin: 0;
		overflow-wrap: anywhere;
		word-break: break-word;
	}

	.result-summary-empty {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-ash);
		font-style: italic;
		margin: 0;
	}

	.error-stripe {
		padding: 0.5rem 0.75rem;
		border-left: 3px solid var(--alt-terracotta);
		font-family: var(--font-body);
		font-size: 0.8rem;
		color: var(--alt-terracotta);
		margin: 0.3rem 0;
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
