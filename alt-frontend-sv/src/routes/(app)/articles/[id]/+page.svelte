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
} from "@lucide/svelte";
import { getFeedContentOnTheFlyClient } from "$lib/api/client/articles";
import RenderFeedDetails from "$lib/components/mobile/RenderFeedDetails.svelte";
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

onDestroy(() => {
	summarizer.abort();
});

async function fetchContent() {
	if (!articleUrl) return;

	isFetching = true;
	contentError = null;

	try {
		const response = await getFeedContentOnTheFlyClient(articleUrl);
		articleContent = response.content || null;
		fetchedArticleId = response.article_id || null;
	} catch (err) {
		contentError =
			err instanceof Error ? err.message : "Failed to fetch article";
	} finally {
		isFetching = false;
	}
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
	<title>Article - Alt</title>
</svelte:head>

<div class="max-w-4xl mx-auto px-4 py-6">
	<!-- Header -->
	<div class="flex items-center gap-4 mb-6">
		<Button variant="ghost" onclick={() => goto("/home")} class="flex items-center gap-2">
			<ArrowLeft class="h-4 w-4" />
			Back to Home
		</Button>
		<div class="ml-auto flex items-center gap-3">
			<!-- Summarize Button -->
			<Button
				onclick={handleSummarize}
				disabled={summarizer.buttonState === 'loading' || (!articleContent && summarizer.buttonState !== 'error' && summarizer.buttonState !== 'success')}
				variant={summarizer.buttonState === 'error' ? 'destructive' : undefined}
				class={summarizer.buttonState === 'error' ? 'flex items-center gap-2' : 'flex items-center gap-2 bg-[#2f4f4f] text-white hover:bg-[#2f4f4f]/90 hover:text-white disabled:opacity-50'}
			>
				{#if summarizer.buttonState === 'loading'}
					<Loader2 class="h-4 w-4 animate-spin" />
					<span>Summarizing...</span>
				{:else if summarizer.buttonState === 'error'}
					<RefreshCw class="h-4 w-4" />
					<span>Try again</span>
				{:else if summarizer.buttonState === 'success'}
					<RefreshCw class="h-4 w-4" />
					<span>Re-summarize</span>
				{:else}
					<Sparkles class="h-4 w-4" />
					<span>Summarize By AI</span>
				{/if}
			</Button>
			{#if articleUrl}
				<a
					href={articleUrl}
					target="_blank"
					rel="noopener noreferrer"
					class="flex items-center gap-2 text-sm text-[var(--interactive-text)] hover:underline"
				>
					Open original
					<ExternalLink class="h-4 w-4" />
				</a>
			{/if}
		</div>
	</div>

	<!-- AI Summary -->
	{#if summarizer.summary}
		<div class="mb-6 p-4 bg-white rounded-lg border border-[var(--surface-border)]">
			<h3 class="text-sm font-semibold text-gray-500 flex items-center gap-2 mb-3">
				<Sparkles class="h-4 w-4" />
				AI SUMMARY
			</h3>
			<div class="text-gray-700 leading-relaxed whitespace-pre-wrap">
				{summarizer.summary}
			</div>
		</div>
	{:else if summarizer.summaryError}
		<div class="mb-6 p-4 bg-white border-2 border-destructive rounded-lg" role="alert">
			<p class="text-red-600 text-sm">{summarizer.summaryError}</p>
		</div>
	{/if}

	<!-- Content -->
	{#if !articleUrl}
		<div class="text-center py-12">
			<p class="text-[var(--text-secondary)]">
				No article URL provided. Unable to load content.
			</p>
			<Button variant="outline" onclick={() => goto("/home")} class="mt-4">
				Return to Home
			</Button>
		</div>
	{:else if isFetching}
		<div class="flex items-center justify-center py-12 gap-3">
			<Loader2 class="h-5 w-5 animate-spin text-[var(--text-secondary)]" />
			<span class="text-[var(--text-secondary)]">Loading article...</span>
		</div>
	{:else if contentError}
		<div class="text-center py-12">
			<p class="text-red-600 mb-4">{contentError}</p>
			<Button variant="outline" onclick={fetchContent}>
				Try again
			</Button>
		</div>
	{:else if articleContent}
		<div class="bg-white rounded-lg border border-[var(--surface-border)] p-6">
			<RenderFeedDetails
				feedDetails={{ content: articleContent, article_id: fetchedArticleId ?? "", og_image_url: "", og_image_proxy_url: "" }}
				error={contentError}
			/>
		</div>
	{:else}
		<div class="text-center py-12">
			<p class="text-[var(--text-secondary)]">No content available.</p>
		</div>
	{/if}
</div>
