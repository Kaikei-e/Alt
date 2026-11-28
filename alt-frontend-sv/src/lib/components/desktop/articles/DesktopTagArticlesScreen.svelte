<script lang="ts">
import { ArrowLeft, Loader2, Tag } from "@lucide/svelte";
import { onMount } from "svelte";
import {
	createClientTransport,
	fetchArticlesByTag,
	type TagTrailArticle,
} from "#lib/connect/index.js";
import { tagTrailArticleToReaderItem } from "#lib/reader/mappers.js";
import { openReaderOverlay } from "#lib/reader/reader-overlay.svelte.js";
import { ReadingSurface } from "#lib/reading/instrumentation.js";
import { browser } from "$app/env";
import ArticleCard from "./ArticleCard.svelte";

interface Props {
	tagName: string;
	articles?: TagTrailArticle[];
	isLoading?: boolean;
	error?: string | null;
	hasMore?: boolean;
	isLoadingMore?: boolean;
	onLoadMore?: () => Promise<void> | void;
}

let {
	tagName,
	articles: externalArticles,
	isLoading: externalIsLoading,
	error: externalError,
	hasMore: externalHasMore,
	isLoadingMore: externalIsLoadingMore,
	onLoadMore,
}: Props = $props();

let internalArticles = $state<TagTrailArticle[]>([]);
let internalIsLoading = $state(true);
let internalError = $state<string | null>(null);
let nextCursor = $state<string | null>(null);
let internalHasMore = $state(false);
let internalIsLoadingMore = $state(false);

const isControlled = $derived(onLoadMore !== undefined);
const articles = $derived(
	isControlled ? (externalArticles ?? []) : internalArticles,
);
const isLoading = $derived(
	isControlled ? (externalIsLoading ?? false) : internalIsLoading,
);
const error = $derived(isControlled ? (externalError ?? null) : internalError);
const hasMore = $derived(
	isControlled ? (externalHasMore ?? false) : internalHasMore,
);
const isLoadingMore = $derived(
	isControlled ? (externalIsLoadingMore ?? false) : internalIsLoadingMore,
);

async function loadArticles(cursor?: string) {
	if (!browser) return;
	try {
		const transport = createClientTransport();
		const result = await fetchArticlesByTag(
			transport,
			tagName,
			undefined,
			cursor ?? undefined,
			20,
		);
		if (cursor) {
			internalArticles = [...internalArticles, ...result.articles];
		} else {
			internalArticles = result.articles;
		}
		nextCursor = result.nextCursor;
		internalHasMore = result.hasMore;
	} catch (e) {
		internalError = e instanceof Error ? e.message : "Failed to load articles";
	}
}

async function loadMore() {
	if (onLoadMore) {
		await onLoadMore();
		return;
	}
	if (!nextCursor || internalIsLoadingMore) return;
	internalIsLoadingMore = true;
	await loadArticles(nextCursor);
	internalIsLoadingMore = false;
}

onMount(async () => {
	if (isControlled) return;
	await loadArticles();
	internalIsLoading = false;
});

function handleSelectArticle(index: number) {
	openReaderOverlay({
		surface: ReadingSurface.OTHER,
		items: articles.map(tagTrailArticleToReaderItem),
		currentIndex: index,
	});
}
</script>

<div class="max-w-[90rem] mx-auto px-6 py-8">
	<div class="flex items-center gap-3 mb-6">
		<a
			href="/today"
			class="flex items-center gap-1 text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)] transition-colors"
		>
			<ArrowLeft class="h-4 w-4" />
			Today
		</a>
	</div>

	<div class="flex items-center gap-2 mb-6">
		<Tag class="h-5 w-5 text-[var(--interactive-text)]" />
		<h1 class="text-xl font-bold text-[var(--text-primary)]">
			Articles tagged "{tagName}"
		</h1>
		{#if !isLoading && articles.length > 0}
			<span class="text-sm text-[var(--text-muted)] ml-2">{articles.length} articles</span>
		{/if}
	</div>

	{#if isLoading}
		<div class="flex justify-center py-12">
			<Loader2 class="h-6 w-6 animate-spin text-[var(--interactive-text)]" />
		</div>
	{:else if error}
		<div class="text-center py-12">
			<p class="text-sm text-[var(--text-secondary)]">{error}</p>
		</div>
	{:else if articles.length === 0}
		<div class="text-center py-12" data-testid="empty-state">
			<p class="text-sm text-[var(--text-secondary)]">No articles found with this tag.</p>
		</div>
	{:else}
		<div data-testid="article-list">
			<div
				class="grid grid-cols-2 lg:grid-cols-3 gap-3"
				data-testid="article-grid"
			>
				{#each articles as article, i (article.id)}
					<ArticleCard
						article={article}
						selected={false}
						onclick={() => handleSelectArticle(i)}
					/>
				{/each}
			</div>

			{#if hasMore}
				<div class="flex justify-center mt-6">
					<button
						type="button"
						class="rounded-lg border border-[var(--surface-border)] px-6 py-2 text-sm font-medium text-[var(--text-primary)] hover:bg-[var(--surface-hover)] transition-colors"
						onclick={loadMore}
						disabled={isLoadingMore}
					>
						{#if isLoadingMore}
							<Loader2 class="h-4 w-4 animate-spin inline mr-2" />
						{/if}
						Load more
					</button>
				</div>
			{/if}
		</div>
	{/if}
</div>
