<script lang="ts">
import { Loader2, RefreshCw, Shuffle, Home, ChevronRight, ExternalLink, Tag } from "@lucide/svelte";
import {
	createClientTransport,
	fetchArticleContent,
	fetchArticlesByTag,
	fetchRandomFeed,
	streamArticleTags,
	type TagTrailArticle,
	type TagTrailTag,
} from "$lib/connect";
import { onDestroy } from "svelte";
import type { TagTrailHop } from "$lib/schema/tagTrail";
import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";

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
let activeStreamControllers = $state<Map<string, AbortController>>(new Map());

onDestroy(() => {
	for (const controller of activeStreamControllers.values()) {
		controller.abort();
	}
	activeStreamControllers = new Map();
});

// Set tags from currentFeed when it changes
$effect(() => {
	if (currentFeed) {
		if (currentFeed.tags && currentFeed.tags.length > 0) {
			feedTags = currentFeed.tags;
			isLoadingFeedTags = false;
		} else {
			isLoadingFeedTags = true;
			loadFeedTagsAsync(currentFeed.url);
		}
	}
});

async function loadFeedTagsAsync(feedUrl: string) {
	try {
		const result = await fetchArticleContent(transport, feedUrl);
		if (result.articleId) {
			const controller = streamArticleTags(
				transport,
				result.articleId,
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
			activeStreamControllers = new Map([...activeStreamControllers, [result.articleId, controller]]);
		} else {
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
					articleTagsCache = new Map([...articleTagsCache, [articleId, event.tags]]);
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

	activeStreamControllers = new Map([...activeStreamControllers, [articleId, controller]]);
}

function getArticleTags(articleId: string): TagTrailTag[] {
	return articleTagsCache.get(articleId) ?? [];
}

async function handleRefresh() {
	isLoadingFeed = true;
	try {
		const feed = await fetchRandomFeed(transport);
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

const formatDate = (dateStr: string) => {
	const date = new Date(dateStr);
	return date.toLocaleDateString(undefined, {
		month: "short",
		day: "numeric",
		year: "numeric",
	});
};
</script>

<div class="flex flex-col h-full py-6 pr-6">
	<!-- Header with Actions -->
	<PageHeader title="Tag Trail" description="Discover content through tag exploration">
		{#snippet actions()}
			<button
				type="button"
				onclick={handleRefresh}
				disabled={isLoadingFeed}
				class="flex items-center gap-2 px-4 py-2 text-sm font-medium rounded-lg transition-colors
					bg-[var(--surface-hover)] hover:bg-[var(--muted)] text-[var(--text-primary)]
					disabled:opacity-50 disabled:cursor-not-allowed"
			>
				<RefreshCw size={16} class={isLoadingFeed ? "animate-spin" : ""} />
				New Random Feed
			</button>
		{/snippet}
	</PageHeader>

	<!-- Main Content: Master-Detail Layout -->
	<div class="flex-1 min-h-0 flex gap-6">
		<!-- Left Panel: Feed Card + Trail History (Master) -->
		<aside class="w-80 flex-shrink-0 flex flex-col gap-4">
			<!-- Feed Card -->
			<div
				class="p-4 rounded-lg border bg-[var(--surface-bg)] border-[var(--surface-border)]"
			>
				{#if isLoadingFeed}
					<div class="flex items-center justify-center py-12">
						<Loader2 size={32} class="animate-spin text-[var(--alt-primary)]" />
					</div>
				{:else if currentFeed}
					<!-- Feed Info -->
					<div class="flex items-start gap-3 mb-4">
						<div class="w-10 h-10 rounded-lg bg-[var(--alt-primary)] bg-opacity-10 flex items-center justify-center flex-shrink-0">
							<Shuffle size={20} class="text-[var(--alt-primary)]" />
						</div>
						<div class="flex-1 min-w-0">
							<a
								href={currentFeed.url}
								target="_blank"
								rel="noopener noreferrer"
								class="text-base font-semibold text-[var(--text-primary)] hover:text-[var(--accent-primary)] hover:underline line-clamp-2 flex items-center gap-1"
							>
								{currentFeed.title || currentFeed.url}
								<ExternalLink size={14} class="flex-shrink-0 opacity-50" />
							</a>
							{#if currentFeed.description}
								<p class="text-sm text-[var(--text-secondary)] mt-1 line-clamp-2">
									{currentFeed.description}
								</p>
							{/if}
						</div>
					</div>

					<!-- Tags Section -->
					<div class="border-t border-[var(--surface-border)] pt-4">
						<h3 class="text-xs font-medium text-[var(--text-tertiary)] uppercase tracking-wide mb-3">
							Click a tag to explore
						</h3>
						{#if isLoadingFeedTags}
							<div class="flex flex-wrap gap-2">
								{#each [1, 2, 3, 4] as i}
									<div
										class="h-8 w-20 rounded-full animate-pulse bg-[var(--muted)]"
									></div>
								{/each}
							</div>
							<p class="text-xs text-[var(--text-tertiary)] mt-2">Generating tags...</p>
						{:else if feedTags.length > 0}
							<div class="flex flex-wrap gap-2">
								{#each feedTags as tag}
									<button
										type="button"
										onclick={() => handleTagClick(tag)}
										class="px-3 py-1.5 text-sm rounded-full transition-all
											{selectedTag?.id === tag.id
											? 'bg-[var(--alt-primary)] text-white font-medium'
											: 'bg-[var(--muted)] text-[var(--text-secondary)] hover:bg-[var(--surface-hover)] hover:text-[var(--text-primary)]'}"
									>
										{tag.name}
									</button>
								{/each}
							</div>
						{:else}
							<p class="text-sm text-[var(--text-secondary)]">No tags available</p>
						{/if}
					</div>
				{:else}
					<!-- No Feed State -->
					<div class="text-center py-8">
						<p class="text-[var(--text-secondary)] mb-4">
							No subscriptions found. Add some feeds to start exploring!
						</p>
						<a
							href="/sv/desktop/settings/feeds"
							class="inline-flex items-center gap-2 px-4 py-2 rounded-lg font-medium
								bg-[var(--alt-primary)] text-white hover:opacity-90 transition-opacity"
						>
							Manage Feeds
						</a>
					</div>
				{/if}
			</div>

			<!-- Trail History Section -->
			{#if hops.length > 0}
				<div
					class="p-4 rounded-lg border bg-[var(--surface-bg)] border-[var(--surface-border)]"
				>
					<h3 class="text-xs font-medium text-[var(--text-tertiary)] uppercase tracking-wide mb-3">
						Trail History
					</h3>
					<div class="overflow-x-auto scrollbar-thin">
						<div class="flex flex-nowrap items-center gap-2 pb-2 min-w-max">
							<!-- Start (Home) -->
							<button
								type="button"
								onclick={() => handleHopClick(-1)}
								class="flex-shrink-0 flex flex-col items-center gap-1 p-2 rounded-lg transition-colors
									hover:bg-[var(--surface-hover)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
							>
								<div class="w-8 h-8 rounded-full bg-[var(--muted)] flex items-center justify-center">
									<Home size={14} />
								</div>
								<span class="text-[10px] max-w-[50px] truncate">Start</span>
							</button>

							{#each hops as hop, index}
								<!-- Arrow -->
								<ChevronRight size={14} class="text-[var(--text-tertiary)] flex-shrink-0" />

								<!-- Hop Card -->
								<button
									type="button"
									onclick={() => handleHopClick(index)}
									disabled={index === hops.length - 1}
									class="flex-shrink-0 flex flex-col items-center gap-1 p-2 rounded-lg transition-colors
										{index === hops.length - 1
										? 'bg-[var(--alt-primary)] bg-opacity-10 ring-2 ring-[var(--alt-primary)] text-[var(--alt-primary)]'
										: 'hover:bg-[var(--surface-hover)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]'}"
								>
									<div class="w-8 h-8 rounded-full flex items-center justify-center
										{index === hops.length - 1
										? 'bg-[var(--alt-primary)] text-white'
										: 'bg-[var(--muted)]'}"
									>
										{#if hop.type === "feed"}
											<Shuffle size={14} />
										{:else}
											<Tag size={14} />
										{/if}
									</div>
									<span class="text-[10px] max-w-[60px] truncate font-medium">{hop.name}</span>
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
				<div class="mb-4">
					<h2 class="text-lg font-semibold text-[var(--text-primary)]">
						Articles tagged "{selectedTag.name}"
					</h2>
					<p class="text-sm text-[var(--text-secondary)]">
						{articles.length} article{articles.length !== 1 ? "s" : ""} found
					</p>
				</div>

				{#if isLoadingArticles && articles.length === 0}
					<div class="flex items-center justify-center py-12">
						<Loader2 size={32} class="animate-spin text-[var(--alt-primary)]" />
					</div>
				{:else if articles.length > 0}
					<!-- Article Grid -->
					<div class="flex-1 overflow-y-auto">
						<div class="grid grid-cols-1 lg:grid-cols-2 2xl:grid-cols-3 gap-4 pb-4">
							{#each articles as article (article.id)}
								<article
									class="p-4 rounded-lg border bg-[var(--surface-bg)] border-[var(--surface-border)]
										hover:border-[var(--accent-primary)] hover:shadow-md transition-all group"
								>
									<a
										href={article.link}
										target="_blank"
										rel="noopener noreferrer"
										class="block"
									>
										<h3 class="text-base font-semibold text-[var(--text-primary)] group-hover:text-[var(--accent-primary)] line-clamp-2 mb-2">
											{article.title}
										</h3>
										<div class="flex items-center gap-2 text-xs text-[var(--text-secondary)]">
											{#if article.feedTitle}
												<span class="truncate max-w-[150px]">{article.feedTitle}</span>
												<span>·</span>
											{/if}
											<span>{formatDate(article.publishedAt)}</span>
										</div>
									</a>

									<!-- Article Tags -->
									<div class="mt-3 pt-3 border-t border-[var(--surface-border)]">
										{#if loadingArticleTags.has(article.id)}
											<div class="flex gap-1">
												{#each [1, 2] as i}
													<div class="h-6 w-14 rounded-full animate-pulse bg-[var(--muted)]"></div>
												{/each}
											</div>
										{:else if getArticleTags(article.id).length > 0}
											<div class="flex flex-wrap gap-1.5">
												{#each getArticleTags(article.id).slice(0, 5) as tag}
													<button
														type="button"
														onclick={() => handleTagClick(tag)}
														class="px-2 py-0.5 text-xs rounded-full bg-[var(--muted)] text-[var(--text-secondary)]
															hover:bg-[var(--surface-hover)] hover:text-[var(--text-primary)] transition-colors"
													>
														{tag.name}
													</button>
												{/each}
												{#if getArticleTags(article.id).length > 5}
													<span class="px-2 py-0.5 text-xs text-[var(--text-tertiary)]">
														+{getArticleTags(article.id).length - 5} more
													</span>
												{/if}
											</div>
										{:else}
											<span class="text-xs text-[var(--text-tertiary)]">No tags</span>
										{/if}
									</div>
								</article>
							{/each}
						</div>

						{#if isLoadingArticles}
							<div class="flex justify-center py-4">
								<Loader2 size={24} class="animate-spin text-[var(--alt-primary)]" />
							</div>
						{/if}

						{#if !isLoadingArticles && hasMoreArticles}
							<div class="flex justify-center py-4">
								<button
									type="button"
									onclick={handleLoadMore}
									class="px-6 py-2 text-sm font-medium rounded-lg
										bg-[var(--surface-hover)] hover:bg-[var(--muted)] text-[var(--text-primary)] transition-colors"
								>
									Load more articles
								</button>
							</div>
						{/if}
					</div>
				{:else}
					<div class="flex-1 flex items-center justify-center">
						<p class="text-[var(--text-secondary)]">No articles found with this tag</p>
					</div>
				{/if}
			{:else}
				<!-- Welcome State -->
				<div class="flex-1 flex items-center justify-center">
					<div class="text-center max-w-md">
						<div class="w-16 h-16 mx-auto mb-4 rounded-full bg-[var(--alt-primary)] bg-opacity-10 flex items-center justify-center">
							<Shuffle size={32} class="text-[var(--alt-primary)]" />
						</div>
						<h2 class="text-xl font-semibold text-[var(--text-primary)] mb-2">
							Start Your Tag Trail
						</h2>
						<p class="text-[var(--text-secondary)]">
							Click on a tag from the feed on the left to discover related articles across all your subscriptions.
							Follow the tag trail to explore new content!
						</p>
					</div>
				</div>
			{/if}
		</main>
	</div>
</div>
