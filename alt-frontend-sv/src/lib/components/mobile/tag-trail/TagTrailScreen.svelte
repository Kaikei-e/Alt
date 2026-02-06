<script lang="ts">
import { ArrowLeft, Loader2, Shuffle } from "@lucide/svelte";
import {
	createClientTransport,
	fetchArticleContent,
	fetchArticlesByTag,
	fetchRandomFeed,
	streamArticleTags,
	type RandomFeed,
	type TagTrailArticle,
	type TagTrailTag,
} from "$lib/connect";
import { onDestroy } from "svelte";
import type { TagTrailHop } from "$lib/schema/tagTrail";
import RandomFeedCard from "./RandomFeedCard.svelte";
import TagArticleList from "./TagArticleList.svelte";
import TagTrailBreadcrumb from "./TagTrailBreadcrumb.svelte";

// Create transport for connect-rpc calls
const transport = createClientTransport();

interface FeedData {
	id: string;
	url: string;
	title?: string;
	description?: string;
	tags?: TagTrailTag[];
}

interface Props {
	initialFeed?: FeedData | null;
}

const { initialFeed }: Props = $props();

// State - using $derived for initial prop to satisfy Svelte 5 reactivity
const initialFeedValue = $derived(initialFeed);
let currentFeed = $state<FeedData | null>(null);

// Initialize currentFeed from props on mount
$effect(() => {
	if (currentFeed === null && initialFeedValue) {
		currentFeed = initialFeedValue;
	}
});
let feedTags = $state<TagTrailTag[]>([]);
let isLoadingFeedTags = $state(false);
let isLoadingFeed = $state(false);

let selectedTag = $state<TagTrailTag | null>(null);
let articles = $state<TagTrailArticle[]>([]);
let isLoadingArticles = $state(false);
let hasMoreArticles = $state(false);
let nextCursor = $state<string | undefined>(undefined);

let hops = $state<TagTrailHop[]>([]);

// Cache for article tags (articleId -> tags)
let articleTagsCache = $state<Map<string, TagTrailTag[]>>(new Map());
let loadingArticleTags = $state<Set<string>>(new Set());

// Track active streaming abort controllers for cleanup
let activeStreamControllers = $state<Map<string, AbortController>>(new Map());

// Cleanup on component destroy
onDestroy(() => {
	for (const controller of activeStreamControllers.values()) {
		controller.abort();
	}
	activeStreamControllers = new Map();
});

// Derived state
const isShowingArticles = $derived(selectedTag !== null);

// Set tags from currentFeed when it changes (ADR-173: tags come from fetchRandomFeed response)
// If no tags, trigger async fetch via fetchArticleContent -> streamArticleTags
$effect(() => {
	if (currentFeed) {
		if (currentFeed.tags && currentFeed.tags.length > 0) {
			// Tags available from backend (existing article had tags)
			feedTags = currentFeed.tags;
			isLoadingFeedTags = false;
		} else {
			// No tags: backend has triggered async article fetch
			// We poll via fetchArticleContent -> streamArticleTags
			isLoadingFeedTags = true;
			loadFeedTagsAsync(currentFeed.url);
		}
	}
});

/**
 * Async load feed tags when backend returned no tags.
 * Backend has already triggered async article fetch in goroutine.
 * We call fetchArticleContent to get articleId, then stream tags.
 */
async function loadFeedTagsAsync(feedUrl: string) {
	try {
		// 1. Fetch article content to get articleId
		const result = await fetchArticleContent(transport, feedUrl);
		if (result.articleId) {
			// 2. Stream tags for the article
			const controller = streamArticleTags(
				transport,
				result.articleId,
				(event) => {
					if (event.eventType === "cached" || event.eventType === "completed") {
						feedTags = event.tags;
						isLoadingFeedTags = false;
					}
					// generating: keep loading state
					// error: handled below
				},
				(error) => {
					console.error("Failed to stream feed tags:", error);
					isLoadingFeedTags = false;
				},
			);
			// Track for cleanup on refresh/navigation
			activeStreamControllers = new Map([
				...activeStreamControllers,
				[result.articleId, controller],
			]);
		} else {
			// No articleId returned (rare edge case)
			isLoadingFeedTags = false;
		}
	} catch (error) {
		console.error("Failed to load feed tags:", error);
		isLoadingFeedTags = false;
	}
}

// Load tags when articles are loaded
$effect(() => {
	if (articles.length > 0) {
		for (const article of articles) {
			if (
				!articleTagsCache.has(article.id) &&
				!loadingArticleTags.has(article.id)
			) {
				loadArticleTags(article.id);
			}
		}
	}
});

// Note: loadFeedTags is no longer needed - tags come from fetchRandomFeed (ADR-173)

/**
 * Loads article tags using Connect-RPC Server Streaming.
 * This provides real-time feedback for tag generation progress.
 */
function loadArticleTags(articleId: string) {
	if (loadingArticleTags.has(articleId)) return;
	if (articleTagsCache.has(articleId)) return;

	// Mark as loading
	loadingArticleTags = new Set([...loadingArticleTags, articleId]);

	// Start streaming
	const controller = streamArticleTags(
		transport,
		articleId,
		(event) => {
			switch (event.eventType) {
				case "cached":
				case "completed": {
					// Update cache with received tags
					articleTagsCache = new Map([
						...articleTagsCache,
						[articleId, event.tags],
					]);
					// Remove from loading state
					const newLoadingSet = new Set(loadingArticleTags);
					newLoadingSet.delete(articleId);
					loadingArticleTags = newLoadingSet;
					// Remove controller from active list
					activeStreamControllers.delete(articleId);
					break;
				}
				case "generating":
					// Keep loading state - tags are being generated
					// Could add UI feedback here if needed (e.g., "Generating...")
					break;
				case "error": {
					console.error("Failed to load article tags:", event.message);
					// Set empty tags on error
					articleTagsCache = new Map([...articleTagsCache, [articleId, []]]);
					const newLoadingSetErr = new Set(loadingArticleTags);
					newLoadingSetErr.delete(articleId);
					loadingArticleTags = newLoadingSetErr;
					activeStreamControllers.delete(articleId);
					break;
				}
			}
		},
		(error) => {
			console.error("Tag stream error:", error);
			articleTagsCache = new Map([...articleTagsCache, [articleId, []]]);
			const newLoadingSetErr = new Set(loadingArticleTags);
			newLoadingSetErr.delete(articleId);
			loadingArticleTags = newLoadingSetErr;
			activeStreamControllers.delete(articleId);
		},
	);

	// Track controller for cleanup
	activeStreamControllers = new Map([
		...activeStreamControllers,
		[articleId, controller],
	]);
}

function getArticleTags(articleId: string): TagTrailTag[] {
	return articleTagsCache.get(articleId) ?? [];
}

async function handleRefresh() {
	isLoadingFeed = true;
	try {
		const feed = await fetchRandomFeed(transport);
		// ADR-173: tags are now included in the fetchRandomFeed response
		currentFeed = feed;
		feedTags = feed.tags ?? [];
		selectedTag = null;
		articles = [];
		hops = [];
		// Abort all active tag streams and clear cache
		for (const controller of activeStreamControllers.values()) {
			controller.abort();
		}
		activeStreamControllers = new Map();
		articleTagsCache = new Map();
		loadingArticleTags = new Set();
	} catch (error) {
		console.error("Failed to get random feed:", error);
	} finally {
		isLoadingFeed = false;
	}
}

async function handleTagClick(tag: TagTrailTag) {
	selectedTag = tag;
	articles = [];
	nextCursor = undefined;
	hasMoreArticles = false;

	// Add to breadcrumb trail
	if (hops.length === 0 && currentFeed) {
		hops = [
			{ type: "feed", id: currentFeed.id, name: currentFeed.title || "Feed" },
			{ type: "tag", id: tag.id, name: tag.name },
		];
	} else {
		hops = [...hops, { type: "tag", id: tag.id, name: tag.name }];
	}

	// Use tag name for cross-feed article discovery
	await loadArticles(tag.name);
}

async function loadArticles(tagName: string) {
	isLoadingArticles = true;
	try {
		// Use tag name for cross-feed discovery via connect-rpc
		const response = await fetchArticlesByTag(
			transport,
			tagName,
			undefined,
			nextCursor ?? undefined,
		);
		articles = [...articles, ...response.articles];
		nextCursor = response.nextCursor ?? undefined;
		hasMoreArticles = response.hasMore;
	} catch (error) {
		console.error("Failed to load articles:", error);
	} finally {
		isLoadingArticles = false;
	}
}

function handleLoadMore() {
	if (selectedTag && nextCursor) {
		loadArticles(selectedTag.name);
	}
}

function handleHopClick(index: number) {
	if (index === -1) {
		// Go back to start
		selectedTag = null;
		articles = [];
		hops = [];
		// Abort all active tag streams and clear cache
		for (const controller of activeStreamControllers.values()) {
			controller.abort();
		}
		activeStreamControllers = new Map();
		articleTagsCache = new Map();
		loadingArticleTags = new Set();
	} else if (index < hops.length - 1) {
		// Go back to a previous hop
		const targetHop = hops[index];
		hops = hops.slice(0, index + 1);

		if (targetHop.type === "feed") {
			selectedTag = null;
			articles = [];
		} else {
			selectedTag = { id: targetHop.id, name: targetHop.name };
			articles = [];
			nextCursor = undefined;
			// Use tag name for cross-feed article discovery
			loadArticles(targetHop.name);
		}
	}
}

function handleBack() {
	if (hops.length > 1) {
		handleHopClick(hops.length - 2);
	} else {
		handleHopClick(-1);
	}
}
</script>

<div class="flex flex-col h-full">
	<!-- Header -->
	<div
		class="flex items-center gap-2 px-4 py-3 border-b"
		style="border-color: var(--surface-border); background: var(--surface-bg);"
	>
		{#if isShowingArticles}
			<button
				type="button"
				class="min-h-[44px] min-w-[44px] flex items-center justify-center rounded-full hover:bg-muted active:scale-95 transition-all"
				onclick={handleBack}
				aria-label="Go back"
			>
				<ArrowLeft size={20} style="color: var(--text-primary);" />
			</button>
		{:else}
			<div class="w-8 h-8 flex items-center justify-center">
				<Shuffle size={20} style="color: var(--alt-primary);" />
			</div>
		{/if}
		<h1 class="text-lg font-semibold flex-1" style="color: var(--text-primary);">
			Tag Trail
		</h1>
	</div>

	<!-- Breadcrumb -->
	{#if hops.length > 0}
		<TagTrailBreadcrumb {hops} onHopClick={handleHopClick} />
	{/if}

	<!-- Content -->
	<div class="flex-1 min-h-0 flex flex-col overflow-hidden">
		{#if isLoadingFeed}
			<div class="flex-1 flex items-center justify-center">
				<Loader2 size={32} class="animate-spin" style="color: var(--alt-primary);" />
			</div>
		{:else if !isShowingArticles}
			<!-- Random feed view -->
			<div class="flex-1 overflow-y-auto py-4">
				{#if currentFeed}
					<RandomFeedCard
						feed={currentFeed}
						tags={feedTags}
						isLoadingTags={isLoadingFeedTags}
						onTagClick={handleTagClick}
						onRefresh={handleRefresh}
					/>
				{:else}
					<div class="flex flex-col items-center justify-center h-full px-4">
						<p class="text-center mb-4" style="color: var(--text-secondary);">
							No subscriptions found. Add some feeds to start exploring!
						</p>
						<a
							href="/sv/mobile/feeds/manage"
							class="px-4 py-2 rounded-lg font-medium min-h-[44px] flex items-center"
							style="background: var(--alt-primary); color: var(--text-primary);"
						>
							Manage Feeds
						</a>
					</div>
				{/if}
			</div>
		{:else if selectedTag}
			<!-- Articles by tag view -->
			<TagArticleList
				{articles}
				isLoading={isLoadingArticles}
				hasMore={hasMoreArticles}
				selectedTagName={selectedTag.name}
				onTagClick={handleTagClick}
				onLoadMore={handleLoadMore}
				{getArticleTags}
				{loadingArticleTags}
			/>
		{/if}
	</div>
</div>
