<script lang="ts">
import { onDestroy } from "svelte";
import type { TagTrailArticle } from "$lib/connect";
import {
	getFeedContentOnTheFlyClient,
	type FeedContentOnTheFlyResponse,
} from "$lib/api/client/articles";
import RenderFeedDetails from "$lib/components/mobile/RenderFeedDetails.svelte";
import { useSummarize } from "$lib/hooks/useSummarize.svelte";
import {
	type ArticleContentPhase,
	CONTENT_PENDING_LABEL,
	CONTENT_RETRYING_LABEL,
	EMPTY_CONTENT_ERROR,
	foregroundRetryDelayMs,
	READ_ORIGINAL_LABEL,
	TRY_AGAIN_LABEL,
} from "$lib/utils/articleContentState";
import { articleContentErrorMessage } from "$lib/utils/errorClassification";
import {
	X,
	ExternalLink,
	FileText,
	Sparkles,
	Loader2,
	RefreshCw,
} from "@lucide/svelte";

interface Props {
	article: TagTrailArticle;
	onClose: () => void;
}

const { article, onClose }: Props = $props();

// Three honest states rather than a boolean pair. `pending` is not `failed`:
// a body still in flight used to share its markup with a body that will never
// arrive, and an empty response produced neither — just a blank panel.
let contentPhase = $state<ArticleContentPhase>("idle");
let fetchedResponse = $state<FeedContentOnTheFlyResponse | null>(null);
let contentError = $state<string | null>(null);
let fetchAbortController: AbortController | null = null;
// The one automatic re-attempt allowed per failure.
let foregroundRetrySpent = false;
let contentRetryTimer: ReturnType<typeof setTimeout> | null = null;

const summarizer = useSummarize();

const articleContent = $derived(fetchedResponse?.content ?? null);
const fetchedArticleId = $derived(fetchedResponse?.article_id ?? null);
const isFetchingContent = $derived(
	contentPhase === "pending" || contentPhase === "retrying",
);
const pendingLabel = $derived(
	contentPhase === "retrying" ? CONTENT_RETRYING_LABEL : CONTENT_PENDING_LABEL,
);

const fetchButtonState = $derived.by(() => {
	if (isFetchingContent) return "loading" as const;
	if (contentPhase === "failed") return "error" as const;
	if (fetchedResponse) return "success" as const;
	return "idle" as const;
});

onDestroy(() => {
	summarizer.abort();
	fetchAbortController?.abort();
	if (contentRetryTimer) clearTimeout(contentRetryTimer);
});

let prevArticleId = $state("");
$effect(() => {
	if (article.id !== prevArticleId) {
		prevArticleId = article.id;
		fetchedResponse = null;
		contentError = null;
		contentPhase = "idle";
		foregroundRetrySpent = false;
		if (contentRetryTimer) clearTimeout(contentRetryTimer);
		summarizer.reset();
		void fetchContent();
	}
});

/**
 * Record a terminal failure. The wording comes from articleContentErrorMessage
 * and nowhere else — ADR-000959 §6 keeps the upstream `message` off the
 * reading surface.
 */
function failContent(err: unknown, requestedId: string) {
	if (article.id !== requestedId) return;
	contentError = articleContentErrorMessage(err);
	contentPhase = "failed";
}

function applyResponse(
	response: FeedContentOnTheFlyResponse,
	requestedId: string,
) {
	if (article.id !== requestedId) return;
	if (!response.content?.trim()) {
		// `content: ""` is a state, not a falsy no-op (ADR-000581). Storing the
		// empty response as a success left the panel with nothing to render and
		// nothing to say about why.
		failContent(EMPTY_CONTENT_ERROR, requestedId);
		return;
	}
	fetchedResponse = response;
	contentError = null;
	contentPhase = "ready";
}

/** One request. Kept separate so the re-attempt reuses it verbatim. */
async function requestBody(
	forceRefresh: boolean,
): Promise<FeedContentOnTheFlyResponse> {
	fetchAbortController?.abort();
	const controller = new AbortController();
	fetchAbortController = controller;
	return await getFeedContentOnTheFlyClient(article.link, {
		forceRefresh,
		signal: controller.signal,
	});
}

/** Fetch the body, with at most ONE automatic re-attempt. */
async function fetchContent(forceRefresh = false) {
	if (!article.link) return;
	const requestedId = article.id;
	contentPhase = "pending";
	contentError = null;

	try {
		applyResponse(await requestBody(forceRefresh), requestedId);
		return;
	} catch (err) {
		if (article.id !== requestedId) return;
		if (fetchAbortController?.signal.aborted) return;

		const delayMs = foregroundRetrySpent ? null : foregroundRetryDelayMs(err);
		if (delayMs === null) {
			failContent(err, requestedId);
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
		if (article.id !== requestedId) return;

		contentPhase = "pending";
		try {
			applyResponse(await requestBody(forceRefresh), requestedId);
		} catch (retryErr) {
			failContent(retryErr, requestedId);
		}
	}
}

function handleFetch() {
	const isRefetch = fetchButtonState === "success";
	if (isRefetch) summarizer.reset();
	// A reader asking again is a fresh budget.
	foregroundRetrySpent = false;
	void fetchContent(isRefetch);
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
	class="fixed right-0 top-0 z-40 h-[100dvh] w-[60%] flex flex-col
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
	</div>

	<!-- Content Area -->
	<div class="flex-1 overflow-y-auto px-6 py-5">
		{#if summarizer.summary}
			<div class="mb-5 p-4 rounded-lg border" data-testid="ai-summary" style="background: var(--badge-teal-bg); border-color: var(--badge-teal-border);">
				<p class="text-xs font-semibold mb-2" style="color: var(--badge-teal-text);">AI Summary</p>
				<p class="text-sm leading-relaxed" style="color: var(--text-primary);">{summarizer.summary}</p>
				{#if summarizer.summaryError}
					<!-- A cut stream leaves its partial text on screen. Without this
					     notice a truncated summary reads as a clean short one. -->
					<p
						class="mt-3 pl-3 border-l-[3px] text-xs"
						data-testid="summary-interrupted"
						role="alert"
						style="border-color: var(--alt-error); color: var(--alt-error);"
					>
						Stream interrupted. Summary may be incomplete.
					</p>
				{/if}
			</div>
		{:else if summarizer.summaryError}
			<div
				class="mb-5 p-4 rounded-lg border"
				data-testid="summary-error"
				role="alert"
				style="border-color: var(--alt-error);"
			>
				<p class="text-xs font-semibold mb-2" style="color: var(--alt-error);">Summarize Error</p>
				<p class="text-sm leading-relaxed" style="color: var(--text-primary);">{summarizer.summaryError}</p>
			</div>
		{/if}

		{#if isFetchingContent}
			<!-- A request in flight is not a failure, and it says which attempt
			     it is on rather than showing a bare spinner. -->
			<div
				class="flex flex-col items-center gap-3 py-12"
				data-testid="article-content-pending"
			>
				<Loader2 class="h-6 w-6 animate-spin" style="color: var(--interactive-text);" />
				<p class="text-sm" style="color: var(--text-secondary);">{pendingLabel}</p>
			</div>
		{:else if contentPhase === "failed"}
			<div class="text-center py-8" data-testid="article-content-failed" role="alert">
				<p class="text-sm" style="color: var(--text-secondary);">{contentError}</p>
				<!-- Stating the problem without offering a way out is the defect
				     (NN/g heuristic 9). A tag-trail article carries no RSS
				     description, so the remedies are what the reader gets. -->
				<div class="mt-4 flex items-center justify-center gap-4">
					<button
						type="button"
						class="inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs font-medium transition-colors"
						style="border-color: var(--surface-border); color: var(--text-primary); background: var(--action-surface);"
						onclick={handleFetch}
						data-testid="retry-content"
					>
						<RefreshCw class="h-3.5 w-3.5" />
						{TRY_AGAIN_LABEL}
					</button>
					{#if article.link}
						<a
							href={article.link}
							target="_blank"
							rel="noopener noreferrer"
							class="text-xs font-medium underline underline-offset-2"
							style="color: var(--interactive-text);"
							data-testid="read-original-link"
						>
							{READ_ORIGINAL_LABEL}
						</a>
					{/if}
				</div>
			</div>
		{:else if fetchedResponse}
			<RenderFeedDetails feedDetails={fetchedResponse} />
		{:else}
			<div class="text-center py-12">
				<FileText class="h-10 w-10 mx-auto mb-4" style="color: var(--text-muted);" />
				<p class="text-sm font-medium" style="color: var(--text-secondary);">
					Click "Fetch Content" to load the article.
				</p>
			</div>
		{/if}
	</div>
</div>
