<script lang="ts">
import { ArrowLeft } from "@lucide/svelte";
import {
	createClientTransport,
	fetchArticlesByTag,
	fetchRandomFeed,
	streamArticleTags,
	type RandomFeed,
	type TagTrailArticle,
	type TagTrailTag,
} from "$lib/connect";
import { onDestroy, untrack } from "svelte";
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
	latestArticleId?: string;
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
let activeStreamControllers: Map<string, AbortController> = new Map();

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
$effect(() => {
	if (currentFeed) {
		if (currentFeed.tags && currentFeed.tags.length > 0) {
			// Tags available from backend (existing article had tags)
			feedTags = currentFeed.tags;
			isLoadingFeedTags = false;
		} else if (currentFeed.latestArticleId) {
			// Tags not included but we have article ID — stream tags directly
			isLoadingFeedTags = true;
			const articleId = currentFeed.latestArticleId;
			untrack(() => streamFeedTags(articleId));
		} else {
			// No tags and no article ID — nothing to load
			feedTags = [];
			isLoadingFeedTags = false;
		}
	}
});

/**
 * Stream tags for a feed's latest article using its article ID directly.
 * Avoids the FetchArticleContent roundtrip that returned unnecessary HTML.
 */
function streamFeedTags(articleId: string) {
	const controller = streamArticleTags(
		transport,
		articleId,
		(event) => {
			if (event.eventType === "cached" || event.eventType === "completed") {
				feedTags = event.tags;
				isLoadingFeedTags = false;
			}
		},
		(error) => {
			console.error("Failed to stream feed tags:", error);
			isLoadingFeedTags = false;
		},
	);
	activeStreamControllers = new Map([
		...activeStreamControllers,
		[articleId, controller],
	]);
}

// Load tags when articles are loaded
$effect(() => {
	if (articles.length > 0) {
		for (const article of articles) {
			if (
				!untrack(() => articleTagsCache.has(article.id)) &&
				!untrack(() => loadingArticleTags.has(article.id))
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

<div class="flex flex-col h-full" data-role="tag-trail-screen">
	<!-- Masthead -->
	<header class="trail-masthead">
		<div class="masthead-rule"></div>
		<div class="masthead-content">
			{#if isShowingArticles}
				<button
					type="button"
					class="back-btn"
					onclick={handleBack}
					aria-label="Go back"
				>
					<ArrowLeft size={18} />
				</button>
			{/if}
			<div class="flex-1 text-center">
				<h1 class="masthead-title">Tag Trail</h1>
				<p class="masthead-sub">Topic Cross-Reference &amp; Discovery</p>
			</div>
			{#if isShowingArticles}
				<div class="min-w-[44px]"></div>
			{/if}
		</div>
		<div class="masthead-rule"></div>
	</header>

	<!-- Breadcrumb -->
	{#if hops.length > 0}
		<TagTrailBreadcrumb {hops} onHopClick={handleHopClick} />
	{/if}

	<!-- Content -->
	<div class="flex-1 min-h-0 flex flex-col overflow-hidden">
		{#if isLoadingFeed}
			<div class="flex-1 flex items-center justify-center gap-3">
				<div class="loading-pulse"></div>
				<span class="loading-text">Discovering a feed&hellip;</span>
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
						<div class="empty-ornament">&#9670;</div>
						<p class="empty-text">
							No subscriptions found. Add some feeds to start exploring.
						</p>
						<a href="/settings/feeds" class="editorial-btn">
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

<style>
	/* ===== Masthead ===== */
	.trail-masthead {
		padding: 0 1rem;
		background: var(--surface-bg, #faf9f7);
	}

	.masthead-rule {
		height: 2px;
		background: var(--alt-charcoal, #1a1a1a);
	}

	.masthead-content {
		display: flex;
		align-items: center;
		padding: 0.5rem 0;
	}

	.masthead-title {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.3rem;
		font-weight: 800;
		letter-spacing: -0.01em;
		line-height: 1.1;
		color: var(--alt-charcoal, #1a1a1a);
		margin: 0;
	}

	.masthead-sub {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.7rem;
		font-style: italic;
		color: var(--alt-slate, #666);
		margin: 0.1rem 0 0;
	}

	/* ===== Back button ===== */
	.back-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		min-height: 44px;
		min-width: 44px;

		color: var(--alt-charcoal, #1a1a1a);
		background: transparent;
		border: 1px solid var(--surface-border, #c8c8c8);
		cursor: pointer;
		transition: background 0.15s, color 0.15s, border-color 0.15s;
	}
	.back-btn:hover {
		background: var(--alt-charcoal, #1a1a1a);
		color: var(--surface-bg, #faf9f7);
		border-color: var(--alt-charcoal, #1a1a1a);
	}

	/* ===== Loading ===== */
	.loading-pulse {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--alt-ash, #999);
		animation: pulse 1.2s ease-in-out infinite;
	}
	@keyframes pulse {
		0%, 100% { opacity: 0.3; }
		50% { opacity: 1; }
	}
	.loading-text {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.85rem;
		font-style: italic;
		color: var(--alt-ash, #999);
	}

	/* ===== Empty state ===== */
	.empty-ornament {
		font-size: 1.5rem;
		color: var(--surface-border, #c8c8c8);
		margin-bottom: 0.75rem;
	}
	.empty-text {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.85rem;
		color: var(--alt-ash, #999);
		text-align: center;
		margin: 0 0 1rem;
	}
	.editorial-btn {
		display: inline-flex;
		align-items: center;
		min-height: 44px;
		padding: 0.5rem 1.25rem;

		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.8rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		text-decoration: none;

		color: var(--alt-charcoal, #1a1a1a);
		background: transparent;
		border: 1.5px solid var(--alt-charcoal, #1a1a1a);
		transition: background 0.15s, color 0.15s;
	}
	.editorial-btn:hover {
		background: var(--alt-charcoal, #1a1a1a);
		color: var(--surface-bg, #faf9f7);
	}

	@media (prefers-reduced-motion: reduce) {
		.loading-pulse { animation: none; opacity: 0.6; }
	}
</style>
