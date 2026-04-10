<script lang="ts">
import { RefreshCw, ChevronRight, ExternalLink } from "@lucide/svelte";
import {
	createClientTransport,
	fetchArticlesByTag,
	fetchRandomFeed,
	streamArticleTags,
	type TagTrailArticle,
	type TagTrailTag,
} from "$lib/connect";
import { onDestroy, untrack } from "svelte";
import type { TagTrailHop } from "$lib/schema/tagTrail";

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

// State
const initialFeedValue = $derived(initialFeed);
let currentFeed = $state<FeedData | null>(null);

$effect(() => {
	if (currentFeed === null && initialFeedValue) {
		currentFeed = initialFeedValue;
	}
});

let feedTags = $state<TagTrailTag[]>([]);
let isLoadingFeedTags = $state(false);
let isLoadingFeed = $state(false);
let refreshError = $state<string | null>(null);
let feedTagsError = $state<string | null>(null);
let feedTagsTimeoutId: ReturnType<typeof setTimeout> | null = null;

let selectedTag = $state<TagTrailTag | null>(null);
let articles = $state<TagTrailArticle[]>([]);
let isLoadingArticles = $state(false);
let hasMoreArticles = $state(false);
let nextCursor = $state<string | undefined>(undefined);

let hops = $state<TagTrailHop[]>([]);

// Cache for article tags
let articleTagsCache = $state<Map<string, TagTrailTag[]>>(new Map());
let loadingArticleTags = $state<Set<string>>(new Set());

// Track active streaming abort controllers for cleanup
let activeStreamControllers: Map<string, AbortController> = new Map();

onDestroy(() => {
	for (const controller of activeStreamControllers.values()) {
		controller.abort();
	}
	activeStreamControllers = new Map();
	if (feedTagsTimeoutId) {
		clearTimeout(feedTagsTimeoutId);
		feedTagsTimeoutId = null;
	}
});

// Set tags from currentFeed when it changes
$effect(() => {
	if (currentFeed) {
		if (currentFeed.tags && currentFeed.tags.length > 0) {
			feedTags = currentFeed.tags;
			isLoadingFeedTags = false;
		} else if (currentFeed.latestArticleId) {
			isLoadingFeedTags = true;
			const articleId = currentFeed.latestArticleId;
			untrack(() => streamFeedTags(articleId));
		} else {
			feedTags = [];
			isLoadingFeedTags = false;
		}
	}
});

function streamFeedTags(articleId: string) {
	feedTagsError = null;

	if (feedTagsTimeoutId) {
		clearTimeout(feedTagsTimeoutId);
		feedTagsTimeoutId = null;
	}

	feedTagsTimeoutId = setTimeout(() => {
		if (isLoadingFeedTags) {
			feedTagsError = "Tag generation timed out. Please refresh.";
			isLoadingFeedTags = false;
			const ctrl = activeStreamControllers.get(articleId);
			if (ctrl) {
				ctrl.abort();
				activeStreamControllers.delete(articleId);
				activeStreamControllers = new Map(activeStreamControllers);
			}
		}
	}, 45000);

	const controller = streamArticleTags(
		transport,
		articleId,
		(event) => {
			if (event.eventType === "cached" || event.eventType === "completed") {
				feedTags = event.tags;
				isLoadingFeedTags = false;
				feedTagsError = null;
				if (feedTagsTimeoutId) {
					clearTimeout(feedTagsTimeoutId);
					feedTagsTimeoutId = null;
				}
			} else if (event.eventType === "error") {
				feedTagsError = event.message || "Failed to generate tags.";
				isLoadingFeedTags = false;
				if (feedTagsTimeoutId) {
					clearTimeout(feedTagsTimeoutId);
					feedTagsTimeoutId = null;
				}
			}
		},
		(error) => {
			console.error("Failed to stream feed tags:", error);
			feedTagsError = "Failed to load tags. Please refresh.";
			isLoadingFeedTags = false;
			if (feedTagsTimeoutId) {
				clearTimeout(feedTagsTimeoutId);
				feedTagsTimeoutId = null;
			}
		},
	);
	activeStreamControllers = new Map([
		...activeStreamControllers,
		[articleId, controller],
	]);
}

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

function loadArticleTags(articleId: string) {
	if (loadingArticleTags.has(articleId)) return;
	if (articleTagsCache.has(articleId)) return;

	loadingArticleTags = new Set([...loadingArticleTags, articleId]);

	const controller = streamArticleTags(
		transport,
		articleId,
		(event) => {
			switch (event.eventType) {
				case "cached":
				case "completed": {
					articleTagsCache = new Map([
						...articleTagsCache,
						[articleId, event.tags],
					]);
					const newLoadingSet = new Set(loadingArticleTags);
					newLoadingSet.delete(articleId);
					loadingArticleTags = newLoadingSet;
					activeStreamControllers.delete(articleId);
					break;
				}
				case "generating":
					break;
				case "error": {
					console.error("Failed to load article tags:", event.message);
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
	feedTagsError = null;
	if (feedTagsTimeoutId) {
		clearTimeout(feedTagsTimeoutId);
		feedTagsTimeoutId = null;
	}
	try {
		const feed = await fetchRandomFeed(transport);
		refreshError = null;
		currentFeed = feed;
		feedTags = feed.tags ?? [];
		selectedTag = null;
		articles = [];
		hops = [];
		for (const controller of activeStreamControllers.values()) {
			controller.abort();
		}
		activeStreamControllers = new Map();
		articleTagsCache = new Map();
		loadingArticleTags = new Set();
	} catch (error) {
		console.error("Failed to get random feed:", error);
		refreshError = "Failed to load feed. Please try again.";
	} finally {
		isLoadingFeed = false;
	}
}

async function handleTagClick(tag: TagTrailTag) {
	selectedTag = tag;
	articles = [];
	nextCursor = undefined;
	hasMoreArticles = false;

	if (hops.length === 0 && currentFeed) {
		hops = [
			{ type: "feed", id: currentFeed.id, name: currentFeed.title || "Feed" },
			{ type: "tag", id: tag.id, name: tag.name },
		];
	} else {
		hops = [...hops, { type: "tag", id: tag.id, name: tag.name }];
	}

	await loadArticles(tag.name);
}

async function loadArticles(tagName: string) {
	isLoadingArticles = true;
	try {
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
		selectedTag = null;
		articles = [];
		hops = [];
		for (const controller of activeStreamControllers.values()) {
			controller.abort();
		}
		activeStreamControllers = new Map();
		articleTagsCache = new Map();
		loadingArticleTags = new Set();
	} else if (index < hops.length - 1) {
		const targetHop = hops[index];
		hops = hops.slice(0, index + 1);

		if (targetHop.type === "feed") {
			selectedTag = null;
			articles = [];
		} else {
			selectedTag = { id: targetHop.id, name: targetHop.name };
			articles = [];
			nextCursor = undefined;
			loadArticles(targetHop.name);
		}
	}
}

const dateStr = new Date().toLocaleDateString("en-US", {
	weekday: "long",
	year: "numeric",
	month: "long",
	day: "numeric",
});

const formatDate = (dateStr: string) => {
	const date = new Date(dateStr);
	return date.toLocaleDateString(undefined, {
		month: "short",
		day: "numeric",
		year: "numeric",
	});
};
</script>

<div class="flex flex-col h-full">
	<!-- Masthead -->
	<header class="trail-masthead">
		<div class="masthead-rule"></div>
		<div class="masthead-inner">
			<span class="masthead-date">{dateStr}</span>
			<h1 class="masthead-title">Tag Trail</h1>
			<p class="masthead-sub">Topic Cross-Reference &amp; Discovery</p>
		</div>
		<div class="masthead-rule"></div>
	</header>

	<!-- Toolbar -->
	<nav class="trail-toolbar">
		<span class="toolbar-label">
			{#if selectedTag}
				{articles.length} article{articles.length !== 1 ? "s" : ""} found
			{:else}
				Serendipitous exploration
			{/if}
		</span>
		<button
			type="button"
			onclick={handleRefresh}
			disabled={isLoadingFeed}
			class="editorial-btn"
			class:editorial-btn--disabled={isLoadingFeed}
		>
			<RefreshCw size={14} class={isLoadingFeed ? "animate-spin" : ""} />
			New Random Feed
		</button>
	</nav>

	<!-- Main Content: Master-Detail Layout -->
	<div class="flex-1 min-h-0 flex gap-6 p-6">
		<!-- Left Panel: Feed Card + Trail History (Master) -->
		<aside class="w-80 flex-shrink-0 flex flex-col gap-4">
			<!-- Feed Card -->
			<div class="aside-panel">
				{#if isLoadingFeed}
					<div class="flex items-center justify-center gap-3 py-12">
						<div class="loading-pulse"></div>
						<span class="loading-text">Discovering&hellip;</span>
					</div>
				{:else if refreshError}
					<div class="error-stripe">
						<p class="error-text">{refreshError}</p>
						<button
							type="button"
							onclick={handleRefresh}
							class="error-link"
						>
							Try again
						</button>
					</div>
				{:else if currentFeed}
					<!-- Feed Info -->
					<div class="flex flex-col gap-1 mb-4">
						<span class="section-label">Featured Section</span>
						<a
							href={currentFeed.url}
							target="_blank"
							rel="noopener noreferrer"
							class="feed-title"
						>
							{currentFeed.title || currentFeed.url}
							<ExternalLink size={12} class="inline-block flex-shrink-0 opacity-40" />
						</a>
						{#if currentFeed.description}
							<p class="feed-desc">{currentFeed.description}</p>
						{/if}
					</div>

					<!-- Tags Section -->
					<div class="tags-section">
						<span class="section-label">Explore Topics</span>
						{#if feedTagsError}
							<div class="error-stripe">
								<p class="error-text">{feedTagsError}</p>
								<button
									type="button"
									onclick={handleRefresh}
									class="error-link"
								>
									Refresh
								</button>
							</div>
						{:else if isLoadingFeedTags}
							<div class="flex flex-wrap gap-2 mb-2">
								{#each [1, 2, 3, 4] as _i}
									<div class="skeleton-tag"></div>
								{/each}
							</div>
							<div class="flex items-center gap-2">
								<div class="loading-pulse"></div>
								<span class="loading-text">Generating tags&hellip;</span>
							</div>
						{:else if feedTags.length > 0}
							<div class="flex flex-wrap gap-2">
								{#each feedTags as tag, i}
									<button
										type="button"
										onclick={() => handleTagClick(tag)}
										class="index-entry"
										class:index-entry--active={selectedTag?.id === tag.id}
										style="--stagger: {i};"
									>
										{tag.name}
									</button>
								{/each}
							</div>
						{:else}
							<p class="empty-hint">No tags available</p>
						{/if}
					</div>
				{:else}
					<!-- No Feed State -->
					<div class="flex flex-col items-center py-8 px-4">
						<div class="empty-ornament">&#9670;</div>
						<p class="empty-hint">
							No subscriptions found. Add some feeds to start exploring.
						</p>
						<a href="/settings/feeds" class="editorial-btn mt-4">
							Manage Feeds
						</a>
					</div>
				{/if}
			</div>

			<!-- Trail History Section -->
			{#if hops.length > 0}
				<div class="aside-panel" data-testid="trail-breadcrumb">
					<span class="section-label">Trail History</span>
					<div class="overflow-x-auto scrollbar-thin">
						<div class="flex flex-nowrap items-center gap-2 pb-2 min-w-max">
							<!-- Start -->
							<button
								type="button"
								onclick={() => handleHopClick(-1)}
								class="trail-node"
								aria-label="Go to start"
							>
								Start
							</button>

							{#each hops as hop, index}
								<ChevronRight size={12} class="flex-shrink-0" style="color: var(--surface-border, #c8c8c8);" />
								<button
									type="button"
									onclick={() => handleHopClick(index)}
									disabled={index === hops.length - 1}
									class="trail-node"
									class:trail-node--current={index === hops.length - 1}
								>
									{hop.name}
								</button>
							{/each}
						</div>
					</div>
				</div>
			{/if}
		</aside>

		<!-- Right Panel: Articles (Detail) -->
		<main class="flex-1 min-w-0 flex flex-col">
			{#if selectedTag}
				<div class="article-header">
					<span class="section-label">Cross-Referenced Stories</span>
					<h2 class="article-heading">
						{selectedTag.name}
					</h2>
					<span class="article-count">
						{articles.length} article{articles.length !== 1 ? "s" : ""}
					</span>
				</div>

				{#if isLoadingArticles && articles.length === 0}
					<div class="flex items-center justify-center gap-3 py-12">
						<div class="loading-pulse"></div>
						<span class="loading-text">Searching across subscriptions&hellip;</span>
					</div>
				{:else if articles.length > 0}
					<!-- Article Grid -->
					<div class="flex-1 overflow-y-auto">
						<div class="grid grid-cols-1 lg:grid-cols-2 2xl:grid-cols-3 gap-4 pb-4">
							{#each articles as article, i (article.id)}
								<article
									class="article-card"
									style="--stagger: {i};"
								>
									<a
										href={article.link}
										target="_blank"
										rel="noopener noreferrer"
										class="block"
									>
										<h3 class="card-title">{article.title}</h3>
										<div class="card-meta">
											{#if article.feedTitle}
												<span class="meta-source">{article.feedTitle}</span>
												<span class="meta-dot">&middot;</span>
											{/if}
											<span>{formatDate(article.publishedAt)}</span>
										</div>
									</a>

									<!-- Article Tags -->
									<div class="card-tags">
										{#if loadingArticleTags.has(article.id)}
											<div class="flex gap-1">
												{#each [1, 2] as _i}
													<div class="skeleton-tag-sm"></div>
												{/each}
											</div>
										{:else if getArticleTags(article.id).length > 0}
											<div class="flex flex-wrap gap-1.5">
												{#each getArticleTags(article.id).slice(0, 5) as tag}
													<button
														type="button"
														onclick={() => handleTagClick(tag)}
														class="article-tag"
													>
														{tag.name}
													</button>
												{/each}
												{#if getArticleTags(article.id).length > 5}
													<span class="tag-overflow">
														+{getArticleTags(article.id).length - 5}
													</span>
												{/if}
											</div>
										{:else}
											<span class="empty-hint">No tags</span>
										{/if}
									</div>
								</article>
							{/each}
						</div>

						{#if isLoadingArticles}
							<div class="flex items-center justify-center gap-3 py-4">
								<div class="loading-pulse"></div>
								<span class="loading-text">Loading&hellip;</span>
							</div>
						{/if}

						{#if !isLoadingArticles && hasMoreArticles}
							<div class="flex justify-center py-4">
								<button
									type="button"
									onclick={handleLoadMore}
									class="editorial-btn"
								>
									Load More Articles
								</button>
							</div>
						{/if}
					</div>
				{:else}
					<div class="flex-1 flex items-center justify-center">
						<p class="empty-hint">No articles found with this tag</p>
					</div>
				{/if}
			{:else}
				<!-- Welcome State -->
				<div class="flex-1 flex items-center justify-center">
					<div class="welcome-panel">
						<div class="empty-ornament">&#9670;</div>
						<h2 class="welcome-title">Start Your Tag Trail</h2>
						<p class="welcome-text">
							Click on a tag from the feed on the left to discover related articles across all your subscriptions.
							Follow the tag trail to explore new content.
						</p>
					</div>
				</div>
			{/if}
		</main>
	</div>
</div>

<style>
	/* ===== Masthead ===== */
	.trail-masthead {
		padding: 0 1.5rem;
	}

	.masthead-rule {
		height: 2px;
		background: var(--alt-charcoal, #1a1a1a);
	}

	.masthead-inner {
		text-align: center;
		padding: 0.75rem 0;
	}

	.masthead-date {
		display: block;
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.65rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.14em;
		color: var(--alt-ash, #999);
	}

	.masthead-title {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: clamp(2rem, 4vw, 2.5rem);
		font-weight: 900;
		letter-spacing: -0.02em;
		line-height: 1.1;
		color: var(--alt-charcoal, #1a1a1a);
		margin: 0.15rem 0 0;
	}

	.masthead-sub {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.75rem;
		font-style: italic;
		color: var(--alt-slate, #666);
		margin: 0.15rem 0 0;
	}

	/* ===== Toolbar ===== */
	.trail-toolbar {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0.5rem 1.5rem;
		border-bottom: 1px solid var(--surface-border, #c8c8c8);
	}

	.toolbar-label {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.75rem;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--alt-ash, #999);
	}

	/* ===== Aside Panels ===== */
	.aside-panel {
		padding: 1rem;
		border: 1px solid var(--surface-border, #c8c8c8);
		background: var(--surface-bg, #faf9f7);
	}

	/* ===== Section Label ===== */
	.section-label {
		display: block;
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.65rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--alt-ash, #999);
		margin-bottom: 0.35rem;
	}

	/* ===== Feed Title / Desc ===== */
	.feed-title {
		display: flex;
		align-items: baseline;
		gap: 0.35rem;
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.15rem;
		font-weight: 700;
		line-height: 1.3;
		color: var(--alt-charcoal, #1a1a1a);
		text-decoration: none;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.feed-title:hover {
		text-decoration: underline;
		text-underline-offset: 2px;
	}

	.feed-desc {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.8rem;
		font-style: italic;
		line-height: 1.5;
		color: var(--alt-slate, #666);
		margin: 0;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	/* ===== Tags Section ===== */
	.tags-section {
		border-top: 1px solid var(--surface-border, #c8c8c8);
		padding-top: 0.75rem;
	}

	/* ===== Index Entry (Tags) ===== */
	.index-entry {
		display: inline-flex;
		align-items: center;
		padding: 0.35rem 0.6rem;
		min-height: 32px;

		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.7rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.06em;

		color: var(--alt-slate, #666);
		background: transparent;
		border: 1px solid var(--surface-border, #c8c8c8);
		cursor: pointer;
		transition: background 0.15s, color 0.15s, border-color 0.15s;

		opacity: 0;
		animation: entry-in 0.3s ease forwards;
		animation-delay: calc(var(--stagger) * 60ms);
	}

	.index-entry:hover {
		background: var(--alt-charcoal, #1a1a1a);
		color: var(--surface-bg, #faf9f7);
		border-color: var(--alt-charcoal, #1a1a1a);
	}

	.index-entry--active {
		background: var(--alt-charcoal, #1a1a1a);
		color: var(--surface-bg, #faf9f7);
		border-color: var(--alt-charcoal, #1a1a1a);
	}

	/* ===== Trail Node (Breadcrumb) ===== */
	.trail-node {
		display: inline-flex;
		align-items: center;
		flex-shrink: 0;
		padding: 0.3rem 0.6rem;
		max-width: 80px;

		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.6rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;

		color: var(--alt-ash, #999);
		background: transparent;
		border: 1px solid var(--surface-border, #c8c8c8);
		cursor: pointer;
		transition: background 0.15s, color 0.15s, border-color 0.15s;
	}

	.trail-node:hover:not(:disabled) {
		background: var(--alt-charcoal, #1a1a1a);
		color: var(--surface-bg, #faf9f7);
		border-color: var(--alt-charcoal, #1a1a1a);
	}

	.trail-node--current {
		background: var(--alt-charcoal, #1a1a1a);
		color: var(--surface-bg, #faf9f7);
		border-color: var(--alt-charcoal, #1a1a1a);
		cursor: default;
	}

	/* ===== Article Header ===== */
	.article-header {
		margin-bottom: 1rem;
		padding-bottom: 0.5rem;
		border-bottom: 1px solid var(--surface-border, #c8c8c8);
	}

	.article-heading {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.15rem;
		font-weight: 700;
		line-height: 1.3;
		color: var(--alt-charcoal, #1a1a1a);
		margin: 0;
	}

	.article-count {
		font-family: var(--font-mono, "IBM Plex Mono", monospace);
		font-size: 0.65rem;
		color: var(--alt-ash, #999);
	}

	/* ===== Article Card ===== */
	.article-card {
		padding: 1rem;
		border: 1px solid var(--surface-border, #c8c8c8);
		background: var(--surface-bg, #faf9f7);
		transition: border-color 0.15s;

		opacity: 0;
		animation: entry-in 0.3s ease forwards;
		animation-delay: calc(var(--stagger) * 60ms);
	}

	.article-card:hover {
		border-color: var(--alt-charcoal, #1a1a1a);
	}

	.card-title {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1rem;
		font-weight: 700;
		line-height: 1.35;
		color: var(--alt-charcoal, #1a1a1a);
		margin: 0 0 0.35rem;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.card-meta {
		display: flex;
		align-items: center;
		gap: 0.35rem;
		font-family: var(--font-mono, "IBM Plex Mono", monospace);
		font-size: 0.65rem;
		color: var(--alt-ash, #999);
	}

	.meta-source {
		max-width: 150px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.meta-dot {
		color: var(--surface-border, #c8c8c8);
	}

	.card-tags {
		margin-top: 0.75rem;
		padding-top: 0.75rem;
		border-top: 1px solid var(--surface-border, #c8c8c8);
	}

	/* ===== Article Tag (in cards) ===== */
	.article-tag {
		display: inline-flex;
		align-items: center;
		padding: 0.2rem 0.5rem;

		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.65rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.04em;

		color: var(--alt-slate, #666);
		background: transparent;
		border: 1px solid var(--surface-border, #c8c8c8);
		cursor: pointer;
		transition: background 0.15s, color 0.15s, border-color 0.15s;
	}

	.article-tag:hover {
		background: var(--alt-charcoal, #1a1a1a);
		color: var(--surface-bg, #faf9f7);
		border-color: var(--alt-charcoal, #1a1a1a);
	}

	.tag-overflow {
		font-family: var(--font-mono, "IBM Plex Mono", monospace);
		font-size: 0.6rem;
		color: var(--alt-ash, #999);
		padding: 0.2rem 0.3rem;
	}

	/* ===== Editorial Button ===== */
	.editorial-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		min-height: 44px;
		padding: 0.5rem 1rem;

		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.8rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		text-decoration: none;

		color: var(--alt-charcoal, #1a1a1a);
		background: transparent;
		border: 1.5px solid var(--alt-charcoal, #1a1a1a);
		cursor: pointer;
		transition: background 0.2s, color 0.2s;
	}

	.editorial-btn:hover {
		background: var(--alt-charcoal, #1a1a1a);
		color: var(--surface-bg, #faf9f7);
	}

	.editorial-btn--disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	/* ===== Welcome State ===== */
	.welcome-panel {
		text-align: center;
		max-width: 28rem;
	}

	.welcome-title {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.3rem;
		font-weight: 700;
		color: var(--alt-charcoal, #1a1a1a);
		margin: 0 0 0.5rem;
	}

	.welcome-text {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.9rem;
		line-height: 1.6;
		color: var(--alt-slate, #666);
		margin: 0;
	}

	/* ===== Shared Utilities ===== */
	.empty-ornament {
		font-size: 1.5rem;
		color: var(--surface-border, #c8c8c8);
		margin-bottom: 0.75rem;
	}

	.empty-hint {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.8rem;
		font-style: italic;
		color: var(--alt-ash, #999);
		margin: 0;
	}

	.loading-pulse {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--alt-ash, #999);
		animation: pulse 1.2s ease-in-out infinite;
	}

	.loading-text {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.85rem;
		font-style: italic;
		color: var(--alt-ash, #999);
	}

	.skeleton-tag {
		height: 32px;
		width: 80px;
		background: var(--muted);
		animation: shimmer 1.5s ease-in-out infinite;
	}

	.skeleton-tag-sm {
		height: 24px;
		width: 56px;
		background: var(--muted);
		animation: shimmer 1.5s ease-in-out infinite;
	}

	.error-stripe {
		padding: 0.5rem 0.75rem;
		border-left: 3px solid var(--alt-terracotta, #b85450);
		background: #fef2f2;
	}

	.error-text {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.8rem;
		color: var(--alt-terracotta, #b85450);
		margin: 0;
	}

	.error-link {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.7rem;
		color: var(--alt-primary, #2f4f4f);
		background: transparent;
		border: none;
		cursor: pointer;
		text-decoration: underline;
		text-underline-offset: 2px;
		padding: 0;
		margin-top: 0.25rem;
	}

	/* ===== Animations ===== */
	@keyframes pulse {
		0%, 100% { opacity: 0.3; }
		50% { opacity: 1; }
	}

	@keyframes shimmer {
		0%, 100% { opacity: 0.5; }
		50% { opacity: 1; }
	}

	@keyframes entry-in {
		to { opacity: 1; }
	}

	@media (prefers-reduced-motion: reduce) {
		.index-entry,
		.article-card {
			animation: none;
			opacity: 1;
		}
		.loading-pulse { animation: none; opacity: 0.6; }
		.skeleton-tag,
		.skeleton-tag-sm { animation: none; opacity: 0.5; }
	}
</style>
