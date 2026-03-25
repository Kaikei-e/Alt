<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import {
	createClientTransport,
	fetchArticlesByTag,
	type TagTrailArticle,
} from "$lib/connect";
import { Loader2, Tag, ArrowLeft } from "@lucide/svelte";
import ArticleCard from "./ArticleCard.svelte";
import ArticleDetailPanel from "./ArticleDetailPanel.svelte";

interface Props {
	tagName: string;
}

const { tagName }: Props = $props();

let articles = $state<TagTrailArticle[]>([]);
let isLoading = $state(true);
let error = $state<string | null>(null);
let nextCursor = $state<string | null>(null);
let hasMore = $state(false);
let isLoadingMore = $state(false);
let selectedArticle = $state<TagTrailArticle | null>(null);

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
			articles = [...articles, ...result.articles];
		} else {
			articles = result.articles;
		}
		nextCursor = result.nextCursor;
		hasMore = result.hasMore;
	} catch (e) {
		error = e instanceof Error ? e.message : "Failed to load articles";
	}
}

async function loadMore() {
	if (!nextCursor || isLoadingMore) return;
	isLoadingMore = true;
	await loadArticles(nextCursor);
	isLoadingMore = false;
}

onMount(async () => {
	await loadArticles();
	isLoading = false;
});
</script>

<div class="max-w-[90rem] mx-auto px-6 py-8">
	<div class="flex items-center gap-3 mb-6">
		<a
			href="/home"
			class="flex items-center gap-1 text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)] transition-colors"
		>
			<ArrowLeft class="h-4 w-4" />
			Home
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
				{#each articles as article (article.id)}
					<ArticleCard
						{article}
						selected={selectedArticle?.id === article.id}
						onclick={() => { selectedArticle = article; }}
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

		<!-- Slide-over Detail Panel (overlay) -->
		{#if selectedArticle}
			<ArticleDetailPanel
				article={selectedArticle}
				onClose={() => { selectedArticle = null; }}
			/>
		{/if}
	{/if}
</div>
