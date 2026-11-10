<script lang="ts">
import { ExternalLink } from "@lucide/svelte";
import {
	createClientTransport,
	fetchArticlesByTag,
	type TagTrailArticle,
} from "#lib/connect/index.js";
import { sanitizeHrefUrl } from "#lib/utils/urlSafety.js";

interface Props {
	tagName: string;
}

let { tagName }: Props = $props();

let articles = $state<TagTrailArticle[]>([]);
let isLoading = $state(false);
let nextCursor = $state<string | null>(null);
let hasMore = $state(false);
let error = $state<string | null>(null);

async function loadMoreArticles() {
	if (!nextCursor || isLoading) return;
	const cursor = nextCursor;
	const currentTag = tagName;
	isLoading = true;
	error = null;
	try {
		const transport = createClientTransport();
		const result = await fetchArticlesByTag(
			transport,
			currentTag,
			undefined,
			cursor,
			20,
		);
		if (currentTag !== tagName) return;
		articles = [...articles, ...result.articles];
		nextCursor = result.nextCursor;
		hasMore = result.hasMore;
	} catch (e) {
		if (currentTag !== tagName) return;
		error = e instanceof Error ? e.message : "Failed to load articles";
	} finally {
		if (currentTag === tagName) {
			isLoading = false;
		}
	}
}

let lastTag: string | null = null;
let requestSeq = 0;

$effect(() => {
	const currentTag = tagName;
	if (currentTag === lastTag) {
		return;
	}
	lastTag = currentTag;
	const seq = ++requestSeq;
	articles = [];
	nextCursor = null;
	hasMore = false;
	isLoading = true;
	error = null;

	const transport = createClientTransport();
	fetchArticlesByTag(transport, currentTag, undefined, undefined, 20)
		.then((result) => {
			if (seq !== requestSeq) return;
			articles = result.articles;
			nextCursor = result.nextCursor;
			hasMore = result.hasMore;
		})
		.catch((e) => {
			if (seq !== requestSeq) return;
			error = e instanceof Error ? e.message : "Failed to load articles";
		})
		.finally(() => {
			if (seq === requestSeq) {
				isLoading = false;
			}
		});
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

<div class="flex flex-1 flex-col overflow-y-auto">
	{#if error}
		<div class="px-4 py-4 text-center text-sm text-[var(--alt-error)]">
			{error}
		</div>
	{/if}

	<div class="divide-y divide-[var(--surface-border)]">
		{#each articles as article (article.id)}
			<a
				href={sanitizeHrefUrl(article.link) ?? "#"}
				target="_blank"
				rel="noopener noreferrer"
				class="group flex flex-col gap-1 px-4 py-3 transition-colors hover:bg-[var(--surface-hover)]"
			>
				<div class="flex items-start justify-between gap-2">
					<h4
						class="line-clamp-2 text-sm font-medium text-[var(--text-primary)] group-hover:text-[var(--interactive-text)]"
						style="font-family: var(--font-body);"
					>
						{article.title}
					</h4>
					<ExternalLink
						class="h-3.5 w-3.5 shrink-0 text-[var(--text-muted)] group-hover:text-[var(--interactive-text)]"
					/>
				</div>
				<div class="flex items-center gap-2 text-xs text-[var(--text-muted)]">
					{#if article.feedTitle}
						<span class="max-w-[180px] truncate text-[var(--text-secondary)]">
							{article.feedTitle}
						</span>
						<span>·</span>
					{/if}
					<span style="font-family: var(--font-mono);">
						{formatDate(article.publishedAt)}
					</span>
				</div>
			</a>
		{/each}
	</div>

	{#if isLoading}
		<div
			class="flex items-center justify-center py-4 text-xs text-[var(--text-muted)]"
			style="font-family: var(--font-mono);"
		>
			Loading articles...
		</div>
	{/if}

	{#if hasMore && !isLoading && nextCursor}
		<div class="p-3">
			<button
				type="button"
				onclick={loadMoreArticles}
				class="w-full rounded-none border border-[var(--surface-border)] bg-[var(--action-surface)] px-4 py-2 text-xs font-medium text-[var(--text-primary)] transition-colors hover:bg-[var(--action-surface-hover)]"
			>
				Load more
			</button>
		</div>
	{/if}

	{#if !isLoading && articles.length === 0 && !error}
		<div class="px-4 py-8 text-center text-sm text-[var(--text-muted)]">
			No articles found for this tag
		</div>
	{/if}
</div>
