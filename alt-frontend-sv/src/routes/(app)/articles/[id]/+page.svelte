<script lang="ts">
import { page } from "$app/state";
import { goto } from "$app/navigation";
import { onDestroy } from "svelte";
import {
	ExternalLink,
	ArrowLeft,
	Loader2,
	Sparkles,
	RefreshCw,
	Download,
} from "@lucide/svelte";
import { getFeedContentOnTheFlyClient } from "$lib/api/client/articles";
import RenderFeedDetails from "$lib/components/mobile/RenderFeedDetails.svelte";
import PageKicker from "$lib/components/recap/job-status/PageKicker.svelte";
import { Button } from "$lib/components/ui/button";
import { useSummarize } from "$lib/hooks/useSummarize.svelte";

const articleId = $derived(page.params.id);
const articleUrl = $derived(page.url.searchParams.get("url"));
const articleTitle = $derived(page.url.searchParams.get("title"));

let isFetching = $state(false);
let articleContent = $state<string | null>(null);
let fetchedArticleId = $state<string | null>(null);
let contentError = $state<string | null>(null);
let previousUrl = $state<string | null>(null);

const summarizer = useSummarize();

const fetchButtonState = $derived.by(() => {
	if (isFetching) return "loading" as const;
	if (contentError) return "error" as const;
	if (articleContent) return "success" as const;
	return "idle" as const;
});

const sourceHost = $derived.by(() => {
	if (!articleUrl) return "";
	try {
		return new URL(articleUrl).host;
	} catch {
		return "";
	}
});

const mastheadKicker = $derived(
	sourceHost ? sourceHost.toUpperCase() : "ARTICLE",
);
const mastheadTitle = $derived(articleTitle ?? sourceHost ?? "Article");

const fetchAriaLabel = $derived.by(() => {
	switch (fetchButtonState) {
		case "loading":
			return "Fetching article";
		case "error":
			return "Retry fetch";
		case "success":
			return "Re-fetch article";
		default:
			return "Fetch article";
	}
});

const summarizeAriaLabel = $derived.by(() => {
	switch (summarizer.buttonState) {
		case "loading":
			return "Summarizing";
		case "error":
			return "Retry summarize";
		case "success":
			return "Re-summarize";
		default:
			return "Summarize with AI";
	}
});

onDestroy(() => {
	summarizer.abort();
});

async function fetchContent(forceRefresh = false) {
	if (!articleUrl) return;

	isFetching = true;
	contentError = null;

	try {
		const response = await getFeedContentOnTheFlyClient(articleUrl, {
			forceRefresh,
		});
		articleContent = response.content || null;
		fetchedArticleId = response.article_id || null;
	} catch (err) {
		contentError =
			err instanceof Error ? err.message : "Failed to fetch article";
	} finally {
		isFetching = false;
	}
}

function handleFetch() {
	const isRefetch = fetchButtonState === "success";
	if (isRefetch) {
		summarizer.reset();
	}
	fetchContent(isRefetch);
}

function handleSummarize() {
	if (!articleUrl) return;
	summarizer.summarize(
		articleUrl,
		fetchedArticleId ?? articleId,
		articleTitle ?? undefined,
		summarizer.buttonState === "success",
	);
}

// Reset state when articleUrl changes
$effect(() => {
	if (articleUrl !== previousUrl) {
		previousUrl = articleUrl;
		articleContent = null;
		fetchedArticleId = null;
		contentError = null;
		isFetching = false;
		summarizer.reset();
	}
});

// Fetch content only when idle and no result/error yet
$effect(() => {
	if (articleUrl && !articleContent && !isFetching && !contentError) {
		fetchContent();
	}
});

// Auto-trigger summarize when ?summarize=true and content is loaded
$effect(() => {
	const autoSummarize = page.url.searchParams.get("summarize") === "true";
	if (
		autoSummarize &&
		articleContent &&
		!summarizer.isSummarizing &&
		!summarizer.summary
	) {
		handleSummarize();
	}
});
</script>

<svelte:head>
	<title>Article — Alt</title>
</svelte:head>

<div class="article-page">
	<header class="action-bar" aria-label="Article actions">
		<Button
			variant="ghost"
			size="icon"
			aria-label="Back to Home"
			onclick={() => goto("/home")}
			class="action-btn action-btn--back"
		>
			<ArrowLeft class="h-4 w-4" />
			<span class="action-label">Back</span>
		</Button>

		<div class="actions-right">
			<Button
				data-testid="fetch-button"
				aria-label={fetchAriaLabel}
				onclick={handleFetch}
				disabled={fetchButtonState === 'loading'}
				variant={fetchButtonState === 'error' ? 'destructive' : 'outline'}
				size="icon"
				class="action-btn"
			>
				{#if fetchButtonState === 'loading'}
					<Loader2 class="h-4 w-4 animate-spin" />
					<span class="action-label">Fetching...</span>
				{:else if fetchButtonState === 'error'}
					<RefreshCw class="h-4 w-4" />
					<span class="action-label">Try again</span>
				{:else if fetchButtonState === 'success'}
					<RefreshCw class="h-4 w-4" />
					<span class="action-label">Re-fetch</span>
				{:else}
					<Download class="h-4 w-4" />
					<span class="action-label">Fetch</span>
				{/if}
			</Button>

			<Button
				data-testid="summarize-button"
				aria-label={summarizeAriaLabel}
				onclick={handleSummarize}
				disabled={summarizer.buttonState === 'loading' || (!articleContent && summarizer.buttonState !== 'error' && summarizer.buttonState !== 'success')}
				variant={summarizer.buttonState === 'error' ? 'destructive' : 'default'}
				size="icon"
				class={summarizer.buttonState === 'error' ? 'action-btn' : 'action-btn action-btn--primary'}
			>
				{#if summarizer.buttonState === 'loading'}
					<Loader2 class="h-4 w-4 animate-spin" />
					<span class="action-label">Summarizing...</span>
				{:else if summarizer.buttonState === 'error'}
					<RefreshCw class="h-4 w-4" />
					<span class="action-label">Try again</span>
				{:else if summarizer.buttonState === 'success'}
					<RefreshCw class="h-4 w-4" />
					<span class="action-label">Re-summarize</span>
				{:else}
					<Sparkles class="h-4 w-4" />
					<span class="action-label">Summarize</span>
				{/if}
			</Button>

			{#if articleUrl}
				<a
					href={articleUrl}
					target="_blank"
					rel="noopener noreferrer"
					aria-label="Open original"
					class="action-link"
				>
					<ExternalLink class="h-4 w-4" />
					<span class="action-label">Open original</span>
				</a>
			{/if}
		</div>
	</header>

	<article class="article-body">
		<PageKicker
			testId="article-masthead"
			kicker={mastheadKicker}
			title={mastheadTitle}
		/>

		{#if summarizer.summary}
			<section class="ai-summary" data-testid="ai-summary" aria-label="AI summary">
				<p class="ai-summary__kicker">
					<Sparkles class="h-[0.75rem] w-[0.75rem]" />
					<span>AI SUMMARY</span>
				</p>
				<div class="ai-summary__body">{summarizer.summary}</div>
			</section>
		{:else if summarizer.summaryError}
			<section class="ai-summary ai-summary--error" role="alert">
				<p class="ai-summary__kicker">SUMMARIZE ERROR</p>
				<div class="ai-summary__body">{summarizer.summaryError}</div>
			</section>
		{/if}

		{#if !articleUrl}
			<div class="placeholder">
				<p>No article URL provided. Unable to load content.</p>
				<Button variant="outline" onclick={() => goto("/home")} class="placeholder-cta mt-1">
					Return to Home
				</Button>
			</div>
		{:else if isFetching}
			<div class="placeholder placeholder--center">
				<Loader2 class="h-5 w-5 animate-spin" />
				<span>Loading article...</span>
			</div>
		{:else if contentError}
			<div class="placeholder">
				<p class="placeholder__error">{contentError}</p>
				<Button variant="outline" onclick={() => fetchContent()} class="placeholder-cta mt-1">
					Try again
				</Button>
			</div>
		{:else if articleContent}
			<div class="article-surface" data-testid="article-content-surface">
				<RenderFeedDetails
					feedDetails={{
						content: articleContent,
						article_id: fetchedArticleId ?? "",
						og_image_url: "",
						og_image_proxy_url: "",
					}}
					error={contentError}
				/>
			</div>
		{:else}
			<div class="placeholder">
				<p>No content available.</p>
			</div>
		{/if}
	</article>
</div>

<style>
.article-page {
	max-width: 720px;
	margin: 0 auto;
	padding: 0 1rem 2rem;
	color: var(--alt-charcoal, #1a1a1a);
	font-family: var(--font-body);
}

.action-bar {
	position: sticky;
	top: 0;
	z-index: 20;
	display: flex;
	align-items: center;
	gap: 0.5rem;
	min-height: 56px;
	padding: calc(env(safe-area-inset-top, 0px) + 0.5rem) 0 0.5rem;
	background: var(--surface-bg, #faf9f7);
	border-bottom: 1px solid var(--surface-border, #c8c8c8);
}

.actions-right {
	display: flex;
	align-items: center;
	gap: 0.25rem;
	margin-left: auto;
	min-width: 0;
}

.action-label {
	display: none;
}

.action-link {
	display: inline-flex;
	align-items: center;
	justify-content: center;
	width: 2.25rem;
	height: 2.25rem;
	border: 1px solid var(--surface-border, #c8c8c8);
	color: var(--interactive-text, #2f4f4f);
	background: transparent;
	text-decoration: none;
	transition: background 0.15s ease;
}

.action-link:hover,
.action-link:focus-visible {
	background: var(--surface-hover, #f3f1ed);
	outline: none;
}

.article-body {
	display: flex;
	flex-direction: column;
	gap: 1.25rem;
	padding-top: 1.25rem;
	min-width: 0;
}

.ai-summary {
	display: flex;
	flex-direction: column;
	gap: 0.75rem;
	padding: 1rem 1.1rem 1.1rem;
	background: var(--surface-2, #f5f4f1);
	border: 1px solid var(--surface-border, #c8c8c8);
	border-left: 3px solid var(--alt-primary, #2f4f4f);
}

.ai-summary--error {
	border-left-color: var(--alt-error, #8c1d1d);
}

.ai-summary__kicker {
	display: inline-flex;
	align-items: center;
	gap: 0.4rem;
	margin: 0;
	font-family: var(--font-mono);
	font-size: 0.65rem;
	font-weight: 600;
	letter-spacing: 0.14em;
	text-transform: uppercase;
	color: var(--alt-ash, #999999);
}

.ai-summary__body {
	font-family: var(--font-body);
	font-size: 1rem;
	line-height: 1.75;
	color: var(--alt-charcoal, #1a1a1a);
	white-space: pre-wrap;
	word-break: break-word;
	overflow-wrap: anywhere;
}

.article-surface {
	border-top: 1px solid var(--surface-border, #c8c8c8);
	padding-top: 0.5rem;
	min-width: 0;
}

.placeholder {
	display: flex;
	flex-direction: column;
	align-items: center;
	justify-content: center;
	gap: 0.75rem;
	padding: 3rem 1rem;
	color: var(--alt-slate, #666666);
	text-align: center;
}

.placeholder--center {
	flex-direction: row;
}

.placeholder__error {
	color: var(--alt-error, #8c1d1d);
	margin: 0;
}

/* Tablet / Desktop — restore label text on wider viewports */
@media (min-width: 640px) {
	.article-page {
		padding: 0 1.5rem 3rem;
	}

	.action-label {
		display: inline;
	}

	.action-link {
		width: auto;
		height: auto;
		gap: 0.4rem;
		padding: 0.4rem 0.75rem;
		font-family: var(--font-body);
		font-size: 0.85rem;
	}
}

@media (min-width: 768px) {
	.article-page {
		max-width: 820px;
		padding: 0 2rem 3rem;
	}
}
</style>
