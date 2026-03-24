<script lang="ts">
import { onDestroy } from "svelte";
import type { TagTrailArticle } from "$lib/connect";
import { getFeedContentOnTheFlyClient } from "$lib/api/client/articles";
import RenderFeedDetails from "$lib/components/mobile/RenderFeedDetails.svelte";
import { useSummarize } from "$lib/hooks/useSummarize.svelte";
import { useTtsPlayback } from "$lib/hooks/useTtsPlayback.svelte";
import {
	X,
	ExternalLink,
	FileText,
	Sparkles,
	Volume2,
	Square,
	Loader2,
	RefreshCw,
} from "@lucide/svelte";

interface Props {
	article: TagTrailArticle;
	onClose: () => void;
}

const { article, onClose }: Props = $props();

let isFetchingContent = $state(false);
let articleContent = $state<string | null>(null);
let fetchedArticleId = $state<string | null>(null);
let contentError = $state<string | null>(null);

const summarizer = useSummarize();
const tts = useTtsPlayback();

const fetchButtonState = $derived.by(() => {
	if (isFetchingContent) return "loading" as const;
	if (contentError) return "error" as const;
	if (articleContent) return "success" as const;
	return "idle" as const;
});

const feedDetailsResponse = $derived(
	articleContent ? { content: articleContent, article_id: fetchedArticleId } : null,
);

onDestroy(() => {
	summarizer.abort();
	tts.stop();
});

let prevArticleId = $state("");
$effect(() => {
	if (article.id !== prevArticleId) {
		prevArticleId = article.id;
		articleContent = null;
		fetchedArticleId = null;
		contentError = null;
		summarizer.reset();
		tts.stop();
		fetchContent();
	}
});

async function fetchContent(forceRefresh = false) {
	if (!article.link) return;
	isFetchingContent = true;
	contentError = null;
	try {
		const response = await getFeedContentOnTheFlyClient(article.link, { forceRefresh });
		articleContent = response.content || null;
		fetchedArticleId = response.article_id || null;
	} catch (err) {
		contentError = err instanceof Error ? err.message : "Failed to fetch article";
	} finally {
		isFetchingContent = false;
	}
}

function handleFetch() {
	const isRefetch = fetchButtonState === "success";
	if (isRefetch) summarizer.reset();
	fetchContent(isRefetch);
}

function handleSummarize() {
	if (!article.link) return;
	summarizer.summarize(
		article.link,
		fetchedArticleId ?? article.id,
		article.title,
		summarizer.buttonState === "success",
	);
}

function handleTts() {
	if (tts.isPlaying || tts.isLoading) {
		tts.stop();
	} else if (summarizer.summary) {
		tts.play(summarizer.summary, { speed: 1.25 });
	}
}

function handleKeydown(e: KeyboardEvent) {
	if (e.key === "Escape") {
		onClose();
	}
}
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- Backdrop -->
<div
	class="fixed inset-0 z-30 bg-black/20 transition-opacity duration-300"
	data-testid="detail-backdrop"
	onclick={onClose}
	onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') onClose(); }}
	role="button"
	tabindex="-1"
	aria-label="Close panel"
></div>

<!-- Slide-over Panel -->
<div
	class="fixed right-0 top-0 z-40 h-screen w-[60%] flex flex-col
		border-l shadow-[-8px_0_24px_rgba(0,0,0,0.12)]
		transition-transform duration-300 ease-out"
	style="background: var(--surface-bg); border-color: var(--surface-border);"
	data-testid="article-detail-panel"
	role="dialog"
	aria-modal="true"
	aria-label="Article detail"
>
	<!-- Header -->
	<div class="flex items-start justify-between gap-3 px-6 py-5 border-b" style="border-color: var(--surface-border);">
		<div class="flex-1 min-w-0">
			<h2 class="text-lg font-bold leading-snug line-clamp-3" style="color: var(--text-primary);">
				{article.title}
			</h2>
			<div class="flex items-center gap-2 mt-2">
				{#if article.feedTitle}
					<span class="text-sm" style="color: var(--text-secondary);">{article.feedTitle}</span>
				{/if}
			</div>
		</div>
		<div class="flex items-center gap-1 flex-shrink-0">
			{#if article.link}
				<a
					href={article.link}
					target="_blank"
					rel="noopener noreferrer"
					class="p-2 rounded-md transition-colors"
					style="color: var(--text-muted);"
					title="Open in new tab"
				>
					<ExternalLink class="h-4 w-4" />
				</a>
			{/if}
			<button
				type="button"
				class="p-2 rounded-md transition-colors"
				style="color: var(--text-muted);"
				onclick={onClose}
				aria-label="Close"
			>
				<X class="h-4 w-4" />
			</button>
		</div>
	</div>

	<!-- Action Buttons -->
	<div class="flex items-center gap-2 px-6 py-3 border-b" style="border-color: var(--surface-border);">
		<button
			type="button"
			class="inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs font-medium transition-colors disabled:opacity-50"
			style="border-color: var(--surface-border); color: var(--text-primary); background: var(--action-surface);"
			onclick={handleFetch}
			disabled={isFetchingContent}
		>
			{#if isFetchingContent}
				<Loader2 class="h-3.5 w-3.5 animate-spin" />
			{:else if fetchButtonState === "success"}
				<RefreshCw class="h-3.5 w-3.5" />
			{:else}
				<FileText class="h-3.5 w-3.5" />
			{/if}
			{fetchButtonState === "success" ? "Refetch" : isFetchingContent ? "Loading..." : "Fetch Content"}
		</button>

		<button
			type="button"
			class="inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs font-medium transition-colors disabled:opacity-50"
			style="border-color: var(--surface-border); color: var(--text-primary); background: var(--action-surface);"
			onclick={handleSummarize}
			disabled={!articleContent || summarizer.buttonState === "loading"}
		>
			{#if summarizer.buttonState === "loading"}
				<Loader2 class="h-3.5 w-3.5 animate-spin" />
			{:else}
				<Sparkles class="h-3.5 w-3.5" />
			{/if}
			Summarize
		</button>

		{#if summarizer.summary}
			<button
				type="button"
				class="inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs font-medium transition-colors"
				style="border-color: var(--surface-border); color: var(--text-primary); background: var(--action-surface);"
				onclick={handleTts}
			>
				{#if tts.isPlaying || tts.isLoading}
					<Square class="h-3.5 w-3.5" />
					Stop
				{:else}
					<Volume2 class="h-3.5 w-3.5" />
					Listen
				{/if}
			</button>
		{/if}
	</div>

	<!-- Content Area -->
	<div class="flex-1 overflow-y-auto px-6 py-5">
		{#if summarizer.summary}
			<div class="mb-5 p-4 rounded-lg border" style="background: var(--badge-teal-bg); border-color: var(--badge-teal-border);">
				<p class="text-xs font-semibold mb-2" style="color: var(--badge-teal-text);">AI Summary</p>
				<p class="text-sm leading-relaxed" style="color: var(--text-primary);">{summarizer.summary}</p>
			</div>
		{/if}

		{#if contentError}
			<div class="text-center py-8">
				<p class="text-sm" style="color: var(--text-secondary);">{contentError}</p>
			</div>
		{:else if articleContent}
			<RenderFeedDetails feedDetails={feedDetailsResponse} />
		{:else if !isFetchingContent}
			<div class="text-center py-12">
				<FileText class="h-10 w-10 mx-auto mb-4" style="color: var(--text-muted);" />
				<p class="text-sm font-medium" style="color: var(--text-secondary);">
					Click "Fetch Content" to load the article.
				</p>
			</div>
		{:else}
			<div class="flex justify-center py-12">
				<Loader2 class="h-6 w-6 animate-spin" style="color: var(--interactive-text);" />
			</div>
		{/if}
	</div>
</div>
