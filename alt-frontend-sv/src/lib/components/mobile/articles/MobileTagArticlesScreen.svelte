<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import {
	createClientTransport,
	fetchArticlesByTag,
	type TagTrailArticle,
} from "$lib/connect";
import { Loader2, Tag, ArrowLeft, SquareArrowOutUpRight } from "@lucide/svelte";

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

async function loadArticles(cursor?: string) {
	if (!browser) return;
	try {
		const transport = createClientTransport();
		const result = await fetchArticlesByTag(transport, tagName, undefined, cursor ?? undefined, 20);
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

function formatDate(dateStr: string): string {
	const date = new Date(dateStr);
	return date.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}
</script>

<div class="flex flex-col h-full" style="background: var(--app-bg);">
	<header class="flex items-center gap-3 px-4 py-3 border-b" style="border-color: var(--surface-border);">
		<a href="/home" class="p-1">
			<ArrowLeft size={20} style="color: var(--text-primary);" />
		</a>
		<div class="flex items-center gap-2">
			<Tag size={16} style="color: var(--alt-primary);" />
			<h1 class="text-base font-semibold" style="color: var(--text-primary);">
				{tagName}
			</h1>
		</div>
	</header>

	<div class="flex-1 overflow-y-auto px-4 py-4">
		{#if isLoading}
			<div class="flex justify-center py-8">
				<Loader2 size={24} class="animate-spin" style="color: var(--alt-primary);" />
			</div>
		{:else if error}
			<div class="text-center py-8">
				<p class="text-sm" style="color: var(--text-secondary);">{error}</p>
			</div>
		{:else if articles.length === 0}
			<div class="text-center py-8" data-testid="empty-state">
				<p class="text-sm" style="color: var(--text-secondary);">No articles found with this tag.</p>
			</div>
		{:else}
			<div class="flex flex-col gap-3" data-testid="article-list">
				{#each articles as article (article.id)}
					<div
						class="p-3 rounded-xl border"
						style="background: var(--surface-bg); border-color: var(--surface-border);"
						data-testid="tag-article-{article.id}"
					>
						<div class="flex items-start gap-2">
							<SquareArrowOutUpRight size={14} class="mt-0.5 flex-shrink-0" style="color: var(--alt-primary);" />
							<div class="flex-1 min-w-0">
								<a
									href={article.link}
									target="_blank"
									rel="noopener noreferrer"
									class="text-base font-semibold hover:underline leading-snug line-clamp-2"
									style="color: var(--text-primary);"
								>
									{article.title}
								</a>
								<div class="flex items-center gap-2 mt-1">
									{#if article.feedTitle}
										<span class="text-xs" style="color: var(--text-secondary);">{article.feedTitle}</span>
										<span style="color: var(--text-secondary);">·</span>
									{/if}
									<span class="text-xs" style="color: var(--text-secondary);">{formatDate(article.publishedAt)}</span>
								</div>
							</div>
						</div>
					</div>
				{/each}
			</div>

			{#if hasMore}
				<button
					type="button"
					class="w-full py-3 text-sm font-medium rounded-xl mt-4 min-h-[44px] transition-all active:scale-95"
					style="background: var(--muted); color: var(--text-primary);"
					onclick={loadMore}
					disabled={isLoadingMore}
				>
					{#if isLoadingMore}
						Loading...
					{:else}
						Load more
					{/if}
				</button>
			{/if}
		{/if}
	</div>
</div>
