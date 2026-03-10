<script lang="ts">
import { onMount } from "svelte";
import {
	createClientTransport,
	fetchArticlesByTag,
	type TagTrailArticle,
} from "$lib/connect";
import { ExternalLink, Loader2 } from "@lucide/svelte";

interface Props {
	tagName: string;
}

let { tagName }: Props = $props();

let articles = $state<TagTrailArticle[]>([]);
let isLoading = $state(false);
let nextCursor = $state<string | null>(null);
let hasMore = $state(false);
let error = $state<string | null>(null);

async function loadArticles(cursor?: string) {
	isLoading = true;
	error = null;
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
	} finally {
		isLoading = false;
	}
}

onMount(() => {
	loadArticles();
});

// Reload when tag changes
$effect(() => {
	tagName; // track dependency
	articles = [];
	nextCursor = null;
	hasMore = false;
	loadArticles();
});

function formatDate(dateStr: string): string {
	try {
		return new Date(dateStr).toLocaleDateString("en-US", {
			month: "short",
			day: "numeric",
			year: "numeric",
		});
	} catch {
		return dateStr;
	}
}
</script>

<div class="flex flex-col gap-2 overflow-y-auto flex-1 pr-1">
	{#if error}
		<div class="text-red-400 text-sm py-4 text-center">{error}</div>
	{/if}

	{#each articles as article (article.id)}
		<a
			href={article.link}
			target="_blank"
			rel="noopener noreferrer"
			class="group flex flex-col gap-1 rounded-lg border border-white/10 bg-white/5 p-3 transition-colors hover:border-cyan-500/30 hover:bg-white/10"
		>
			<div class="flex items-start justify-between gap-2">
				<h4 class="text-sm font-medium text-white/90 group-hover:text-cyan-300 line-clamp-2">
					{article.title}
				</h4>
				<ExternalLink class="h-3.5 w-3.5 flex-shrink-0 text-white/40 group-hover:text-cyan-400" />
			</div>
			<div class="flex items-center gap-2 text-xs text-white/50">
				{#if article.feedTitle}
					<span class="truncate max-w-[180px]">{article.feedTitle}</span>
					<span>·</span>
				{/if}
				<span>{formatDate(article.publishedAt)}</span>
			</div>
		</a>
	{/each}

	{#if isLoading}
		<div class="flex items-center justify-center py-4">
			<Loader2 class="h-5 w-5 animate-spin text-cyan-400" />
		</div>
	{/if}

	{#if hasMore && !isLoading && nextCursor}
		<button
			type="button"
			onclick={() => loadArticles(nextCursor ?? undefined)}
			class="w-full py-2 text-sm text-cyan-400 hover:text-cyan-300 transition-colors"
		>
			Load more
		</button>
	{/if}

	{#if !isLoading && articles.length === 0 && !error}
		<div class="text-white/40 text-sm py-8 text-center">
			No articles found for this tag
		</div>
	{/if}
</div>
