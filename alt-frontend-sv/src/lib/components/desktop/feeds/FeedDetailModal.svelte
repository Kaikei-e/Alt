<script lang="ts">
import {
	Check,
	ChevronLeft,
	ChevronRight,
	ExternalLink,
	FileText,
	Loader2,
	Sparkles,
} from "@lucide/svelte";
import { getFeedContentOnTheFlyClient } from "$lib/api/client/articles";
import { updateFeedReadStatusClient } from "$lib/api/client/feeds";
import RenderFeedDetails from "$lib/components/mobile/RenderFeedDetails.svelte";
import { Button } from "$lib/components/ui/button";
import * as Dialog from "$lib/components/ui/dialog";
import {
	createClientTransport,
	streamSummarizeWithAbortAdapter,
} from "$lib/connect";
import type { RenderFeed } from "$lib/schema/feed";
import { articlePrefetcher } from "$lib/utils/articlePrefetcher";

interface Props {
	open: boolean;
	feed: RenderFeed | null;
	onOpenChange: (open: boolean) => void;
	onMarkAsRead?: (feedUrl: string) => void;
	hasPrevious?: boolean;
	hasNext?: boolean;
	onPrevious?: () => void;
	onNext?: () => void;
	feeds?: RenderFeed[];
	currentIndex?: number;
	/** Disable mark as read button (e.g., while parent is processing) */
	disableMarkAsRead?: boolean;
}

let {
	open = $bindable(),
	feed,
	onOpenChange,
	onMarkAsRead,
	hasPrevious = false,
	hasNext = false,
	onPrevious,
	onNext,
	feeds,
	currentIndex,
	disableMarkAsRead = false,
}: Props = $props();

// Mark as read state
let isMarkingAsRead = $state(false);

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

// Track previous feed URL to detect actual feed changes
let previousFeedUrl = $state<string | null>(null);

// Cleanup on modal close
$effect(() => {
	if (!open) {
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
		previousFeedUrl = null;
	}
});

// Reset content states when feed changes (for arrow navigation)
$effect(() => {
	const currentFeedUrl = feed?.normalizedUrl ?? null;

	// Only reset when feed actually changes
	if (currentFeedUrl === previousFeedUrl) return;

	previousFeedUrl = currentFeedUrl;

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
});

// Auto-fetch article content when modal opens
$effect(() => {
	if (
		open &&
		feed?.normalizedUrl &&
		!articleContent &&
		!isFetchingContent &&
		!contentError
	) {
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

async function handleMarkAsRead() {
	if (!feed || isMarkingAsRead) return;

	try {
		isMarkingAsRead = true;
		await updateFeedReadStatusClient(feed.normalizedUrl);
		onMarkAsRead?.(feed.normalizedUrl);
		// Parent handles navigation/closing after removing the feed
	} catch (error) {
		console.error("Failed to mark feed as read:", error);
	} finally {
		isMarkingAsRead = false;
	}
}

async function handleFetchFullArticle() {
	if (!feed?.normalizedUrl || isFetchingContent) return;

	// Check prefetch cache first (using normalizedUrl for consistency)
	const cachedContent = articlePrefetcher.getCachedContent(feed.normalizedUrl);
	const cachedArticleId = articlePrefetcher.getCachedArticleId(
		feed.normalizedUrl,
	);

	if (cachedContent) {
		articleContent = cachedContent;
		articleID = cachedArticleId;
		return;
	}

	try {
		isFetchingContent = true;
		contentError = null;

		// Use normalizedUrl for API call (consistent with prefetcher)
		const response = await getFeedContentOnTheFlyClient(feed.normalizedUrl);

		articleContent = response.content || null;
		articleID = response.article_id || null;
	} catch (err) {
		contentError =
			err instanceof Error ? err.message : "Failed to fetch article";
	} finally {
		isFetchingContent = false;
	}
}

async function handleSummarize() {
	if (!feed?.link || isSummarizing) return;

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
			},
			(chunk: string) => {
				summary = (summary || "") + chunk;
			},
			{}, // No typewriter effect for desktop
			(result) => {
				// onComplete
				isSummarizing = false;
				abortController = null;
			},
			(error) => {
				// onError
				if (error.name !== "AbortError") {
					summaryError = error.message || "Failed to generate summary";
				}
				isSummarizing = false;
				abortController = null;
			},
		);
	} catch (err) {
		if (err instanceof Error && err.name === "AbortError") {
			// User cancelled, ignore
			return;
		}
		summaryError =
			err instanceof Error ? err.message : "Failed to generate summary";
		isSummarizing = false;
		abortController = null;
	}
}
</script>

<Dialog.Root {open} onOpenChange={onOpenChange}>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 bg-black/50 z-50" />
		<Dialog.Content
			class="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 w-[75vw] sm:max-w-[1800px] h-[75vh] bg-white rounded-lg shadow-xl overflow-hidden flex flex-col z-50"
		>
			<!-- Navigation Arrows (inside modal at edges) -->
			{#if hasPrevious}
				<button
					onclick={onPrevious}
					class="absolute left-3 top-1/2 -translate-y-1/2 p-2 rounded-full bg-gray-100 hover:bg-gray-200 transition-colors z-10 shadow"
					aria-label="Previous feed"
				>
					<ChevronLeft class="h-6 w-6 text-gray-700" />
				</button>
			{/if}
			{#if hasNext}
				<button
					onclick={onNext}
					class="absolute right-3 top-1/2 -translate-y-1/2 p-2 rounded-full bg-gray-100 hover:bg-gray-200 transition-colors z-10 shadow"
					aria-label="Next feed"
				>
					<ChevronRight class="h-6 w-6 text-gray-700" />
				</button>
			{/if}

			{#if feed}
				<!-- Header Section -->
				<div class="py-6 border-b border-gray-200" style="padding-left: 70px; padding-right: 70px;">
					<!-- Title with external link -->
					<a
						href={feed.link}
						target="_blank"
						rel="noopener noreferrer"
						class="group flex items-start gap-2 hover:underline"
					>
						<h2 class="text-2xl font-bold text-[#1a1a1a] flex-1">
							{feed.title || "Untitled"}
						</h2>
						<ExternalLink class="h-5 w-5 text-gray-400 group-hover:text-blue-600 flex-shrink-0" />
					</a>

					<!-- Metadata -->
					<div class="flex items-center gap-4 mt-2 text-sm text-gray-600">
						{#if feed.author}
							<span>{feed.author}</span>
						{/if}
						{#if feed.publishedAtFormatted}
							{#if feed.author}
								<span>•</span>
							{/if}
							<span>{feed.publishedAtFormatted}</span>
						{/if}
					</div>

					<!-- Tags -->
					{#if feed.mergedTagsLabel}
						<div class="flex gap-2 mt-3 flex-wrap">
							{#each feed.mergedTagsLabel.split(" / ") as tag}
								<span class="px-2 py-1 bg-gray-100 text-gray-700 text-xs rounded">
									{tag}
								</span>
							{/each}
						</div>
					{/if}
				</div>

				<!-- Scrollable Content Section -->
				<div class="flex-1 overflow-y-auto py-6 bg-[#f8f8f8]" style="padding-left: 70px; padding-right: 70px;">
					<!-- Excerpt (always visible) -->
					{#if feed.excerpt}
						<div class="mb-6 p-4 bg-white rounded border border-gray-200">
							<h3 class="text-sm font-semibold text-gray-500 mb-2">EXCERPT</h3>
							<p class="text-gray-700 leading-relaxed whitespace-pre-wrap">{feed.excerpt}</p>
						</div>
					{/if}

					<!-- Full Article Section -->
					{#if articleContent}
						<div class="mb-6 p-4 bg-white rounded border border-gray-200">
							<h3 class="text-sm font-semibold text-gray-500 mb-3">FULL ARTICLE</h3>
							<RenderFeedDetails
								feedDetails={articleContent ? { content: articleContent, article_id: articleID ?? "" } : null}
								error={contentError}
							/>
						</div>
					{:else if contentError}
						<div class="mb-6 p-4 bg-red-50 border border-red-200 rounded">
							<p class="text-red-600 text-sm">{contentError}</p>
						</div>
					{/if}

					<!-- AI Summary Section -->
					{#if summary}
						<div class="mb-6 p-4 bg-white rounded border border-gray-200">
							<h3 class="text-sm font-semibold text-gray-500 mb-3 flex items-center gap-2">
								<Sparkles class="h-4 w-4" />
								AI SUMMARY
							</h3>
							<div class="text-gray-700 leading-relaxed whitespace-pre-wrap">
								{summary}
							</div>
						</div>
					{:else if summaryError}
						<div class="mb-6 p-4 bg-red-50 border border-red-200 rounded">
							<p class="text-red-600 text-sm">{summaryError}</p>
						</div>
					{/if}
				</div>

				<!-- Footer Actions -->
				<div class="py-4 border-t border-gray-200 bg-gray-50 flex flex-wrap gap-3 items-center" style="padding-left: 70px; padding-right: 70px;">
					<!-- 左側グループ: アクションボタン -->
					<div class="flex gap-3 flex-1 min-w-0">
						<!-- Full Article Button -->
						<Button
							onclick={handleFetchFullArticle}
							disabled={isFetchingContent || !!articleContent}
							class="flex items-center gap-2"
							variant="outline"
						>
							{#if isFetchingContent}
								<Loader2 class="h-4 w-4 animate-spin" />
								<span>Loading...</span>
							{:else if articleContent}
								<Check class="h-4 w-4" />
								<span>Article Loaded</span>
							{:else}
								<FileText class="h-4 w-4" />
								<span>Full Article</span>
							{/if}
						</Button>

						<!-- Summarize Button -->
						<Button
							onclick={handleSummarize}
							disabled={isSummarizing || !articleContent}
							class="flex items-center gap-2 bg-[#2f4f4f] text-white hover:opacity-90 disabled:opacity-50"
						>
							{#if isSummarizing}
								<Loader2 class="h-4 w-4 animate-spin" />
								<span>Summarizing...</span>
							{:else}
								<Sparkles class="h-4 w-4" />
								<span>Summarize By AI</span>
							{/if}
						</Button>
					</div>

					<!-- 右側グループ: 状態変更とクローズ -->
					<div class="flex gap-3 flex-shrink-0">
						<!-- Mark as Read -->
						<Button
							onclick={handleMarkAsRead}
							variant="outline"
							disabled={isMarkingAsRead || disableMarkAsRead}
						>
							{isMarkingAsRead ? "Marking..." : "Mark as Read"}
						</Button>

						<!-- Close -->
						<Dialog.Close class="inline-flex items-center justify-center gap-2 rounded-none text-base font-bold px-4 py-2 h-9 bg-transparent text-[var(--text-primary)] border-2 border-transparent hover:bg-[var(--surface-hover)] hover:border-[var(--surface-border)] transition-all focus-visible:outline-none disabled:opacity-60">
							Close
						</Dialog.Close>
					</div>
				</div>
			{/if}
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
